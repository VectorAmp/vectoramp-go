<div align="center">
  <a href="https://vectoramp.com/">
    <picture>
      <source media="(prefers-color-scheme: light)" srcset="https://vectoramp.com/logo-full-light.svg">
      <source media="(prefers-color-scheme: dark)" srcset="https://vectoramp.com/logo-full-dark.svg">
      <img alt="VectorAmp Logo" src="https://vectoramp.com/logo-full-dark.svg" width="50%">
    </picture>
  </a>
</div>

# VectorAmp Go SDK

Idiomatic Go client for the public [VectorAmp](https://vectoramp.com) API.

- Default API base URL: `https://api.vectoramp.com`
- Auth: `X-API-Key: <api_key>`
- REST transport today, with a small transport interface so gRPC can be added later
- Dataset creation always uses SABLE; the SDK intentionally does not expose an index type option

## Install

```bash
go get github.com/vectoramp/vectoramp-go
```

```go
import vectoramp "github.com/vectoramp/vectoramp-go"
```

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    vectoramp "github.com/vectoramp/vectoramp-go"
)

func main() {
    ctx := context.Background()
    client := vectoramp.NewClient(os.Getenv("VECTORAMP_API_KEY"))

    // Only a name is required. Dim is inferred (2560), the embedding defaults to
    // VectorAmp-Embedding-4B, the metric defaults to cosine, and the index is SABLE.
    ds, err := client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{Name: "product-docs"})
    if err != nil {
        log.Fatal(err)
    }

    // Object -> method: operate on the returned dataset directly.
    if _, err := ds.AddTexts(ctx, []string{
        "VectorAmp is a high-performance vector database.",
    }); err != nil {
        log.Fatal(err)
    }

    answer, err := ds.Ask(ctx, "What is VectorAmp?", vectoramp.WithTopK(5))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(answer.Answer)
}
```

## Configure the client

`NewClient` needs only an API key; everything else has a sane default.

```go
client := vectoramp.NewClient(
    os.Getenv("VECTORAMP_API_KEY"),
    vectoramp.WithBaseURL("https://api.vectoramp.com"),
)

// Custom HTTP client:
client = vectoramp.NewClient(apiKey, vectoramp.WithHTTPClient(myHTTPClient))
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

### Create

Only a name is required. `Dim` is inferred from the embedding model, the metric
defaults to `cosine`, the embedding defaults to `vectoramp/VectorAmp-Embedding-4B`,
and the index is always SABLE. `CreateDatasetRequest` has no `IndexType` field —
the SDK always sends `index_type: "sable"`.

```go
// Minimal: name only. Embedding config is omitted so VectorAmp uses
// the managed VectorAmp-Embedding-4B model and infers dim 2560.
ds, err := client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{Name: "docs"})

// Hybrid (dense + sparse) index.
ds, err = client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{Name: "docs", Hybrid: true})

// Optional BYOM: use OpenAI only when you intentionally want that provider
// (dim inferred: small -> 1536, large -> 3072).
ds, err = client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{
    Name:      "openai-docs",
    Embedding: vectoramp.OpenAIEmbedding("large"),
})

// Custom / unknown embedding models require an explicit Dim.
ds, err = client.Datasets.Create(ctx, vectoramp.CreateDatasetRequest{
    Name:      "docs",
    Dim:       768,
    Embedding: &vectoramp.EmbeddingConfig{Provider: "acme", Model: "acme-embed-v1"},
})
```

### List / get / delete

```go
page, err := client.Datasets.List(ctx, 50, 0)
_ = page.Pagination()

ds, err := client.Datasets.Get(ctx, "dataset-id")

err = ds.Delete(ctx) // object -> method
// or: client.Datasets.Delete(ctx, "dataset-id")
```

`Create`, `Get`, and `List` return bound `Dataset` values. Call instance methods
directly (`ds.Search(...)`) or use the service-style form
(`client.Datasets.Search(id, ...)`) — both are supported everywhere.

### Insert vectors

Vector ids accept **strings or integers**. Integer ids serialize as JSON numbers
so the API keeps them as-is; leave the id unset to let the API assign one.

