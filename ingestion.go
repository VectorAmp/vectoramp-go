package vectoramp

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

type IngestionService struct{ client *Client }

func (s *IngestionService) ListSources(ctx context.Context, limit, offset int) (*SourceList, error) {
	var out SourceList
	err := s.client.do(ctx, "GET", "/ingestion/sources", paginationQuery(limit, offset), nil, &out)
	return &out, err
}
func (s *IngestionService) GetSource(ctx context.Context, sourceID string) (*Source, error) {
	var out Source
	err := s.client.do(ctx, "GET", fmt.Sprintf("/ingestion/sources/%s", sourceID), nil, nil, &out)
	return &out, err
}
func (s *IngestionService) CreateSource(ctx context.Context, req CreateSourceRequest) (*Source, error) {
	var out Source
	err := s.client.do(ctx, "POST", "/v1/sources", nil, req, &out)
	return &out, err
}
func (s *IngestionService) StartJob(ctx context.Context, req StartIngestionRequest) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "POST", "/ingestion/jobs", nil, req, &out)
	return &out, err
}
func (s *IngestionService) ListJobs(ctx context.Context, datasetID string, limit, offset int) (*JobList, error) {
	q := paginationQuery(limit, offset)
	if datasetID != "" {
		q.Set("dataset_id", datasetID)
	}
	var out JobList
	err := s.client.do(ctx, "GET", "/ingestion/jobs", q, nil, &out)
	return &out, err
}
func (s *IngestionService) GetJob(ctx context.Context, jobID string) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "GET", fmt.Sprintf("/ingestion/jobs/%s", jobID), nil, nil, &out)
	return &out, err
}
func (s *IngestionService) InitUpload(ctx context.Context, sourceID string, req InitUploadRequest) (*InitUploadResponse, error) {
	var out InitUploadResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/v1/sources/%s/upload/init", sourceID), nil, req, &out)
	return &out, err
}
func (s *IngestionService) CompleteUpload(ctx context.Context, sourceID string, req CompleteUploadRequest) (*Job, error) {
	var out Job
	err := s.client.do(ctx, "POST", fmt.Sprintf("/v1/sources/%s/upload/complete", sourceID), nil, req, &out)
	return &out, err
}

type IngestFilesOptions struct {
	SourceName  string
	Description string
	PipelineID  string
	Metadata    Metadata
}

func (s *IngestionService) IngestFiles(ctx context.Context, datasetID string, paths []string, opts *IngestFilesOptions) (*Job, error) {
	if opts == nil {
		opts = &IngestFilesOptions{}
	}
	name := opts.SourceName
	if name == "" {
		name = "go-sdk-file-upload"
	}
	md := Metadata{"dataset_id": datasetID}
	for k, v := range opts.Metadata {
		md[k] = v
	}
	src, err := s.CreateSource(ctx, CreateSourceRequest{SourceType: "file_upload", Name: name, Description: opts.Description, Config: map[string]interface{}{"storage_provider": "s3", "sync_mode": "full"}, Metadata: md})
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
	init, err := s.InitUpload(ctx, src.ID, InitUploadRequest{Files: files})
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
	return s.CompleteUpload(ctx, src.ID, CompleteUploadRequest{JobID: init.JobID, FileIDs: fileIDs})
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
