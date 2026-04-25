# VectorAmp Go SDK

Idiomatic Go client for the public VectorAmp API.

- Default API base URL: `https://api.vectoramp.com`
- Auth: `X-API-Key: <api_key>`
- REST transport today, with a small transport interface so gRPC can be added later
- Dataset creation always uses SABLE; the SDK intentionally does not expose an index type option

> This module is source-ready. It has not been published or tagged yet.

## Install

```bash
go get gitlab.com/VectorAmp/SDK/Go
```

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    vectoramp "gitlab.com/VectorAmp/SDK/Go"
)

func main() {
    ctx := context.Background()
    client := vectoramp.NewClient(os.Getenv("VECTORAMP_API_KEY"))

    ds, err := client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{
        Name:   "product-docs",
        Dim:    2560,
        Metric: "cosine",
        Embedding: &vectoramp.EmbeddingConfig{
            Provider: "vectoramp",
            Model:    "Qwen/Qwen3-Embedding-4B",
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    _, err = client.Datasets.AddTexts(ctx, ds.ID, vectoramp.AddTextsRequest{
        Texts: []vectoramp.TextDocument{
            {ID: "intro", Text: "VectorAmp is a high-performance vector database.", Metadata: vectoramp.Metadata{"section": "intro"}},
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    answer, err := client.Ask(ctx, "What is VectorAmp?", vectoramp.WithDataset(ds.ID), vectoramp.WithTopK(5))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(answer.Answer)
}
```

## Configure the client

```go
client := vectoramp.NewClient(
    os.Getenv("VECTORAMP_API_KEY"),
    vectoramp.WithBaseURL("https://api.vectoramp.com"),
)
```

Custom HTTP client:

```go
client := vectoramp.NewClient(apiKey, vectoramp.WithHTTPClient(myHTTPClient))
```

Custom transport for tests or future protocols:

```go
type MyTransport struct{}
func (t MyTransport) Do(ctx context.Context, req *vectoramp.Request) (*vectoramp.Response, error) {
    // implement transport
}
client := vectoramp.NewClient(apiKey, vectoramp.WithTransport(MyTransport{}))
```

## Datasets

### List / get / create / delete

```go
page, err := client.Datasets.List(ctx, 50, 0)
_ = page.Pagination()

dataset, err := client.Datasets.Get(ctx, "dataset-id")

created, err := client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{
    Name:   "docs",
    Dim:    2560,
    Metric: "cosine",
})

err = client.Datasets.Delete(ctx, created.ID)
```

`CreateDatasetRequest` does not include `IndexType`. The SDK always sends `index_type: "sable"`.

### Insert vectors

```go
_, err := client.Datasets.Insert(ctx, "dataset-id", []vectoramp.Vector{
    {ID: "doc-1", Values: []float64{0.1, 0.2, 0.3}, Metadata: vectoramp.Metadata{"title": "Intro"}},
})
```

### Add texts

`AddTexts` embeds text through the dataset embedding model and inserts the resulting vectors.

```go
_, err := client.Datasets.AddTexts(ctx, "dataset-id", vectoramp.AddTextsRequest{
    Texts: []vectoramp.TextDocument{
        {ID: "doc-1", Text: "Hello world", Metadata: vectoramp.Metadata{"source": "manual"}},
        {ID: "doc-2", Text: "Machine learning notes"},
    },
})
```

### Search

```go
includeMetadata := true
resp, err := client.Datasets.Search(ctx, "dataset-id", vectoramp.SearchRequest{
    QueryText:        "machine learning best practices",
    TopK:             10,
    Filters:          map[string]string{"category": "engineering"},
    IncludeDocuments: true,
    IncludeMetadata:  &includeMetadata,
})
```

Raw vector search is also supported with `Query: []float64{...}`.

## Ingestion

### Sources and jobs

```go
sources, err := client.Ingestion.ListSources(ctx, 50, 0)
source, err := client.Ingestion.GetSource(ctx, sources.Sources[0].ID)

job, err := client.Ingestion.StartJob(ctx, vectoramp.StartIngestionRequest{
    SourceID:  source.ID,
    DatasetID: "dataset-id",
})

jobs, err := client.Ingestion.ListJobs(ctx, "dataset-id", 50, 0)
job, err = client.Ingestion.GetJob(ctx, job.JobID)
```

### Filesystem upload ingestion

For local files, the SDK creates a `file_upload` source, initializes presigned uploads, uploads file bytes, and completes the upload.

```go
job, err := client.Ingestion.IngestFiles(ctx, "dataset-id", []string{"./docs/guide.pdf"}, &vectoramp.IngestFilesOptions{
    SourceName: "product-docs-upload",
})
```

## Intelligence / RAG

### Non-streaming

```go
answer, err := client.Ask(ctx, "What are the key product features?", vectoramp.WithAllDatasets())
```

Equivalent explicit call:

```go
answer, err := client.Intelligence.Ask(ctx, vectoramp.AskRequest{
    Query:     "What are the key product features?",
    DatasetID: "all",
    TopK:      5,
})
```

### Streaming SSE

```go
stream, err := client.Intelligence.Stream(ctx, vectoramp.AskRequest{
    Query:     "Summarize the launch plan",
    DatasetID: "dataset-id",
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for {
    event, ok := stream.Next()
    if !ok {
        break
    }
    if event.ChunkType == "text" {
        fmt.Print(event.Content)
    }
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
```

## Errors

Non-2xx responses return `*vectoramp.APIError`.

```go
if err != nil {
    if apiErr, ok := err.(*vectoramp.APIError); ok {
        fmt.Println(apiErr.StatusCode, apiErr.Message)
    }
}
```

## Development

```bash
go test ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

GitLab CI runs the race-enabled test job and publishes `coverage.out` as an artifact.