```go
ds, err := client.Datasets.Get(ctx, "dataset-id")
_, err = ds.Insert(ctx, []vectoramp.Vector{
    {ID: vectoramp.StringID("doc-1"), Values: []float64{0.1, 0.2, 0.3}, Metadata: vectoramp.Metadata{"title": "Intro"}},
    {ID: vectoramp.IntID(42), Values: []float64{0.4, 0.5, 0.6}}, // serialized as "id": 42
    {Values: []float64{0.7, 0.8, 0.9}},                          // no id -> API assigns one
})
```

### Add texts

`AddTexts` embeds text through the dataset embedding model and inserts the
resulting vectors. Pass a string or `[]string` for quick inserts; the SDK
generates stable ids (`text-1`, `text-2`, ...). Use `AddTextsRequest` for custom
ids (string or numeric) and metadata. The source text is copied into
`metadata.text` when not already present.

```go
ds, err := client.Datasets.Get(ctx, "dataset-id")
_, err = ds.AddTexts(ctx, []string{"Hello world", "Machine learning notes"})

_, err = ds.AddTexts(ctx, vectoramp.AddTextsRequest{
    Texts: []vectoramp.TextDocument{
        {ID: vectoramp.StringID("doc-1"), Text: "Hello world", Metadata: vectoramp.Metadata{"source": "manual"}},
        {ID: vectoramp.IntID(2), Text: "Machine learning notes"},
    },
})
```

### Search

A search query may be a bare string (text search), a `[]float64` (vector search),
or a full `SearchRequest`. `top_k` defaults to 10. `WithSearchRerank(true)`
expands to the full rerank object.

```go
ds, err := client.Datasets.Get(ctx, "dataset-id")
resp, err := ds.Search(ctx, "machine learning best practices",
    vectoramp.WithSearchTopK(10), vectoramp.WithSearchRerank(true))

includeMetadata := true
resp, err = ds.Search(ctx, vectoramp.SearchRequest{
    QueryText:        "machine learning best practices",
    TopK:             10,
    Filters:          map[string]string{"category": "engineering"},
    IncludeDocuments: true,
    IncludeMetadata:  &includeMetadata,
    Rerank:           vectoramp.RerankConfig{Enabled: true}, // vectoramp / VectorAmp-Rerank-v1
})

// Hybrid search: pass a sparse query and an alpha blend weight.
alpha := 0.5
resp, err = ds.Search(ctx, vectoramp.SearchRequest{
    QueryText:   "vector database",
    Hybrid:      true,
    SparseQuery: "vector database",
    Alpha:       &alpha,
})
```

### Source documents

Datasets expose retained original source documents from ingestion or file upload.
Listing is cursor-based: pass `NextCursor` into the next call. `DownloadDocument`
returns the original bytes and follows redirects.

```go
ds, err := client.Datasets.Get(ctx, "dataset-id")
docs, err := ds.ListDocuments(ctx, vectoramp.DocumentListOptions{Limit: 25, Status: "ready"})
for _, doc := range docs.Documents {
    if doc.DownloadAvailable {
        raw, err := ds.DownloadDocument(ctx, doc.ID)
        _, _ = raw, err
    }
}
if docs.NextCursor != "" {
    next, err := ds.ListDocuments(ctx, vectoramp.DocumentListOptions{Cursor: docs.NextCursor})
    _, _ = next, err
}
```

## Ingestion

### Typed source builders

Typed builders make source creation safer while preserving `CreateSourceRequest`
for fully manual calls. Supported public `source_type` values: `web`, `s3`, `gcs`,
`gdrive`, `jira`, `confluence`, `file_upload`. Use `GenericSource` for custom or
future types.

```go
web, err := client.Sources.CreateWeb(ctx, vectoramp.WebSource{
    StartURLs: []string{"https://docs.example.com"}, // name defaults to web-docs-example-com
    MaxDepth:  2,
})

s3, err := client.Sources.CreateS3(ctx, vectoramp.S3Source{
    Bucket:          "my-bucket", // name defaults to s3-my-bucket
    Prefix:          "docs/",
    AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
    SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
})

confluence, err := client.Sources.CreateConfluence(ctx, vectoramp.ConfluenceSource{
    CloudID:  "your-cloud-id",
    Username: "user@example.com",
    APIToken: os.Getenv("ATLASSIAN_API_TOKEN"),
    Spaces:   []string{"ENG", "DOCS"},
})

jira, err := client.Sources.CreateJira(ctx, vectoramp.JiraSource{CloudID: "your-cloud-id"})

custom, err := client.Sources.Create(ctx, vectoramp.GenericSource{
    SourceType: "custom",
    Name:       "custom-source",
    Config:     map[string]interface{}{"type": "custom"},
})

_, _, _, _, _ = web, s3, confluence, jira, custom
```

