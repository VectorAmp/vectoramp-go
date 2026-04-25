package vectoramp

import "encoding/json"

type Metadata map[string]interface{}

type Pagination struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

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

type EmbeddingConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

type DatasetList struct {
	Datasets []Dataset `json:"datasets"`
	Total    int       `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

func (p DatasetList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

func (p *DatasetList) bind(s *DatasetService) {
	for i := range p.Datasets {
		p.Datasets[i].bind(s)
	}
}

type CreateDatasetRequest struct {
	Name      string                 `json:"name"`
	Dim       int                    `json:"dim"`
	Metric    string                 `json:"metric,omitempty"`
	Tuning    map[string]interface{} `json:"tuning,omitempty"`
	Embedding *EmbeddingConfig       `json:"embedding,omitempty"`
	Metadata  Metadata               `json:"metadata,omitempty"`
}

func (r CreateDatasetRequest) MarshalJSON() ([]byte, error) {
	type alias CreateDatasetRequest
	return json.Marshal(struct {
		alias
		IndexType string `json:"index_type"`
	}{alias: alias(r), IndexType: "sable"})
}

type Vector struct {
	ID       string    `json:"id"`
	Values   []float64 `json:"values"`
	Metadata Metadata  `json:"metadata,omitempty"`
}

type InsertVectorsRequest struct {
	Vectors []Vector `json:"vectors"`
}
type InsertVectorsResponse struct {
	Inserted int `json:"inserted"`
}

type TextDocument struct {
	ID       string
	Text     string
	Metadata Metadata
}
type AddTextsRequest struct {
	Texts             []TextDocument
	EmbeddingProvider string
	EmbeddingModel    string
}
type AddTextsResponse struct {
	Inserted   int `json:"inserted"`
	Embeddings int `json:"embeddings"`
}

type EmbedRequest struct {
	Text              string   `json:"text,omitempty"`
	Texts             []string `json:"texts,omitempty"`
	EmbeddingProvider string   `json:"embedding_provider,omitempty"`
	EmbeddingModel    string   `json:"embedding_model,omitempty"`
}
type EmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Embedding  []float64   `json:"embedding,omitempty"`
}

type SearchRequest struct {
	Query               []float64         `json:"query,omitempty"`
	QueryText           string            `json:"query_text,omitempty"`
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
}

type AdvancedFilter struct {
	Field  string        `json:"field"`
	Op     string        `json:"op"`
	Value  interface{}   `json:"value,omitempty"`
	Values []interface{} `json:"values,omitempty"`
}

type SearchResult struct {
	ID        interface{} `json:"id"`
	Score     float64     `json:"score"`
	Metadata  Metadata    `json:"metadata,omitempty"`
	Embedding []float64   `json:"embedding,omitempty"`
	DocKind   *string     `json:"doc_kind,omitempty"`
	DocValue  *string     `json:"doc_value,omitempty"`
}

type SearchResponse struct {
	Results     []SearchResult `json:"results"`
	DatasetID   string         `json:"dataset_id,omitempty"`
	QueryTimeMS float64        `json:"query_time_ms,omitempty"`
}

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
type SourceList struct {
	Sources []Source `json:"sources"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

func (p SourceList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

type CreateSourceRequest struct {
	SourceType  string                 `json:"source_type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Config      map[string]interface{} `json:"config"`
	Metadata    Metadata               `json:"metadata,omitempty"`
}

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
type JobList struct {
	Jobs   []Job `json:"jobs"`
	Total  int   `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

func (p JobList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

type StartIngestionRequest struct {
	SourceID   string `json:"source_id"`
	DatasetID  string `json:"dataset_id"`
	PipelineID string `json:"pipeline_id,omitempty"`
}

type UploadFile struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type,omitempty"`
}
type InitUploadRequest struct {
	Files []UploadFile `json:"files"`
}
type UploadTarget struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	UploadURL string `json:"upload_url"`
}
type InitUploadResponse struct {
	JobID   string         `json:"job_id"`
	Uploads []UploadTarget `json:"uploads"`
}
type CompleteUploadRequest struct {
	JobID   string   `json:"job_id"`
	FileIDs []string `json:"file_ids"`
}

type AskRequest struct {
	Query               string                `json:"query"`
	DatasetID           interface{}           `json:"dataset_id,omitempty"`
	TopK                int                   `json:"top_k,omitempty"`
	ConversationHistory []ConversationMessage `json:"conversation_history,omitempty"`
	Stream              bool                  `json:"stream"`
	IncludeSources      *bool                 `json:"include_sources,omitempty"`
}
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type AskResponse struct {
	Answer   string           `json:"answer"`
	Sources  []SourceCitation `json:"sources,omitempty"`
	Chunks   []RAGChunk       `json:"chunks,omitempty"`
	Message  *string          `json:"message,omitempty"`
	Metadata Metadata         `json:"metadata,omitempty"`
}
type SourceCitation struct {
	Name         string     `json:"name,omitempty"`
	Path         string     `json:"path,omitempty"`
	URL          *string    `json:"url,omitempty"`
	SourceType   string     `json:"source_type,omitempty"`
	ContentType  string     `json:"content_type,omitempty"`
	Relevance    float64    `json:"relevance,omitempty"`
	Pages        []int      `json:"pages,omitempty"`
	ChunkCount   int        `json:"chunk_count,omitempty"`
	Preview      string     `json:"preview,omitempty"`
	Chunks       []RAGChunk `json:"chunks,omitempty"`
	FileID       string     `json:"file_id,omitempty"`
	ThumbnailURL *string    `json:"thumbnail_url,omitempty"`
}
type RAGChunk struct {
	ID        string      `json:"id,omitempty"`
	ChunkID   string      `json:"chunk_id,omitempty"`
	Text      string      `json:"text,omitempty"`
	Score     float64     `json:"score,omitempty"`
	Source    string      `json:"source,omitempty"`
	SourceURL *string     `json:"source_url,omitempty"`
	Page      interface{} `json:"page,omitempty"`
	Metadata  Metadata    `json:"metadata,omitempty"`
}
type StreamEvent struct {
	Event     string
	ChunkType string   `json:"chunk_type"`
	Content   string   `json:"content"`
	Metadata  Metadata `json:"metadata,omitempty"`
	Raw       []byte
}
