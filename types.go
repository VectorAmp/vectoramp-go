package vectoramp

import (
	"encoding/json"
	"fmt"
)

// Metadata is arbitrary user or API metadata attached to resources and results.
type Metadata map[string]interface{}

// Pagination summarizes a paginated list response.
type Pagination struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// Dataset is a VectorAmp vector dataset returned by the API.
//
// Dataset resource methods require the value to be returned by a Client so it is
// bound to the originating client. Public dataset creation is SABLE-only; newly
// created datasets always use index_type="sable".
type Dataset struct {
	service *DatasetService `json:"-"`
	client  *Client         `json:"-"`

	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	OrgID     string                 `json:"org_id,omitempty"`
	Dim       int                    `json:"dim"`
	Metric    string                 `json:"metric"`
	Tuning    map[string]interface{} `json:"tuning,omitempty"`
	Embedding *EmbeddingConfig       `json:"embedding,omitempty"`
	IndexType string                 `json:"index_type,omitempty"`
	CreatedAt string                 `json:"created_at,omitempty"`
	UpdatedAt string                 `json:"updated_at,omitempty"`
	Metadata  Metadata               `json:"metadata,omitempty"`
	Raw       json.RawMessage        `json:"-"`
}

// UnmarshalJSON decodes a dataset and preserves the raw response body.
func (d *Dataset) UnmarshalJSON(data []byte) error {
	type alias Dataset
	var out alias
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*d = Dataset(out)
	d.Raw = append(d.Raw[:0], data...)
	return nil
}

func (d *Dataset) bind(s *DatasetService) {
	d.service = s
	if s != nil {
		d.client = s.client
	}
}

func (d *Dataset) datasetService() *DatasetService {
	if d.service != nil {
		return d.service
	}
	if d.client != nil {
		return d.client.Datasets
	}
	panic("vectoramp: Dataset resource is not bound to a Client; get it from Client.Datasets.Create/Get/List before calling resource methods")
}

// EmbeddingConfig selects the embedding provider and model associated with a dataset.
type EmbeddingConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// VectorAmpEmbedding returns the default VectorAmp embedding config (the 4B model,
// dim 2560). Pass it as CreateDatasetRequest.Embedding to be explicit.
func VectorAmpEmbedding() *EmbeddingConfig {
	return &EmbeddingConfig{Provider: DefaultEmbeddingProvider, Model: DefaultEmbeddingModel}
}

// OpenAIEmbedding returns an OpenAI embedding config for dataset creation.
//
// size is "small" (text-embedding-3-small, dim 1536) or "large"
// (text-embedding-3-large, dim 3072). Any other value selects the small model.
// Pass the result as CreateDatasetRequest.Embedding; Dim is inferred from it.
func OpenAIEmbedding(size string) *EmbeddingConfig {
	model := "text-embedding-3-small"
	if size == "large" {
		model = "text-embedding-3-large"
	}
	return &EmbeddingConfig{Provider: "openai", Model: model}
}

