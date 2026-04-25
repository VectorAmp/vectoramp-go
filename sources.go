package vectoramp

import (
	"context"
	"fmt"
)

const (
	SourceTypeS3         = "s3"
	SourceTypeWeb        = "web"
	SourceTypeGDrive     = "gdrive"
	SourceTypeFileUpload = "file_upload"
)

// SourceBuilder is implemented by typed ingestion source definitions that can
// be converted into the public CreateSourceRequest body.
type SourceBuilder interface {
	ToCreateSourceRequest() CreateSourceRequest
}

// GenericSource is an escape hatch for source types or config fields not yet
// modeled by this SDK.
type GenericSource struct {
	SourceType  string
	Name        string
	Description string
	Config      map[string]interface{}
	Metadata    Metadata
}

func (s GenericSource) ToCreateSourceRequest() CreateSourceRequest {
	return CreateSourceRequest{SourceType: s.SourceType, Name: s.Name, Description: s.Description, Config: cloneMap(s.Config), Metadata: cloneMetadata(s.Metadata)}
}

// WebSource describes a web crawler ingestion source.
type WebSource struct {
	Name             string
	StartURLs        []string
	MaxDepth         int
	MaxPages         int
	AllowedDomains   []string
	RateLimitMS      int
	RespectRobotsTxt *bool
	Selectors        *WebSelectors
	Headers          map[string]string
	Description      string
	Metadata         Metadata
	ConfigExtra      map[string]interface{}
}

