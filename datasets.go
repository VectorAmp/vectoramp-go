package vectoramp

import (
	"context"
	"fmt"
)

// defaultSearchTopK is applied when a search request omits TopK.
const defaultSearchTopK = 10

// SearchOption customizes a SearchRequest built from convenience inputs.
type SearchOption func(*SearchRequest)

// WithSearchTopK sets the maximum number of search results. The default is 10.
func WithSearchTopK(k int) SearchOption { return func(r *SearchRequest) { r.TopK = k } }

// WithSearchMetadata controls whether result metadata is returned. The API default is true.
func WithSearchMetadata(include bool) SearchOption {
	return func(r *SearchRequest) { r.IncludeMetadata = &include }
}

// WithSearchDocuments controls whether document payload fields are returned.
func WithSearchDocuments(include bool) SearchOption {
	return func(r *SearchRequest) { r.IncludeDocuments = include }
}

// WithSearchRerank enables or disables VectorAmp reranking for search results.
func WithSearchRerank(enabled bool) SearchOption {
	return func(r *SearchRequest) { r.Rerank = enabled }
}

// WithSearchRerankConfig sets rerank options. Only Enabled is required; provider defaults to vectoramp and model to VectorAmp-Rerank-v1.
func WithSearchRerankConfig(config RerankConfig) SearchOption {
	return func(r *SearchRequest) { r.Rerank = config }
}

// CreateDatasetOption customizes dataset creation.
type CreateDatasetOption func(context.Context, *DatasetService, *CreateDatasetRequest) error

// WithOpenAIAPIKeySecret stores/updates the organization OpenAI API key before
// dataset creation and configures embedding.secret_ref to reference it.
func WithOpenAIAPIKeySecret(apiKey string) CreateDatasetOption {
	return func(ctx context.Context, s *DatasetService, r *CreateDatasetRequest) error {
		if err := s.client.OrgSecrets.PutOpenAIAPIKey(ctx, apiKey); err != nil {
			return err
		}
		if r.Embedding == nil {
			r.Embedding = OpenAIEmbedding("small")
		}
		r.Embedding.Provider = "openai"
		if r.Embedding.Model == "" {
			r.Embedding.Model = "text-embedding-3-small"
		}
		r.Embedding.SecretRef = OpenAIAPIKeySecretRef
		return nil
	}
}

// AddTextsOption customizes an AddTextsRequest built from convenience inputs.
type AddTextsOption func(*AddTextsRequest)

// WithEmbedding selects the embedding provider and model used by AddTexts.
func WithEmbedding(provider, model string) AddTextsOption {
	return func(r *AddTextsRequest) {
		r.EmbeddingProvider = provider
		r.EmbeddingModel = model
	}
}

// DatasetService manages datasets and dataset-scoped operations.
type DatasetService struct{ client *Client }

