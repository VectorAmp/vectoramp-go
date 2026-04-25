package vectoramp

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// IngestionService manages sources, upload flows, and ingestion jobs.
type IngestionService struct{ client *Client }

// ListSources returns ingestion sources using optional limit and offset pagination.
//
// Pass zero for limit or offset to omit that query parameter. The response
// includes sources plus total, limit, and offset pagination metadata.
func (s *IngestionService) ListSources(ctx context.Context, limit, offset int) (*SourceList, error) {
	var out SourceList
	err := s.client.do(ctx, "GET", "/ingestion/sources", paginationQuery(limit, offset), nil, &out)
	return &out, err
}

// GetSource returns one ingestion source by ID.
func (s *IngestionService) GetSource(ctx context.Context, sourceID string) (*Source, error) {
	var out Source
	err := s.client.do(ctx, "GET", fmt.Sprintf("/ingestion/sources/%s", sourceID), nil, nil, &out)
	return &out, err
}

// CreateSource creates an ingestion source and returns it.
//
// source may be a CreateSourceRequest, *CreateSourceRequest, or typed
// SourceBuilder such as WebSource, S3Source, GoogleDriveSource, or
// FileUploadSource. Typed builders fill source_type, config defaults, and source
// name defaults before sending the request.
func (s *IngestionService) CreateSource(ctx context.Context, source interface{}) (*Source, error) {
	req, ok := normalizeCreateSourceRequest(source)
	if !ok {
		return nil, fmt.Errorf("vectoramp: unsupported source create input %T", source)
	}
	var out Source
	err := s.client.do(ctx, "POST", "/v1/sources", nil, req, &out)
	return &out, err
}

// StartJob starts an ingestion job and returns the created job.
//
// req.SourceID and req.DatasetID are required. req.PipelineID is optional; omit
// it to let the API choose the default pipeline.
func (s *IngestionService) StartJob(ctx context.Context, req StartIngestionRequest) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "POST", "/ingestion/jobs", nil, req, &out)
	return &out, err
}

// ListJobs returns ingestion jobs, optionally filtered by datasetID.
//
// Pass an empty datasetID to list across datasets. Pass zero for limit or offset
// to omit that pagination parameter.
func (s *IngestionService) ListJobs(ctx context.Context, datasetID string, limit, offset int) (*JobList, error) {
	q := paginationQuery(limit, offset)
	if datasetID != "" {
		q.Set("dataset_id", datasetID)
	}
	var out JobList
	err := s.client.do(ctx, "GET", "/ingestion/jobs", q, nil, &out)
	return &out, err
}

// GetJob returns one ingestion job by ID.
func (s *IngestionService) GetJob(ctx context.Context, jobID string) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "GET", fmt.Sprintf("/ingestion/jobs/%s", jobID), nil, nil, &out)
	return &out, err
}

// InitUpload initializes presigned uploads for a file_upload source.
//
// req.Files describes the files to upload. The response includes a job ID and
// one upload target per file.
func (s *IngestionService) InitUpload(ctx context.Context, sourceID string, req InitUploadRequest) (*InitUploadResponse, error) {
	var out InitUploadResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/v1/sources/%s/upload/init", sourceID), nil, req, &out)
	return &out, err
}

// CompleteUpload completes a presigned file-upload job and returns the job.
func (s *IngestionService) CompleteUpload(ctx context.Context, sourceID string, req CompleteUploadRequest) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "POST", fmt.Sprintf("/v1/sources/%s/upload/complete", sourceID), nil, req, &out)
	return &out, err
}

// IngestFilesOptions customizes the high-level local file ingestion flow.
//
// SourceName defaults to go-sdk-file-upload-<dataset>-<timestamp>. Description
// and Metadata are copied to the auto-created file_upload source. PipelineID is
// reserved for API compatibility with ingestion flows that select a pipeline.
type IngestFilesOptions struct {
	SourceName  string
	Description string
	PipelineID  string
	Metadata    Metadata
}

// IngestFiles uploads local files into datasetID and returns the ingestion job.
//
// The SDK automatically creates a file_upload source, using opts.SourceName or a
// generated go-sdk-file-upload-<dataset>-<timestamp> name, initializes presigned
// uploads, PUTs each local path with a detected content type, and completes the
// upload job. opts may be nil.
func (s *IngestionService) IngestFiles(ctx context.Context, datasetID string, paths []string, opts *IngestFilesOptions) (*Job, error) {
	if opts == nil {
		opts = &IngestFilesOptions{}
	}
	name := opts.SourceName
	if name == "" {
		name = defaultUploadSourceName(datasetID, time.Now().UTC())
	}
	md := Metadata{"dataset_id": datasetID}
	for k, v := range opts.Metadata {
		md[k] = v
	}
	src, err := s.CreateFileUpload(ctx, FileUploadSource{Name: name, Description: opts.Description, Metadata: md})
	if err != nil {
		return nil, err
	}
	sourceID, err := sourceIdentifier(*src)
	if err != nil {
		return nil, err
	}
	files := make([]UploadFile, len(paths))
	for i, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		ct := mime.TypeByExtension(filepath.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		files[i] = UploadFile{Name: filepath.Base(p), SizeBytes: st.Size(), ContentType: ct}
	}
	init, err := s.InitUpload(ctx, sourceID, InitUploadRequest{Files: files})
	if err != nil {
		return nil, err
	}
	fileIDs := make([]string, 0, len(init.Uploads))
	for i, u := range init.Uploads {
		if i >= len(paths) {
			break
		}
		if err := putFile(ctx, u.UploadURL, paths[i], files[i].ContentType); err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, u.FileID)
	}
	return s.CompleteUpload(ctx, sourceID, CompleteUploadRequest{JobID: init.JobID, FileIDs: fileIDs})
}

func putFile(ctx context.Context, uploadURL, path, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, f)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Header: resp.Header, Message: "file upload failed"}
	}
	return nil
}

func defaultUploadSourceName(datasetID string, now time.Time) string {
	if datasetID == "" {
		return "go-sdk-file-upload-" + now.Format("20060102-150405")
	}
	return "go-sdk-file-upload-" + sanitizeName(datasetID) + "-" + now.Format("20060102-150405")
}
