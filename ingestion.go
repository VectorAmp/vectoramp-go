package vectoramp

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
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
	err := s.client.do(ctx, "POST", "/ingestion/sources", nil, req, &out)
	return &out, err
}

// DeleteSourceOption customizes a DeleteSource request.
type DeleteSourceOption func(*deleteSourceOptions)

type deleteSourceOptions struct {
	force bool
}

// WithForce forces deletion of a source even when it is still referenced by
// datasets, jobs, or schedules, sending force=true. Without it the API rejects
// deletes of in-use sources.
func WithForce() DeleteSourceOption {
	return func(o *deleteSourceOptions) { o.force = true }
}

// DeleteSource deletes an ingestion source by ID.
//
// By default the API refuses to delete a source that is still referenced; pass
// WithForce to delete it regardless of its references.
func (s *IngestionService) DeleteSource(ctx context.Context, sourceID string, opts ...DeleteSourceOption) error {
	var o deleteSourceOptions
	for _, opt := range opts {
		opt(&o)
	}
	var q url.Values
	if o.force {
		q = url.Values{}
		q.Set("force", "true")
	}
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/ingestion/sources/%s", sourceID), q, nil, nil)
}

// ListUnusedSources returns ingestion sources that are not referenced by any
// dataset, job, or schedule, using optional limit and offset pagination.
//
// Pass zero for limit or offset to omit that query parameter.
func (s *IngestionService) ListUnusedSources(ctx context.Context, limit, offset int) (*SourceList, error) {
	var out SourceList
	err := s.client.do(ctx, "GET", "/ingestion/sources/unused", paginationQuery(limit, offset), nil, &out)
	return &out, err
}

// DeletedSource identifies one source removed by a cleanup operation.
type DeletedSource struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// CleanupUnusedSourcesResponse reports the sources removed by CleanupUnusedSources.
type CleanupUnusedSourcesResponse struct {
	Deleted []DeletedSource `json:"deleted"`
	Count   int             `json:"count"`
}

// CleanupUnusedSources deletes every unused ingestion source and reports which
// source IDs were removed.
func (s *IngestionService) CleanupUnusedSources(ctx context.Context) (*CleanupUnusedSourcesResponse, error) {
	var out CleanupUnusedSourcesResponse
	err := s.client.do(ctx, "POST", "/ingestion/sources/cleanup", nil, nil, &out)
	return &out, err
}

// ScheduleReference identifies a schedule that references an ingestion source.
type ScheduleReference struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// SourceReferences reports what currently depends on an ingestion source: the
// active schedules and in-flight jobs that make it "in use" (not safe to remove).
type SourceReferences struct {
	Schedules      []ScheduleReference `json:"schedules"`
	ScheduleCount  int                 `json:"schedule_count"`
	ActiveJobCount int                 `json:"active_job_count"`
	InUse          bool                `json:"in_use"`
}

// GetSourceReferences returns the resources (datasets, jobs, schedules) that
// reference an ingestion source.
func (s *IngestionService) GetSourceReferences(ctx context.Context, sourceID string) (*SourceReferences, error) {
	var out SourceReferences
	err := s.client.do(ctx, "GET", fmt.Sprintf("/ingestion/sources/%s/references", sourceID), nil, nil, &out)
	return &out, err
}

// ValidateSourceResponse reports the result of validating a source config.
type ValidateSourceResponse struct {
	Success          bool                     `json:"success"`
	Message          string                   `json:"message,omitempty"`
	NormalizedConfig map[string]interface{}   `json:"normalized_config,omitempty"`
	Samples          []map[string]interface{} `json:"samples,omitempty"`
	Warnings         []string                 `json:"warnings,omitempty"`
}

// ValidateSource validates a source type and config without creating a source,
// returning whether the config is valid plus any errors or warnings.
func (s *IngestionService) ValidateSource(ctx context.Context, sourceType string, config map[string]interface{}) (*ValidateSourceResponse, error) {
	body := map[string]interface{}{"source_type": sourceType, "config": config}
	var out ValidateSourceResponse
	err := s.client.do(ctx, "POST", "/ingestion/sources/validate", nil, body, &out)
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

// RetryJob queues a fresh full-rerun job from an eligible failed or cancelled job.
func (s *IngestionService) RetryJob(ctx context.Context, jobID string) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "POST", fmt.Sprintf("/ingestion/jobs/%s/retry", jobID), nil, nil, &out)
	return &out, err
}

// InitUpload initializes presigned uploads for a file_upload source.
//
// req.Files describes the files to upload. The response includes a job ID and
// one upload target per file.
func (s *IngestionService) InitUpload(ctx context.Context, sourceID string, req InitUploadRequest) (*InitUploadResponse, error) {
	var out InitUploadResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/ingestion/sources/%s/upload/init", sourceID), nil, req, &out)
	return &out, err
}

// CompleteUpload completes a presigned file-upload job and returns the job.
func (s *IngestionService) CompleteUpload(ctx context.Context, sourceID string, req CompleteUploadRequest) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "POST", fmt.Sprintf("/ingestion/sources/%s/upload/complete", sourceID), nil, req, &out)
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
	job, err := s.CompleteUpload(ctx, sourceID, CompleteUploadRequest{JobID: init.JobID, FileIDs: fileIDs})
	if err != nil {
		return nil, err
	}
	if job.JobID == "" {
		job.JobID = init.JobID
	}
	return job, nil
}

func putFile(ctx context.Context, uploadURL, path, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, f)
	if err != nil {
		return err
	}
	req.ContentLength = st.Size()
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