// List returns datasets using optional limit and offset pagination.
//
// Pass zero for limit or offset to omit that query parameter. The response
// includes datasets plus total, limit, and offset pagination metadata.
func (s *DatasetService) List(ctx context.Context, limit, offset int) (*DatasetList, error) {
	var out DatasetList
	err := s.client.do(ctx, "GET", "/datasets", paginationQuery(limit, offset), nil, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}

// Get returns one dataset by ID.
func (s *DatasetService) Get(ctx context.Context, datasetID string) (*Dataset, error) {
	var out Dataset
	err := s.client.do(ctx, "GET", fmt.Sprintf("/datasets/%s", datasetID), nil, nil, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}

// Create creates a SABLE dataset and returns the created resource.
//
// Only req.Name is required. When omitted, req.Dim is inferred from the
// embedding model (defaulting to vectoramp/VectorAmp-Embedding-4B → 2560),
// req.Metric defaults to cosine, and the embedding defaults to the VectorAmp 4B
// model. Set req.Hybrid to create a hybrid (dense + sparse) index. The SDK
// always sends index_type="sable"; public dataset creation is SABLE-only.
//
// A minimal create needs only a name:
//
//	ds, err := client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{Name: "docs"})
func (s *DatasetService) Create(ctx context.Context, req CreateDatasetRequest, opts ...CreateDatasetOption) (*Dataset, error) {
	for _, opt := range opts {
		if err := opt(ctx, s, &req); err != nil {
			return nil, err
		}
	}
	prepared, err := req.withDefaults()
	if err != nil {
		return nil, err
	}
	var out Dataset
	err = s.client.do(ctx, "POST", "/datasets", nil, prepared, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}

type updateMetadataSchemaRequest struct {
	Schema MetadataSchema `json:"schema"`
	Mode   string         `json:"mode"`
}

// PatchMetadataSchema adds or updates fields while retaining omitted fields.
func (s *DatasetService) PatchMetadataSchema(ctx context.Context, datasetID string, schema MetadataSchema) (*Dataset, error) {
	return s.updateMetadataSchema(ctx, datasetID, schema, "merge")
}

// ReplaceMetadataSchema replaces the complete schema, including with an empty schema.
func (s *DatasetService) ReplaceMetadataSchema(ctx context.Context, datasetID string, schema MetadataSchema) (*Dataset, error) {
	return s.updateMetadataSchema(ctx, datasetID, schema, "replace")
}

func (s *DatasetService) updateMetadataSchema(ctx context.Context, datasetID string, schema MetadataSchema, mode string) (*Dataset, error) {
	var out Dataset
	err := s.client.do(ctx, "PATCH", fmt.Sprintf("/datasets/%s/schema", datasetID), nil, updateMetadataSchemaRequest{Schema: schema, Mode: mode}, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}

// Delete removes a dataset by ID.
func (s *DatasetService) Delete(ctx context.Context, datasetID string) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/datasets/%s", datasetID), nil, nil, nil)
}

// ListDocuments lists retained source documents for a dataset using cursor pagination.
//
// Pass DocumentListOptions.Cursor from the previous response's NextCursor to
// fetch the next page. The API intentionally does not expose offset pagination
// for documents, so callers should not infer totals or offsets.
func (s *DatasetService) ListDocuments(ctx context.Context, datasetID string, opts DocumentListOptions) (*DatasetDocumentList, error) {
	var out DatasetDocumentList
	err := s.client.do(ctx, "GET", fmt.Sprintf("/datasets/%s/documents", datasetID), documentListQuery(opts), nil, &out)
	return &out, err
}

// DownloadDocument downloads the retained original bytes for a dataset document.
//
// The default HTTP transport follows redirects, so this returns the final raw
// object bytes rather than JSON metadata.
func (s *DatasetService) DownloadDocument(ctx context.Context, datasetID, documentID string) ([]byte, error) {
	return s.client.download(ctx, "GET", fmt.Sprintf("/datasets/%s/documents/%s/download", datasetID, documentID), nil)
}

// Search queries a dataset by text, vector, or full SearchRequest.
//
// input may be a string query_text, []float64 query vector, SearchRequest, or
// *SearchRequest. Options override the normalized request. TopK defaults to 10
// when omitted. The response contains ranked results and query timing.
func (s *DatasetService) Search(ctx context.Context, datasetID string, input interface{}, opts ...SearchOption) (*SearchResponse, error) {
	req, err := normalizeSearchRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	var out SearchResponse
	err = s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/search", datasetID), nil, req, &out)
	return &out, err
}

// Insert writes vectors into a dataset and returns the inserted count.
func (s *DatasetService) Insert(ctx context.Context, datasetID string, vectors []Vector) (*InsertVectorsResponse, error) {
	var out InsertVectorsResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/insert", datasetID), nil, InsertVectorsRequest{Vectors: vectors}, &out)
	return &out, err
}

// DeleteVectors removes vectors from a dataset by id.
func (s *DatasetService) DeleteVectors(ctx context.Context, datasetID string, ids []VectorID, writeConcern ...string) (*DeleteVectorsResponse, error) {
	req := DeleteVectorsRequest{IDs: ids}
	if len(writeConcern) > 0 {
		req.WriteConcern = writeConcern[0]
	}
	var out DeleteVectorsResponse
	err := s.client.do(ctx, "DELETE", fmt.Sprintf("/datasets/%s/vectors", datasetID), nil, req, &out)
	return &out, err
}

// Embed creates embeddings for one text or a batch using the dataset context.
func (s *DatasetService) Embed(ctx context.Context, datasetID string, req EmbedRequest) (*EmbedResponse, error) {
	var out EmbedResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/embed", datasetID), nil, req, &out)
	return &out, err
}

// AddTexts embeds text documents, inserts them as vectors, and returns counts.
//
// input may be a string, []string, []TextDocument, AddTextsRequest, or
// *AddTextsRequest. String inputs receive generated IDs text-1, text-2, and so
// on. Text is copied into metadata["text"] when that key is not already set.
func (s *DatasetService) AddTexts(ctx context.Context, datasetID string, input interface{}, opts ...AddTextsOption) (*AddTextsResponse, error) {
	req, err := normalizeAddTextsRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(req.Texts))
	for i, t := range req.Texts {
		texts[i] = t.Text
	}
	emb, err := s.Embed(ctx, datasetID, EmbedRequest{Texts: texts, EmbeddingProvider: req.EmbeddingProvider, EmbeddingModel: req.EmbeddingModel})
	if err != nil {
		return nil, err
	}
	embeddings := emb.Embeddings
	if len(embeddings) == 0 && len(emb.Embedding) > 0 {
		embeddings = [][]float64{emb.Embedding}
	}
	vectors := make([]Vector, len(req.Texts))
	for i, t := range req.Texts {
		md := Metadata{}
		for k, v := range t.Metadata {
			md[k] = v
		}
		if _, ok := md["text"]; !ok {
			md["text"] = t.Text
		}
		var vals []float64
		if i < len(embeddings) {
			vals = embeddings[i]
		}
		id := t.ID
		if id.IsZero() {
			id = StringID(fmt.Sprintf("text-%d", i+1))
		}
		vectors[i] = Vector{ID: id, Values: vals, Metadata: md}
	}
	inserted, err := s.Insert(ctx, datasetID, vectors)
	if err != nil {
		return nil, err
	}
	return &AddTextsResponse{Inserted: inserted.Inserted, Embeddings: len(embeddings)}, nil
}

// IngestFiles uploads local files into a dataset and returns the ingestion job.
//
// It creates a file_upload source automatically, initializes presigned uploads,
// PUTs each file, and completes the upload job. If opts.SourceName is empty, a
// go-sdk-file-upload-<dataset>-<timestamp> name is generated.
func (s *DatasetService) IngestFiles(ctx context.Context, datasetID string, paths []string, opts *IngestFilesOptions) (*Job, error) {
	return s.client.Ingestion.IngestFiles(ctx, datasetID, paths, opts)
}

// IngestSource starts ingestion from an existing or newly created source.
//
// source may be a source ID string, Source, *Source, CreateSourceRequest, or a
// typed SourceBuilder. Non-existing source definitions are created first.
// pipelineID is optional; omit it to let the API select the default pipeline.
func (s *DatasetService) IngestSource(ctx context.Context, datasetID string, source interface{}, pipelineID ...string) (*Job, error) {
	sourceID, err := s.client.Ingestion.resolveSourceID(ctx, source)
	if err != nil {
		return nil, err
	}
	pipeline := ""
	if len(pipelineID) > 0 {
		pipeline = pipelineID[0]
	}
	return s.client.Ingestion.StartJob(ctx, StartIngestionRequest{SourceID: sourceID, DatasetID: datasetID, PipelineID: pipeline})
}

// Ask runs an intelligence query scoped to datasetID.
func (s *DatasetService) Ask(ctx context.Context, datasetID string, input interface{}, opts ...AskOption) (*AskResponse, error) {
	req, err := normalizeAskRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	req.DatasetID = datasetID
	return s.client.Intelligence.Ask(ctx, req)
}

// Search queries this dataset. See DatasetService.Search.
func (d *Dataset) Search(ctx context.Context, input interface{}, opts ...SearchOption) (*SearchResponse, error) {
	return d.datasetService().Search(ctx, d.ID, input, opts...)
}

// ListDocuments lists retained source documents for this dataset.
func (d *Dataset) ListDocuments(ctx context.Context, opts DocumentListOptions) (*DatasetDocumentList, error) {
	return d.datasetService().ListDocuments(ctx, d.ID, opts)
}

// DownloadDocument downloads a retained original source document from this dataset.
func (d *Dataset) DownloadDocument(ctx context.Context, documentID string) ([]byte, error) {
	return d.datasetService().DownloadDocument(ctx, d.ID, documentID)
}

// Insert writes vectors into this dataset. See DatasetService.Insert.
func (d *Dataset) Insert(ctx context.Context, vectors []Vector) (*InsertVectorsResponse, error) {
	return d.datasetService().Insert(ctx, d.ID, vectors)
}

// DeleteVectors removes vectors from this dataset by id. See DatasetService.DeleteVectors.
func (d *Dataset) DeleteVectors(ctx context.Context, ids []VectorID, writeConcern ...string) (*DeleteVectorsResponse, error) {
	return d.datasetService().DeleteVectors(ctx, d.ID, ids, writeConcern...)
}

// AddTexts embeds and inserts texts into this dataset. See DatasetService.AddTexts.
func (d *Dataset) AddTexts(ctx context.Context, input interface{}, opts ...AddTextsOption) (*AddTextsResponse, error) {
	return d.datasetService().AddTexts(ctx, d.ID, input, opts...)
}

// Delete removes this dataset.
func (d *Dataset) Delete(ctx context.Context) error {
	return d.datasetService().Delete(ctx, d.ID)
}

// PatchMetadataSchema adds or updates typed metadata fields on this dataset.
func (d *Dataset) PatchMetadataSchema(ctx context.Context, schema MetadataSchema) (*Dataset, error) {
	return d.datasetService().PatchMetadataSchema(ctx, d.ID, schema)
}

// ReplaceMetadataSchema replaces this dataset's complete typed metadata schema.
func (d *Dataset) ReplaceMetadataSchema(ctx context.Context, schema MetadataSchema) (*Dataset, error) {
	return d.datasetService().ReplaceMetadataSchema(ctx, d.ID, schema)
}

// Ask runs an intelligence query scoped to this dataset.
func (d *Dataset) Ask(ctx context.Context, input interface{}, opts ...AskOption) (*AskResponse, error) {
	return d.datasetService().Ask(ctx, d.ID, input, opts...)
}

// IngestFiles uploads local files into this dataset. See DatasetService.IngestFiles.
func (d *Dataset) IngestFiles(ctx context.Context, paths []string, opts *IngestFilesOptions) (*Job, error) {
	return d.datasetService().IngestFiles(ctx, d.ID, paths, opts)
}

// IngestSource ingests an existing or newly created source into this dataset.
func (d *Dataset) IngestSource(ctx context.Context, source interface{}, pipelineID ...string) (*Job, error) {
	pipeline := ""
	if len(pipelineID) > 0 {
		pipeline = pipelineID[0]
	}
	return d.datasetService().IngestSource(ctx, d.ID, source, pipeline)
}

func normalizeSearchRequest(input interface{}, opts ...SearchOption) (SearchRequest, error) {
	var req SearchRequest
	switch v := input.(type) {
	case SearchRequest:
		req = v
	case *SearchRequest:
		if v == nil {
			return SearchRequest{}, fmt.Errorf("vectoramp: search request is nil")
		}
		req = *v
	case string:
		req.QueryText = v
	case []float64:
		req.Query = append([]float64(nil), v...)
	default:
		return SearchRequest{}, fmt.Errorf("vectoramp: unsupported search input %T", input)
	}
	for _, opt := range opts {
		opt(&req)
	}
	if req.QueryText == "" && req.SearchText != "" {
		req.QueryText = req.SearchText
	}
	req.SearchText = ""
	if req.TopK == 0 {
		req.TopK = defaultSearchTopK
	}
	return req, nil
}

func normalizeAddTextsRequest(input interface{}, opts ...AddTextsOption) (AddTextsRequest, error) {
	var req AddTextsRequest
	switch v := input.(type) {
	case AddTextsRequest:
		req = v
	case *AddTextsRequest:
		if v == nil {
			return AddTextsRequest{}, fmt.Errorf("vectoramp: add texts request is nil")
		}
		req = *v
	case string:
		req.Texts = []TextDocument{{Text: v}}
	case []string:
		req.Texts = make([]TextDocument, len(v))
		for i, text := range v {
			req.Texts[i] = TextDocument{Text: text}
		}
	case []TextDocument:
		req.Texts = append([]TextDocument(nil), v...)
	default:
		return AddTextsRequest{}, fmt.Errorf("vectoramp: unsupported add texts input %T", input)
	}
	for _, opt := range opts {
		opt(&req)
	}
	return req, nil
}