### Ingest a source into a dataset

`IngestSource` accepts an existing source ID/`Source` **or** a typed builder.
Passing a builder creates the source first, then starts the ingestion job.

```go
ds, err := client.Datasets.Get(ctx, "dataset-id")

// One-liner: build + create + start job.
job, err := ds.IngestSource(ctx, vectoramp.WebSource{StartURLs: []string{"https://docs.example.com"}})

// Existing source IDs still work.
job, err = ds.IngestSource(ctx, "source-id")

// Service-style:
job, err = client.Ingestion.StartJob(ctx, vectoramp.StartIngestionRequest{
    SourceID: "source-id", DatasetID: ds.ID,
})

jobs, err := client.Ingestion.ListJobs(ctx, ds.ID, 50, 0)
job, err = client.Ingestion.GetJob(ctx, job.JobID)
```

### Filesystem upload ingestion

For local files, the SDK creates a `file_upload` source, initializes presigned
uploads, uploads file bytes, and completes the upload — all behind one call.

```go
ds, err := client.Datasets.Get(ctx, "dataset-id")
job, err := ds.IngestFiles(ctx, []string{"./docs/guide.pdf"}, nil)

// Override optional details only when you need them.
job, err = ds.IngestFiles(ctx, []string{"./docs/guide.pdf"}, &vectoramp.IngestFilesOptions{
    SourceName: "product-docs-upload",
})
```

### Schedules

```go
sch, err := client.Schedules.Create(ctx, vectoramp.CreateScheduleRequest{
    SourceID: "source-id", DatasetID: "dataset-id", Cron: "0 0 * * *",
})
page, err := client.Schedules.List(ctx, 50, 0)
_, err = client.Schedules.Trigger(ctx, sch.ID)
```

## Intelligence / RAG

### Ask (non-streaming)

`ask` defaults to `top_k=5`, includes source citations, and uses all datasets
when unscoped.

