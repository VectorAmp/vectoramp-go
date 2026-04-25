package vectoramp

import (
	"context"
	"fmt"
)

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
func (s *DatasetService) Search(ctx context.Context, datasetID string, req SearchRequest) (*SearchResponse, error) {
	var out SearchResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/datasets/%s/search", datasetID), nil, req, &out)
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
func (s *DatasetService) AddTexts(ctx context.Context, datasetID string, req AddTextsRequest) (*AddTextsResponse, error) {
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

func (s *DatasetService) IngestSource(ctx context.Context, datasetID, sourceID, pipelineID string) (*Job, error) {
	return s.client.Ingestion.StartJob(ctx, StartIngestionRequest{SourceID: sourceID, DatasetID: datasetID, PipelineID: pipelineID})
}

func (s *DatasetService) Ask(ctx context.Context, datasetID string, req AskRequest) (*AskResponse, error) {
	req.DatasetID = datasetID
	return s.client.Intelligence.Ask(ctx, req)
}

func (d *Dataset) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	return d.datasetService().Search(ctx, d.ID, req)
}

func (d *Dataset) Insert(ctx context.Context, vectors []Vector) (*InsertVectorsResponse, error) {
	return d.datasetService().Insert(ctx, d.ID, vectors)
}

func (d *Dataset) AddTexts(ctx context.Context, req AddTextsRequest) (*AddTextsResponse, error) {
	return d.datasetService().AddTexts(ctx, d.ID, req)
}

func (d *Dataset) Delete(ctx context.Context) error {
	return d.datasetService().Delete(ctx, d.ID)
}

func (d *Dataset) Ask(ctx context.Context, req AskRequest) (*AskResponse, error) {
	return d.datasetService().Ask(ctx, d.ID, req)
}

func (d *Dataset) IngestFiles(ctx context.Context, paths []string, opts *IngestFilesOptions) (*Job, error) {
	return d.datasetService().IngestFiles(ctx, d.ID, paths, opts)
}

func (d *Dataset) IngestSource(ctx context.Context, sourceID string, pipelineID ...string) (*Job, error) {
	pipeline := ""
	if len(pipelineID) > 0 {
		pipeline = pipelineID[0]
	}
	return d.datasetService().IngestSource(ctx, d.ID, sourceID, pipeline)
}
