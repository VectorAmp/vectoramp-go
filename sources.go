package vectoramp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

const (
	// SourceTypeS3 identifies Amazon S3 ingestion sources.
	SourceTypeS3 = "s3"
	// SourceTypeWeb identifies web crawler ingestion sources.
	SourceTypeWeb = "web"
	// SourceTypeGCS identifies Google Cloud Storage ingestion sources.
	SourceTypeGCS = "gcs"
	// SourceTypeGDrive identifies Google Drive ingestion sources.
	SourceTypeGDrive = "gdrive"
	// SourceTypeJira identifies Jira ingestion sources.
	SourceTypeJira = "jira"
	// SourceTypeConfluence identifies Confluence ingestion sources.
	SourceTypeConfluence = "confluence"
	// SourceTypeFileUpload identifies presigned file-upload sources.
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

// ToCreateSourceRequest converts GenericSource into a create-source request.
func (s GenericSource) ToCreateSourceRequest() CreateSourceRequest {
	return CreateSourceRequest{SourceType: s.SourceType, Name: s.Name, Description: s.Description, Config: cloneMap(s.Config), Metadata: cloneMetadata(s.Metadata)}
}

// WebSource describes a web crawler ingestion source.
//
// Name defaults to web-<host> from the first StartURLs entry, or
// go-sdk-web-source when no URL is available. Zero-value optional fields are
// omitted from config; ConfigExtra is merged into config last.
type WebSource struct {
	Name             string
	StartURLs        []string
	MaxDepth         int
	MaxPages         int
	AllowedDomains   []string
	RateLimitMS      int
	RespectRobotsTxt *bool
	IncludeAssets    *bool
	MaxAssetsPerPage int
	Selectors        *WebSelectors
	Headers          map[string]string
	Description      string
	Metadata         Metadata
	ConfigExtra      map[string]interface{}
}

// WebSelectors configures CSS selectors for WebSource extraction.
type WebSelectors struct {
	Content string   `json:"content,omitempty"`
	Title   string   `json:"title,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// ToCreateSourceRequest converts WebSource into a create-source request.
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
	setBoolPtr(config, "include_assets", s.IncludeAssets)
	setNonZero(config, "max_assets_per_page", s.MaxAssetsPerPage)
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
	return CreateSourceRequest{SourceType: SourceTypeWeb, Name: defaultString(s.Name, webSourceDefaultName(s.StartURLs)), Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// S3Source describes an Amazon S3 ingestion source.
//
// Name defaults to s3-<bucket>, or go-sdk-s3-source when Bucket is empty.
// SyncMode defaults to incremental. Zero-value optional fields are omitted from
// config; ConfigExtra is merged into config last.
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

// ToCreateSourceRequest converts S3Source into a create-source request.
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
	return CreateSourceRequest{SourceType: SourceTypeS3, Name: defaultString(s.Name, s3SourceDefaultName(s.Bucket)), Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// GCSSource describes a Google Cloud Storage ingestion source.
type GCSSource struct {
	Name            string
	Bucket          string
	Prefix          string
	ProjectID       string
	CredentialsJSON string
	FilePatterns    []string
	MaxFileSizeMB   int
	SyncMode        string
	Description     string
	Metadata        Metadata
	ConfigExtra     map[string]interface{}
}

// ToCreateSourceRequest converts GCSSource into a create-source request.
func (s GCSSource) ToCreateSourceRequest() CreateSourceRequest {
	config := map[string]interface{}{"type": SourceTypeGCS, "bucket": s.Bucket, "sync_mode": defaultString(s.SyncMode, "incremental")}
	setNonEmpty(config, "prefix", s.Prefix)
	setNonEmpty(config, "project_id", s.ProjectID)
	setNonEmpty(config, "credentials_json", s.CredentialsJSON)
	setStringSlice(config, "file_patterns", s.FilePatterns)
	setNonZero(config, "max_file_size_mb", s.MaxFileSizeMB)
	mergeExtra(config, s.ConfigExtra)
	return CreateSourceRequest{SourceType: SourceTypeGCS, Name: defaultString(s.Name, gcsSourceDefaultName(s.Bucket)), Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// GoogleDriveSource describes a Google Drive ingestion source. The public
// source_type is "gdrive".
//
// Name defaults to gdrive-<drive_id>, then gdrive-<first_folder_id>, then
// go-sdk-gdrive-source. SyncMode defaults to incremental. Zero-value optional
// fields are omitted from config; ConfigExtra is merged into config last.
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

// ToCreateSourceRequest converts GoogleDriveSource into a create-source request.
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
	return CreateSourceRequest{SourceType: SourceTypeGDrive, Name: defaultString(s.Name, gdriveSourceDefaultName(s)), Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// FileUploadSource models the source record used by the presigned file-upload
// flow. Use IngestFiles for the full local file upload flow.
//
// Name defaults to go-sdk-file-upload. StorageProvider defaults to s3 and
// SyncMode defaults to full. Zero-value optional fields are omitted from config;
// ConfigExtra is merged into config last.
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

// ToCreateSourceRequest converts FileUploadSource into a create-source request.
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

// JiraSource describes a Jira ingestion source. IncludeComments defaults to true.
type JiraSource struct {
	Name            string
	CloudID         string
	AccessToken     string
	ProjectKeys     []string
	JQL             string
	IncludeComments *bool
	SyncMode        string
	Description     string
	Metadata        Metadata
	ConfigExtra     map[string]interface{}
}

// ToCreateSourceRequest converts JiraSource into a create-source request.
func (s JiraSource) ToCreateSourceRequest() CreateSourceRequest {
	config := map[string]interface{}{"type": SourceTypeJira, "cloud_id": s.CloudID, "include_comments": true, "sync_mode": defaultString(s.SyncMode, "incremental")}
	setNonEmpty(config, "access_token", s.AccessToken)
	setStringSlice(config, "project_keys", s.ProjectKeys)
	setNonEmpty(config, "jql", s.JQL)
	setBoolPtr(config, "include_comments", s.IncludeComments)
	mergeExtra(config, s.ConfigExtra)
	hint := s.CloudID
	if len(s.ProjectKeys) > 0 && s.ProjectKeys[0] != "" {
		hint = s.ProjectKeys[0]
	}
	return CreateSourceRequest{SourceType: SourceTypeJira, Name: defaultString(s.Name, defaultSourceName(SourceTypeJira, hint)), Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// CreateJira creates a Jira ingestion source and returns it.
func (s *IngestionService) CreateJira(ctx context.Context, source JiraSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// ConfluenceSource describes a Confluence ingestion source.
//
// Provide either CloudID (Atlassian OAuth cloud/site id) or BaseURL (for example
// https://company.atlassian.net). AuthMode defaults to basic, which uses
// Username plus APIToken; use OAuthCredentials for OAuth. Spaces selects
// specific space keys (empty means all accessible). IncludeAttachments defaults
// to false. SyncMode defaults to incremental. ConfigExtra is merged last.
type ConfluenceSource struct {
	Name               string
	CloudID            string
	BaseURL            string
	AuthMode           string
	Username           string
	APIToken           string
	OAuthCredentials   map[string]interface{}
	Spaces             []string
	IncludeAttachments *bool
	SyncMode           string
	Description        string
	Metadata           Metadata
	ConfigExtra        map[string]interface{}
}

// ToCreateSourceRequest converts ConfluenceSource into a create-source request.
func (s ConfluenceSource) ToCreateSourceRequest() CreateSourceRequest {
	config := map[string]interface{}{
		"type":      SourceTypeConfluence,
		"auth_mode": defaultString(s.AuthMode, "basic"),
		"sync_mode": defaultString(s.SyncMode, "incremental"),
	}
	setNonEmpty(config, "cloud_id", s.CloudID)
	setNonEmpty(config, "base_url", s.BaseURL)
	setNonEmpty(config, "username", s.Username)
	setNonEmpty(config, "api_token", s.APIToken)
	if s.OAuthCredentials != nil {
		config["oauth_credentials"] = cloneMap(s.OAuthCredentials)
	}
	setStringSlice(config, "spaces", s.Spaces)
	setBoolPtr(config, "include_attachments", s.IncludeAttachments)
	mergeExtra(config, s.ConfigExtra)
	hint := s.CloudID
	if hint == "" && len(s.Spaces) > 0 && s.Spaces[0] != "" {
		hint = s.Spaces[0]
	}
	if hint == "" {
		hint = s.BaseURL
	}
	return CreateSourceRequest{SourceType: SourceTypeConfluence, Name: defaultString(s.Name, defaultSourceName(SourceTypeConfluence, hint)), Description: s.Description, Config: config, Metadata: cloneMetadata(s.Metadata)}
}

// CreateConfluence creates a Confluence ingestion source and returns it.
func (s *IngestionService) CreateConfluence(ctx context.Context, source ConfluenceSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// Create is an alias for CreateSource.
func (s *IngestionService) Create(ctx context.Context, source interface{}) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// CreateWeb creates a web ingestion source and returns it.
func (s *IngestionService) CreateWeb(ctx context.Context, source WebSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// CreateS3 creates an S3 ingestion source and returns it.
func (s *IngestionService) CreateS3(ctx context.Context, source S3Source) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// CreateGCS creates a Google Cloud Storage ingestion source and returns it.
func (s *IngestionService) CreateGCS(ctx context.Context, source GCSSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// CreateGoogleDrive creates a Google Drive ingestion source and returns it.
func (s *IngestionService) CreateGoogleDrive(ctx context.Context, source GoogleDriveSource) (*Source, error) {
	return s.CreateSource(ctx, source)
}

// CreateFileUpload creates a file_upload source and returns it.
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

func webSourceDefaultName(startURLs []string) string {
	if len(startURLs) == 0 || startURLs[0] == "" {
		return "go-sdk-web-source"
	}
	parsed, err := url.Parse(startURLs[0])
	if err == nil && parsed.Hostname() != "" {
		return "web-" + sanitizeName(parsed.Hostname())
	}
	return "web-" + sanitizeName(startURLs[0])
}

func s3SourceDefaultName(bucket string) string {
	if bucket == "" {
		return "go-sdk-s3-source"
	}
	return "s3-" + sanitizeName(bucket)
}

func gcsSourceDefaultName(bucket string) string {
	if bucket == "" {
		return "go-sdk-gcs-source"
	}
	return defaultSourceName(SourceTypeGCS, bucket)
}

func defaultSourceName(sourceType, hint string) string {
	if hint == "" {
		return sourceType
	}
	return sourceType + "-" + sanitizeName(hint)
}

func gdriveSourceDefaultName(source GoogleDriveSource) string {
	if source.DriveID != "" {
		return "gdrive-" + sanitizeName(source.DriveID)
	}
	if len(source.FolderIDs) > 0 && source.FolderIDs[0] != "" {
		return "gdrive-" + sanitizeName(source.FolderIDs[0])
	}
	return "go-sdk-gdrive-source"
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		keep := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if keep {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "source"
	}
	return out
}