```go
answer, err := client.Ask(ctx, "What are the key product features?", vectoramp.WithAllDatasets())

ds, err := client.Datasets.Get(ctx, "dataset-id")
answer, err = ds.Ask(ctx, "What are the key product features?", vectoramp.WithTopK(5))
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

### Sessions

```go
session, err := client.Intelligence.CreateSession(ctx, vectoramp.SessionCreateRequest{Title: "Planning", DatasetID: "dataset-id"})
_, err = client.Intelligence.AppendMessage(ctx, session.ID, vectoramp.SessionMessageCreateRequest{Role: "user", Content: "Summarize the docs"})
messages, err := client.Intelligence.ListMessages(ctx, session.ID, 100)
sessions, err := client.Intelligence.ListSessions(ctx, 50)
```

Intelligence answers return `Sources` and `Chunks`. Inline `[1]` citations refer
to `Sources[0]`; `PreviewRef` is an opaque preview token, not a storage key.

## Errors

Non-2xx responses return `*vectoramp.APIError`.

```go
if err != nil {
    if apiErr, ok := err.(*vectoramp.APIError); ok {
        fmt.Println(apiErr.StatusCode, apiErr.Message)
    }
}
```

## Method reference

All methods take `context.Context` as the first argument (omitted below for
brevity). Both the service-style (`client.Datasets.Search(id, ...)`) and
object-style (`ds.Search(...)`) forms work where listed.

### `client.Datasets` / `*Dataset`

| Method | Required | Optional | Returns |
|---|---|---|---|
| `List(limit, offset int)` | — | limit, offset (0 omits) | `*DatasetList` |
| `Get(id string)` | id | — | `*Dataset` |
| `Create(req CreateDatasetRequest)` | `req.Name` | `Dim` (inferred), `Metric` (cosine), `Hybrid`, `Embedding`, `Tuning`, `Metadata` | `*Dataset` |
| `Delete(id)` / `ds.Delete()` | id | — | `error` |
| `ListDocuments(id, opts)` / `ds.ListDocuments(opts)` | id | `opts.Limit`, `opts.Cursor`, `opts.Status` | `*DatasetDocumentList` |
| `DownloadDocument(id, docID)` / `ds.DownloadDocument(docID)` | id, docID | — | `[]byte` |
| `Search(id, input, opts...)` / `ds.Search(input, opts...)` | id, input (string, `[]float64`, or `SearchRequest`) | `WithSearchTopK` (10), `WithSearchMetadata`, `WithSearchDocuments`, `WithSearchRerank`, `WithSearchRerankConfig` | `*SearchResponse` |
| `Insert(id, vectors)` / `ds.Insert(vectors)` | id, vectors | — | `*InsertVectorsResponse` |
| `Embed(id, req)` / `ds.Embed(req)`* | id, `req.Text` or `req.Texts` | `EmbeddingProvider`, `EmbeddingModel` | `*EmbedResponse` |
| `AddTexts(id, input, opts...)` / `ds.AddTexts(input, opts...)` | id, input (string, `[]string`, `[]TextDocument`, or `AddTextsRequest`) | `WithEmbedding(provider, model)` | `*AddTextsResponse` |
| `IngestSource(id, source, pipelineID...)` / `ds.IngestSource(source, pipelineID...)` | id, source (ID, `Source`, or builder) | pipelineID | `*Job` |
| `IngestFiles(id, paths, opts)` / `ds.IngestFiles(paths, opts)` | id, paths | `opts.SourceName`, `opts.Description`, `opts.PipelineID`, `opts.Metadata` | `*Job` |
| `Ask(id, input, opts...)` / `ds.Ask(input, opts...)` | id, input (string or `AskRequest`) | `WithTopK` (5), `WithSources`, `WithHistory` | `*AskResponse` |

\* `Embed` is available on `*Dataset` via `ds.Embed(...)` as well as `client.Datasets.Embed(id, ...)`.

Vector id helpers: `StringID(s)`, `IntID(n)`, `NewVectorID(any)`. Embedding
helpers: `VectorAmpEmbedding()`, `OpenAIEmbedding("small"|"large")`.

### `client.Sources` / `client.Ingestion`

| Method | Required | Returns |
|---|---|---|
| `CreateSource(source)` / `Create(source)` | source (builder or `CreateSourceRequest`) | `*Source` |
| `CreateWeb`/`CreateS3`/`CreateGCS`/`CreateGoogleDrive`/`CreateJira`/`CreateConfluence`/`CreateFileUpload(source)` | typed builder | `*Source` |
| `ListSources(limit, offset)` | — | `*SourceList` |
| `GetSource(id)` | id | `*Source` |
| `StartJob(req)` | `req.SourceID`, `req.DatasetID` | `*Job` |
| `ListJobs(datasetID, limit, offset)` | — (datasetID optional filter) | `*JobList` |
| `GetJob(id)` | id | `*Job` |
| `RetryJob(id)` | id | `*Job` |
| `IngestFiles(datasetID, paths, opts)` | datasetID, paths | `*Job` |

Source builders: `WebSource`, `S3Source`, `GCSSource`, `GoogleDriveSource`,
`JiraSource`, `ConfluenceSource`, `FileUploadSource`, `GenericSource`.

### `client.Schedules`

`List(limit, offset)`, `Get(id)`, `Create(req)`, `Update(id, req)`, `Delete(id)`,
`Trigger(id)`.

### `client.Intelligence`

`Ask(input, opts...)`, `Stream(req)`, `CreateSession(req)`, `ListSessions(limit)`,
`GetSession(id)`, `AppendMessage(sessionID, req)`, `ListMessages(sessionID, limit)`.
`client.Ask(...)` is a shortcut for `client.Intelligence.Ask(...)`.

## Development

```bash
go build ./...
go test ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## License

Apache License 2.0. See [LICENSE](./LICENSE) and [NOTICE](./NOTICE). Go has no
package manifest license field; the license is declared by these repo-root files.
