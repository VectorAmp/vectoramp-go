package vectoramp

import (
	"context"
	"fmt"
)

const defaultSearchTopK = 10

type SearchOption func(*SearchRequest)

func WithSearchTopK(k int) SearchOption { return func(r *SearchRequest) { r.TopK = k } }
func WithSearchMetadata(include bool) SearchOption {
	return func(r *SearchRequest) { r.IncludeMetadata = &include }
}
func WithSearchDocuments(include bool) SearchOption {
	return func(r *SearchRequest) { r.IncludeDocuments = include }
}

type AddTextsOption func(*AddTextsRequest)

func WithEmbedding(provider, model string) AddTextsOption {
	return func(r *AddTextsRequest) {
		r.EmbeddingProvider = provider
		r.EmbeddingModel = model
	}
}

type DatasetService struct{ client *Client }

func (s *DatasetService) List(ctx context.Context, limit, offset int) (*DatasetList, error) {
	var out DatasetList
	err := s.client.do(ctx, "GET", "/datasets", paginationQuery(limit, offset), nil, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}
func (s *DatasetService) Get(ctx context.Context, datasetID string) (*Dataset, error) {
	var out Dataset
	err := s.client.do(ctx, "GET", fmt.Sprintf("/datasets/%s", datasetID), nil, nil, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}
func (s *DatasetService) Create(ctx context.Context, req CreateDatasetRequest) (*Dataset, error) {
	var out Dataset
	err := s.client.do(ctx, "POST", "/datasets", nil, req, &out)
	if err == nil {
		out.bind(s)
	}
	return &out, err
}
func (s *DatasetService) Delete(ctx context.Context, datasetID string) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/datasets/%s", datasetID), nil, nil, nil)
}
func (s *DatasetService) Search(ctx context.Context, datasetID string, input interface{}, opts ...SearchOption) (*SearchResponse, error) {
	req, err := normalizeSearchRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	var out SearchResponse
	err = s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/search", datasetID), nil, req, &out)
	return &out, err
}
func (s *DatasetService) Insert(ctx context.Context, datasetID string, vectors []Vector) (*InsertVectorsResponse, error) {
	var out InsertVectorsResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/insert", datasetID), nil, InsertVectorsRequest{Vectors: vectors}, &out)
	return &out, err
}
func (s *DatasetService) Embed(ctx context.Context, datasetID string, req EmbedRequest) (*EmbedResponse, error) {
	var out EmbedResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/embed", datasetID), nil, req, &out)
	return &out, err
}
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
		vectors[i] = Vector{ID: t.ID, Values: vals, Metadata: md}
	}
	inserted, err := s.Insert(ctx, datasetID, vectors)
	if err != nil {
		return nil, err
	}
	return &AddTextsResponse{Inserted: inserted.Inserted, Embeddings: len(embeddings)}, nil
}

func (s *DatasetService) IngestFiles(ctx context.Context, datasetID string, paths []string, opts *IngestFilesOptions) (*Job, error) {
	return s.client.Ingestion.IngestFiles(ctx, datasetID, paths, opts)
}

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

func (s *DatasetService) Ask(ctx context.Context, datasetID string, input interface{}, opts ...AskOption) (*AskResponse, error) {
	req, err := normalizeAskRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	req.DatasetID = datasetID
	return s.client.Intelligence.Ask(ctx, req)
}

func (d *Dataset) Search(ctx context.Context, input interface{}, opts ...SearchOption) (*SearchResponse, error) {
	return d.datasetService().Search(ctx, d.ID, input, opts...)
}

func (d *Dataset) Insert(ctx context.Context, vectors []Vector) (*InsertVectorsResponse, error) {
	return d.datasetService().Insert(ctx, d.ID, vectors)
}

func (d *Dataset) AddTexts(ctx context.Context, input interface{}, opts ...AddTextsOption) (*AddTextsResponse, error) {
	return d.datasetService().AddTexts(ctx, d.ID, input, opts...)
}

func (d *Dataset) Delete(ctx context.Context) error {
	return d.datasetService().Delete(ctx, d.ID)
}

func (d *Dataset) Ask(ctx context.Context, input interface{}, opts ...AskOption) (*AskResponse, error) {
	return d.datasetService().Ask(ctx, d.ID, input, opts...)
}

func (d *Dataset) IngestFiles(ctx context.Context, paths []string, opts *IngestFilesOptions) (*Job, error) {
	return d.datasetService().IngestFiles(ctx, d.ID, paths, opts)
}

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
		req.Texts = []TextDocument{{ID: "text-1", Text: v}}
	case []string:
		req.Texts = make([]TextDocument, len(v))
		for i, text := range v {
			req.Texts[i] = TextDocument{ID: fmt.Sprintf("text-%d", i+1), Text: text}
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