// DatasetList is a paginated collection of datasets.
type DatasetList struct {
	Datasets []Dataset `json:"datasets"`
	Total    int       `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// Pagination returns the list pagination metadata as a common struct.
func (p DatasetList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

func (p *DatasetList) bind(s *DatasetService) {
	for i := range p.Datasets {
		p.Datasets[i].bind(s)
	}
}

// Default embedding provider and model used when CreateDatasetRequest omits one.
const (
	// DefaultEmbeddingProvider is the provider used when none is specified.
	DefaultEmbeddingProvider = "vectoramp"
	// DefaultEmbeddingModel is the model used when none is specified.
	DefaultEmbeddingModel = "VectorAmp-Embedding-4B"
	// DefaultMetric is the distance metric used when none is specified.
	DefaultMetric = "cosine"
)

// embeddingDimensions maps known provider/model pairs to their vector dimension
// so CreateDatasetRequest can infer Dim when the caller omits it.
var embeddingDimensions = map[string]map[string]int{
	"vectoramp": {
		"VectorAmp-Embedding-4B": 2560,
	},
	"openai": {
		"text-embedding-3-small": 1536,
		"text-embedding-3-large": 3072,
	},
}

// inferDim returns the known dimension for a provider/model pair, or false.
func inferDim(provider, model string) (int, bool) {
	if models, ok := embeddingDimensions[provider]; ok {
		if dim, ok := models[model]; ok {
			return dim, true
		}
	}
	return 0, false
}

// CreateDatasetRequest is the request body for creating a dataset.
//
// Only Name is required. When omitted, Dim is inferred from the embedding model
// (defaulting to vectoramp/VectorAmp-Embedding-4B → 2560), Metric defaults to
// cosine, and Embedding defaults to the VectorAmp 4B model. Set Hybrid to enable
// a hybrid (dense + sparse) index. Custom or unknown embedding models require an
// explicit Dim. MarshalJSON always adds index_type="sable" because public
// dataset creation is SABLE-only.
type CreateDatasetRequest struct {
	Name      string                 `json:"name"`
	Dim       int                    `json:"dim,omitempty"`
	Metric    string                 `json:"metric,omitempty"`
	Hybrid    bool                   `json:"hybrid,omitempty"`
	Tuning    map[string]interface{} `json:"tuning,omitempty"`
	Embedding *EmbeddingConfig       `json:"embedding,omitempty"`
	Metadata  Metadata               `json:"metadata,omitempty"`
}

// withDefaults returns a copy of the request with SDK defaults applied: a
// default embedding model, a default metric, and an inferred Dim when omitted.
// It returns an error if Dim is omitted and cannot be inferred from the model.
func (r CreateDatasetRequest) withDefaults() (CreateDatasetRequest, error) {
	out := r
	if out.Metric == "" {
		out.Metric = DefaultMetric
	}
	if out.Embedding == nil {
		out.Embedding = &EmbeddingConfig{Provider: DefaultEmbeddingProvider, Model: DefaultEmbeddingModel}
	} else {
		emb := *out.Embedding
		if emb.Provider == "" {
			emb.Provider = DefaultEmbeddingProvider
		}
		if emb.Model == "" {
			emb.Model = DefaultEmbeddingModel
		}
		out.Embedding = &emb
	}
	if out.Dim == 0 {
		dim, ok := inferDim(out.Embedding.Provider, out.Embedding.Model)
		if !ok {
			return out, fmt.Errorf("vectoramp: cannot infer dim for embedding %s/%s; set CreateDatasetRequest.Dim explicitly", out.Embedding.Provider, out.Embedding.Model)
		}
		out.Dim = dim
	}
	return out, nil
}

// MarshalJSON encodes CreateDatasetRequest with index_type="sable".
func (r CreateDatasetRequest) MarshalJSON() ([]byte, error) {
	type alias CreateDatasetRequest
	return json.Marshal(struct {
		alias
		IndexType string `json:"index_type"`
	}{alias: alias(r), IndexType: "sable"})
}

// Vector is one vector record to insert into a dataset.
//
// ID is a VectorID that preserves the caller's string or integer type on the
// wire: integer ids serialize as JSON numbers, string ids as JSON strings.
// Build one with StringID, IntID, or NewVectorID. A zero ID is omitted so the
// API can assign one.
type Vector struct {
	ID       VectorID  `json:"id"`
	Values   []float64 `json:"values"`
	Metadata Metadata  `json:"metadata,omitempty"`
}

// MarshalJSON serializes a Vector, omitting the id field when it is unset so the
// API generates one, and otherwise preserving the numeric/string id type.
func (v Vector) MarshalJSON() ([]byte, error) {
	type alias Vector
	if v.ID.IsZero() {
		return json.Marshal(struct {
			Values   []float64 `json:"values"`
			Metadata Metadata  `json:"metadata,omitempty"`
		}{Values: v.Values, Metadata: v.Metadata})
	}
	return json.Marshal(alias(v))
}

// InsertVectorsRequest is the request body for vector insertion.
type InsertVectorsRequest struct {
	Vectors []Vector `json:"vectors"`
}

// InsertVectorsResponse reports how many vectors were inserted.
type InsertVectorsResponse struct {
	Inserted int `json:"inserted"`
}

// TextDocument is an input document for AddTexts.
//
// ID is optional; leave it zero to have AddTexts generate a stable id. When set,
// it preserves the string or integer id type. Metadata is optional. AddTexts
// copies Text into metadata["text"] unless that key is already present.
type TextDocument struct {
	ID       VectorID
	Text     string
	Metadata Metadata
}

// AddTextsRequest contains documents and optional embedding settings for AddTexts.
type AddTextsRequest struct {
	Texts             []TextDocument
	EmbeddingProvider string
	EmbeddingModel    string
}

// AddTextsResponse reports embedding and insertion counts from AddTexts.
type AddTextsResponse struct {
	Inserted   int `json:"inserted"`
	Embeddings int `json:"embeddings"`
}

// EmbedRequest creates embeddings for Text or Texts.
//
// EmbeddingProvider and EmbeddingModel are optional; omit them to use the API or
// dataset defaults.
type EmbedRequest struct {
	Text              string   `json:"text,omitempty"`
	Texts             []string `json:"texts,omitempty"`
	EmbeddingProvider string   `json:"embedding_provider,omitempty"`
	EmbeddingModel    string   `json:"embedding_model,omitempty"`
}

// EmbedResponse contains either batch embeddings or a single embedding.
type EmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Embedding  []float64   `json:"embedding,omitempty"`
}

// RerankConfig configures VectorAmp search reranking. Only Enabled is required;
// provider defaults to vectoramp and model defaults to VectorAmp-Rerank-v1.
type RerankConfig struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// SearchRequest is the request body for dataset search.
//
// Provide either Query for vector search or QueryText/SearchText for text search. For hybrid indexes, the API uses this single text field for both dense embedding generation and sparse matching when configured. TopK
// defaults to 10 when using Search convenience inputs and left at zero. Filters,
// AdvancedFilters, embedding settings, hybrid fields, and SABLE tuning overrides
// are optional. IncludeMetadata is a pointer so nil preserves the API default
// (true), while false explicitly omits metadata from results. IncludeDocuments
// controls doc_kind/doc_value fields.
type SearchRequest struct {
	Query     []float64 `json:"query,omitempty"`
	QueryText string    `json:"query_text,omitempty"`
	// SearchText is an alias for QueryText used by the public API docs; Search normalizes it to query_text.
	SearchText          string            `json:"-"`
	EmbeddingModel      string            `json:"embedding_model,omitempty"`
	EmbeddingProvider   string            `json:"embedding_provider,omitempty"`
	TopK                int               `json:"top_k"`
	Filters             map[string]string `json:"filters,omitempty"`
	AdvancedFilters     []AdvancedFilter  `json:"advanced_filters,omitempty"`
	NProbeOverride      int               `json:"nprobe_override,omitempty"`
	RerankDepthOverride int               `json:"rerank_depth_override,omitempty"`
	Hybrid              bool              `json:"hybrid,omitempty"`
	SparseQuery         string            `json:"sparse_query,omitempty"`
	Alpha               *float64          `json:"alpha,omitempty"`
	IncludeEmbeddings   bool              `json:"include_embeddings,omitempty"`
	IncludeDocuments    bool              `json:"include_documents,omitempty"`
	IncludeMetadata     *bool             `json:"include_metadata,omitempty"`
	Rerank              interface{}       `json:"rerank,omitempty"`
}

// AdvancedFilter describes one structured metadata filter.
type AdvancedFilter struct {
	Field  string        `json:"field"`
	Op     string        `json:"op"`
	Value  interface{}   `json:"value,omitempty"`
	Values []interface{} `json:"values,omitempty"`
}

// SearchResult is one ranked dataset search result.
type SearchResult struct {
	ID        interface{} `json:"id"`
	Score     float64     `json:"score"`
	Metadata  Metadata    `json:"metadata,omitempty"`
	Embedding []float64   `json:"embedding,omitempty"`
	DocKind   *string     `json:"doc_kind,omitempty"`
	DocValue  *string     `json:"doc_value,omitempty"`
}

// SearchResponse is the result set and timing for a search query.
type SearchResponse struct {
	Results     []SearchResult `json:"results"`
	DatasetID   string         `json:"dataset_id,omitempty"`
	QueryTimeMS float64        `json:"query_time_ms,omitempty"`
}

// Source is an ingestion source returned by the API.
type Source struct {
	ID          string                 `json:"id"`
	SourceID    string                 `json:"source_id,omitempty"`
	UUID        string                 `json:"uuid,omitempty"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type,omitempty"`
	SourceType  string                 `json:"source_type,omitempty"`
	Description string                 `json:"description,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Metadata    Metadata               `json:"metadata,omitempty"`
	CreatedAt   string                 `json:"created_at,omitempty"`
	UpdatedAt   string                 `json:"updated_at,omitempty"`
}

// SourceList is a paginated collection of ingestion sources.
type SourceList struct {
	Sources []Source `json:"sources"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

// Pagination returns the list pagination metadata as a common struct.
func (p SourceList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

// CreateSourceRequest is the request body for creating an ingestion source.
//
// SourceType, Name, and Config are required by the API. Description and Metadata
// are optional. Prefer typed builders for SDK-managed defaults.
type CreateSourceRequest struct {
	SourceType  string                 `json:"source_type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Config      map[string]interface{} `json:"config"`
	Metadata    Metadata               `json:"metadata,omitempty"`
}

// Job is an ingestion job returned by the API.
type Job struct {
	JobID                 string      `json:"job_id"`
	Status                string      `json:"status"`
	Message               string      `json:"message,omitempty"`
	DocumentsProcessed    int         `json:"documents_processed,omitempty"`
	VectorsInserted       int         `json:"vectors_inserted,omitempty"`
	ProcessingTimeSeconds float64     `json:"processing_time_seconds,omitempty"`
	PipelineResult        interface{} `json:"pipeline_result,omitempty"`
	ErrorDetails          interface{} `json:"error_details,omitempty"`
	StartedAt             string      `json:"started_at,omitempty"`
	CompletedAt           *string     `json:"completed_at,omitempty"`
	ProgressPercentage    float64     `json:"progress_percentage,omitempty"`
	CurrentStep           *string     `json:"current_step,omitempty"`
}

// JobList is a paginated collection of ingestion jobs.
type JobList struct {
	Jobs   []Job `json:"jobs"`
	Total  int   `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// Pagination returns the list pagination metadata as a common struct.
func (p JobList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

// StartIngestionRequest starts ingestion from a source into a dataset.
//
// SourceID and DatasetID are required. PipelineID is optional; omit it to use
// the API default pipeline.
type StartIngestionRequest struct {
	SourceID   string `json:"source_id"`
	DatasetID  string `json:"dataset_id"`
	PipelineID string `json:"pipeline_id,omitempty"`
}

// UploadFile describes a local file before requesting a presigned upload target.
type UploadFile struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type,omitempty"`
}

// InitUploadRequest requests presigned upload targets for files.
type InitUploadRequest struct {
	Files []UploadFile `json:"files"`
}

// UploadTarget is one presigned upload destination returned by InitUpload.
type UploadTarget struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	UploadURL string `json:"upload_url"`
}

// InitUploadResponse contains the upload job ID and presigned upload targets.
type InitUploadResponse struct {
	JobID   string         `json:"job_id"`
	Uploads []UploadTarget `json:"uploads"`
}

// CompleteUploadRequest completes a file-upload job after all PUTs finish.
type CompleteUploadRequest struct {
	JobID   string   `json:"job_id"`
	FileIDs []string `json:"file_ids"`
}

// AskRequest is the request body for intelligence queries.
//
// Query is required. DatasetID is optional and may be a dataset ID or "all".
// TopK, ConversationHistory, and IncludeSources are optional. Stream is managed
// by Ask and Stream helpers.
type AskRequest struct {
	Query               string                `json:"query"`
	DatasetID           interface{}           `json:"dataset_id,omitempty"`
	TopK                int                   `json:"top_k,omitempty"`
	ConversationHistory []ConversationMessage `json:"conversation_history,omitempty"`
	Stream              bool                  `json:"stream"`
	IncludeSources      *bool                 `json:"include_sources,omitempty"`
}

// ConversationMessage is one prior chat turn for an intelligence query.
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AskResponse is the non-streaming intelligence response.
type AskResponse struct {
	Answer   string           `json:"answer"`
	Sources  []SourceCitation `json:"sources,omitempty"`
	Chunks   []RAGChunk       `json:"chunks,omitempty"`
	Message  *string          `json:"message,omitempty"`
	Metadata Metadata         `json:"metadata,omitempty"`
}

// SourceCitation describes a source cited by an intelligence answer.
type SourceCitation struct {
	Name              string      `json:"name,omitempty"`
	Path              string      `json:"path,omitempty"`
	URL               *string     `json:"url,omitempty"`
	DatasetID         string      `json:"dataset_id,omitempty"`
	DatasetDocumentID string      `json:"dataset_document_id,omitempty"`
	SourceType        string      `json:"source_type,omitempty"`
	ContentType       string      `json:"content_type,omitempty"`
	Relevance         float64     `json:"relevance,omitempty"`
	Pages             []int       `json:"pages,omitempty"`
	SheetNames        []string    `json:"sheet_names,omitempty"`
	ChunkCount        int         `json:"chunk_count,omitempty"`
	Preview           string      `json:"preview,omitempty"`
	Chunks            []RAGChunk  `json:"chunks,omitempty"`
	TimestampStart    interface{} `json:"timestamp_start,omitempty"`
	TimestampEnd      interface{} `json:"timestamp_end,omitempty"`
	FileID            string      `json:"file_id,omitempty"`
	ThumbnailURL      *string     `json:"thumbnail_url,omitempty"`
	PreviewRef        string      `json:"preview_ref,omitempty"`
}

// RAGChunk is a retrieved chunk used or returned by an intelligence query.
type RAGChunk struct {
	ID          string      `json:"id,omitempty"`
	ChunkID     string      `json:"chunk_id,omitempty"`
	Text        string      `json:"text,omitempty"`
	Score       float64     `json:"score,omitempty"`
	Source      string      `json:"source,omitempty"`
	SourceURL   *string     `json:"source_url,omitempty"`
	Page        interface{} `json:"page,omitempty"`
	Metadata    Metadata    `json:"metadata,omitempty"`
	ChunkIndex  int         `json:"chunk_index,omitempty"`
	SheetName   string      `json:"sheet_name,omitempty"`
	RowStart    int         `json:"row_start,omitempty"`
	RowEnd      int         `json:"row_end,omitempty"`
	ColumnNames []string    `json:"column_names,omitempty"`
}

// SessionCreateRequest is the request body for creating a persistent Intelligence session.
type SessionCreateRequest struct {
	Title       string   `json:"title,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	DatasetID   string   `json:"dataset_id,omitempty"`
	Metadata    Metadata `json:"metadata,omitempty"`
}

// IntelligenceSession is a persistent Intelligence workspace/chat session.
type IntelligenceSession struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	DatasetID   string   `json:"dataset_id,omitempty"`
	Metadata    Metadata `json:"metadata,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

// SessionMessageCreateRequest is the request body for appending a session message.
type SessionMessageCreateRequest struct {
	Role     string   `json:"role"`
	Content  string   `json:"content"`
	Metadata Metadata `json:"metadata,omitempty"`
}

// SessionMessage is one message stored in a persistent Intelligence session.
type SessionMessage struct {
	ID        string   `json:"id"`
	SessionID string   `json:"session_id,omitempty"`
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Metadata  Metadata `json:"metadata,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// SessionList is the API envelope returned by ListSessions.
type SessionList struct {
	Sessions []IntelligenceSession `json:"sessions"`
}

// MessageList is the API envelope returned by ListMessages.
type MessageList struct {
	Messages []SessionMessage `json:"messages"`
}

// StreamEvent is one decoded server-sent event from an intelligence stream.
type StreamEvent struct {
	Event     string
	ChunkType string   `json:"chunk_type"`
	Content   string   `json:"content"`
	Metadata  Metadata `json:"metadata,omitempty"`
	Raw       []byte
}

// DocumentListOptions controls cursor pagination for dataset document listing.
// Cursor comes from a previous response's NextCursor; Status filters by the
// catalog status such as "ready", "processing", or "failed".
type DocumentListOptions struct {
	Limit  int
	Cursor string
	Status string
}

// DatasetDocument is source/original document metadata returned by the dataset
// document catalog. DownloadAvailable indicates whether the retained original
// bytes can be fetched through DownloadDocument.
type DatasetDocument struct {
	ID                string   `json:"id"`
	DatasetID         string   `json:"dataset_id,omitempty"`
	SourceID          string   `json:"source_id,omitempty"`
	SourceType        string   `json:"source_type,omitempty"`
	ExternalID        string   `json:"external_id,omitempty"`
	FileName          string   `json:"file_name,omitempty"`
	MimeType          string   `json:"mime_type,omitempty"`
	SizeBytes         *int64   `json:"size_bytes,omitempty"`
	ContentHash       string   `json:"content_hash,omitempty"`
	Status            string   `json:"status,omitempty"`
	Version           *int     `json:"version,omitempty"`
	ChunkCount        *int     `json:"chunk_count,omitempty"`
	EmbeddingsCount   *int     `json:"embeddings_count,omitempty"`
	DownloadAvailable bool     `json:"download_available,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	Raw               Metadata `json:"-"`
}

// DatasetDocumentList is a cursor-paginated collection of retained source
// documents. Use NextCursor, when non-empty, as DocumentListOptions.Cursor for
// the next request; do not infer pagination from offsets or page length.
type DatasetDocumentList struct {
	Documents  []DatasetDocument `json:"documents"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Limit      int               `json:"limit,omitempty"`
}