type WebSelectors struct {
	Content string   `json:"content,omitempty"`
	Title   string   `json:"title,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

func (s WebSource) ToCreateSourceRequest() CreateSourceRequest {
	config := map[string]interface{}{
		"type":       SourceTypeWeb,
		"start_urls": append([]string(nil), s.StartURLs...),
	}
	setNonZero(config, "max_depth", s.MaxDepth)
	setNonZero(config, "max_pages", s.MaxPages)
	setStringSlice(config, "allowed_domains", s.AllowedDomains)
	setNonZero(config, "rate_limit_ms", s.RateLimitMS)
	if s.RespectRobotsTxt != nil {
		config["respect_robots_txt"] = *s.RespectRobotsTxt
	}
	if s.Selectors != nil {
		config["selectors"] = s.Selectors
	}
	if len(s.Headers) > 0 {
		headers := make(map[string]string, len(s.Headers))
		for k, v := range s.Headers {
			headers[k] = v
		}
		config["headers"] = headers
	}
	mergeExtra(config, s.ConfigExtra)
	return CreateSourceRequest{SourceType: SourceTypeWeb, Name: s.Name, Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// S3Source describes an Amazon S3 ingestion source.
type S3Source struct {
	Name            string
	Bucket          string
	Prefix          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	FilePatterns    []string
	MaxFileSizeMB   int
	SyncMode        string
	Description     string
	Metadata        Metadata
	ConfigExtra     map[string]interface{}
}

func (s S3Source) ToCreateSourceRequest() CreateSourceRequest {
	config := map[string]interface{}{
		"type":      SourceTypeS3,
		"bucket":    s.Bucket,
		"sync_mode": defaultString(s.SyncMode, "incremental"),
	}
	setNonEmpty(config, "prefix", s.Prefix)
	setNonEmpty(config, "region", s.Region)
	setNonEmpty(config, "access_key_id", s.AccessKeyID)
	setNonEmpty(config, "secret_access_key", s.SecretAccessKey)
	setStringSlice(config, "file_patterns", s.FilePatterns)
	setNonZero(config, "max_file_size_mb", s.MaxFileSizeMB)
	mergeExtra(config, s.ConfigExtra)
	return CreateSourceRequest{SourceType: SourceTypeS3, Name: s.Name, Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// GoogleDriveSource describes a Google Drive ingestion source. The public
// source_type is "gdrive".
type GoogleDriveSource struct {
	Name                   string
	AuthMode               string
	ServiceAccountJSON     string
	DelegatedUser          string
	OAuthCredentials       map[string]interface{}
	DriveID                string
	FolderIDs              []string
	Query                  string
	MimeTypes              []string
	IncludeSharedDrives    *bool
	IncludeTeamDrives      *bool
	SyncMode               string
	PageSize               int
	ResumeCursor           string
	FetchAttachments       *bool
	SamplingEnabled        *bool
	SamplingLimit          int
	MaxConcurrentDownloads int
	Description            string
	Metadata               Metadata
	ConfigExtra            map[string]interface{}
}

func (s GoogleDriveSource) ToCreateSourceRequest() CreateSourceRequest {
	config := map[string]interface{}{
		"type":      SourceTypeGDrive,
		"sync_mode": defaultString(s.SyncMode, "incremental"),
	}
	setNonEmpty(config, "auth_mode", s.AuthMode)
	setNonEmpty(config, "service_account_json", s.ServiceAccountJSON)
	setNonEmpty(config, "delegated_user", s.DelegatedUser)
	if s.OAuthCredentials != nil {
		config["oauth_credentials"] = cloneMap(s.OAuthCredentials)
	}
	setNonEmpty(config, "drive_id", s.DriveID)
	setStringSlice(config, "folder_ids", s.FolderIDs)
	setNonEmpty(config, "query", s.Query)
	setStringSlice(config, "mime_types", s.MimeTypes)
	setBoolPtr(config, "include_shared_drives", s.IncludeSharedDrives)
	setBoolPtr(config, "include_team_drives", s.IncludeTeamDrives)
	setNonZero(config, "page_size", s.PageSize)
	setNonEmpty(config, "resume_cursor", s.ResumeCursor)
	setBoolPtr(config, "fetch_attachments", s.FetchAttachments)
	setBoolPtr(config, "sampling_enabled", s.SamplingEnabled)
	setNonZero(config, "sampling_limit", s.SamplingLimit)
	setNonZero(config, "max_concurrent_downloads", s.MaxConcurrentDownloads)
	mergeExtra(config, s.ConfigExtra)
	return CreateSourceRequest{SourceType: SourceTypeGDrive, Name: s.Name, Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// FileUploadSource models the source record used by the presigned file-upload
// flow. Use IngestFiles for the full local file upload flow.
type FileUploadSource struct {
	Name                string
	StorageProvider     string
	KeyPrefixTemplate   string
	AllowedContentTypes []string
	MaxFileSizeMB       int
	MaxFilesPerJob      int
	SyncMode            string
	Description         string
	Metadata            Metadata
	ConfigExtra         map[string]interface{}
}

func (s FileUploadSource) ToCreateSourceRequest() CreateSourceRequest {
	name := defaultString(s.Name, "go-sdk-file-upload")
	config := map[string]interface{}{
		"type":             SourceTypeFileUpload,
		"storage_provider": defaultString(s.StorageProvider, "s3"),
		"sync_mode":        defaultString(s.SyncMode, "full"),
	}
	setNonEmpty(config, "key_prefix_template", s.KeyPrefixTemplate)
	setStringSlice(config, "allowed_content_types", s.AllowedContentTypes)
	setNonZero(config, "max_file_size_mb", s.MaxFileSizeMB)
	setNonZero(config, "max_files_per_job", s.MaxFilesPerJob)
	mergeExtra(config, s.ConfigExtra)
	return CreateSourceRequest{SourceType: SourceTypeFileUpload, Name: name, Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

func (s *IngestionService) Create(ctx context.Context, source interface{}) (*Source, error) {
	return s.CreateSource(ctx, source)
}

func (s *IngestionService) CreateWeb(ctx context.Context, source WebSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

func (s *IngestionService) CreateS3(ctx context.Context, source S3Source) (*Source, error) {
	return s.CreateSource(ctx, source)
}

func (s *IngestionService) CreateGoogleDrive(ctx context.Context, source GoogleDriveSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

func (s *IngestionService) CreateFileUpload(ctx context.Context, source FileUploadSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

func normalizeCreateSourceRequest(source interface{}) (CreateSourceRequest, bool) {
	switch v := source.(type) {
	case CreateSourceRequest:
		return v, true
	case *CreateSourceRequest:
		if v == nil {
			return CreateSourceRequest{}, false
		}
		return *v, true
	case SourceBuilder:
		return v.ToCreateSourceRequest(), true
	default:
		return CreateSourceRequest{}, false
	}
}

func (s *IngestionService) resolveSourceID(ctx context.Context, source interface{}) (string, error) {
	switch v := source.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("vectoramp: source id is empty")
		}
		return v, nil
	case Source:
		return sourceIdentifier(v)
	case *Source:
		if v == nil {
			return "", fmt.Errorf("vectoramp: source is nil")
		}
		return sourceIdentifier(*v)
	default:
		created, err := s.CreateSource(ctx, source)
		if err != nil {
			return "", err
		}
		return sourceIdentifier(*created)
	}
}

func sourceIdentifier(source Source) (string, error) {
	for _, id := range []string{source.ID, source.SourceID, source.UUID} {
		if id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("vectoramp: source response did not include id, source_id, or uuid")
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func setNonEmpty(m map[string]interface{}, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func setNonZero(m map[string]interface{}, key string, value int) {
	if value != 0 {
		m[key] = value
	}
}

func setBoolPtr(m map[string]interface{}, key string, value *bool) {
	if value != nil {
		m[key] = *value
	}
}

func setStringSlice(m map[string]interface{}, key string, value []string) {
	if value != nil {
		m[key] = append([]string(nil), value...)
	}
}

func mergeExtra(config, extra map[string]interface{}) {
	for k, v := range extra {
		config[k] = v
	}
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMetadata(in Metadata) Metadata {
	if in == nil {
		return nil
	}
	out := make(Metadata, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
