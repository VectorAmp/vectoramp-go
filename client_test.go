package vectoramp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return NewClient("test-key", WithBaseURL(ts.URL), WithHTTPClient(ts.Client()))
}

func decodeBody(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return got
}

func TestClientDefaultsAndAPIError(t *testing.T) {
	c := NewClient("secret")
	if c.transport.BaseURL.String() != DefaultBaseURL {
		t.Fatalf("base URL = %s", c.transport.BaseURL.String())
	}

	c = testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing api key header: %q", r.Header.Get("X-API-Key"))
		}
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing user agent")
		}
		http.Error(w, `{"error":"nope"}`, http.StatusTeapot)
	}))
	_, err := c.Datasets.List(context.Background(), 0, 0)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusTeapot || !strings.Contains(apiErr.Error(), "nope") {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDatasetListCreateGetDeleteSearchInsertAndAddTexts(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/datasets":
			seen["list"] = true
			if r.URL.Query().Get("limit") != "10" || r.URL.Query().Get("offset") != "20" {
				t.Fatalf("bad pagination query: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"datasets":[{"id":"ds1","name":"docs","dim":3,"metric":"cosine","index_type":"sable"}],"total":1,"limit":10,"offset":20}`))
		case r.Method == "POST" && r.URL.Path == "/datasets":
			seen["create"] = true
			body := decodeBody(t, r)
			if body["index_type"] != "sable" {
				t.Fatalf("dataset create did not force SABLE: %#v", body)
			}
			if _, exposed := reflect.TypeOf(CreateDatasetRequest{}).FieldByName("IndexType"); exposed {
				t.Fatal("CreateDatasetRequest exposes IndexType")
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"ds1","name":"docs","dim":3,"metric":"cosine","index_type":"sable"}`))
		case r.Method == "GET" && r.URL.Path == "/datasets/ds1":
			seen["get"] = true
			w.Write([]byte(`{"id":"ds1","name":"docs","dim":3,"metric":"cosine"}`))
		case r.Method == "DELETE" && r.URL.Path == "/datasets/ds1":
			seen["delete"] = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/search":
			seen["search"] = true
			body := decodeBody(t, r)
			if body["query_text"] != "hello" || body["top_k"].(float64) != 5 || body["include_metadata"] != false || body["rerank"] != true {
				t.Fatalf("bad search body: %#v", body)
			}
			w.Write([]byte(`{"results":[{"id":123,"score":0.9,"metadata":{"title":"A"}}],"dataset_id":"ds1","query_time_ms":1.5}`))
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/embed":
			seen["embed"] = true
			body := decodeBody(t, r)
			if !reflect.DeepEqual(body["texts"], []interface{}{"one", "two"}) {
				t.Fatalf("bad embed body: %#v", body)
			}
			w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3],[0.4,0.5,0.6]]}`))
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/insert":
			seen["insert"] = true
			body := decodeBody(t, r)
			vectors := body["vectors"].([]interface{})
			first := vectors[0].(map[string]interface{})
			if first["id"] != "a" || first["metadata"].(map[string]interface{})["text"] != "one" {
				t.Fatalf("bad insert body: %#v", body)
			}
			w.Write([]byte(`{"inserted":2}`))
		case r.Method == "POST" && r.URL.Path == "/intelligence/query":
			seen["ask"] = true
			body := decodeBody(t, r)
			if body["dataset_id"] != "ds1" || body["query"] != "why" {
				t.Fatalf("bad ask body: %#v", body)
			}
			w.Write([]byte(`{"answer":"because"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/jobs":
			seen["ingestSource"] = true
			body := decodeBody(t, r)
			if body["dataset_id"] != "ds1" || body["source_id"] != "src1" || body["pipeline_id"] != "pipe1" {
				t.Fatalf("bad ingest source body: %#v", body)
			}
			w.Write([]byte(`{"job_id":"job1","status":"pending"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	list, err := c.Datasets.List(context.Background(), 10, 20)
	if err != nil || list.Total != 1 || list.Pagination().Offset != 20 || len(list.Datasets[0].Raw) == 0 {
		t.Fatalf("list: %#v %v", list, err)
	}
	created, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{Name: "docs", Dim: 3, Metric: "cosine"})
	if err != nil || created.IndexType != "sable" {
		t.Fatalf("create: %#v %v", created, err)
	}
	got, err := c.Datasets.Get(context.Background(), "ds1")
	if err != nil || got.ID != "ds1" || len(got.Raw) == 0 {
		t.Fatalf("get: %#v %v", got, err)
	}
	includeMetadata := false
	search, err := c.Datasets.Search(context.Background(), "ds1", SearchRequest{QueryText: "hello", TopK: 5, IncludeMetadata: &includeMetadata, Rerank: true})
	if err != nil || len(search.Results) != 1 {
		t.Fatalf("search: %#v %v", search, err)
	}
	resourceSearch, err := created.Search(context.Background(), SearchRequest{QueryText: "hello", TopK: 5, IncludeMetadata: &includeMetadata, Rerank: true})
	if err != nil || len(resourceSearch.Results) != 1 {
		t.Fatalf("resource search: %#v %v", resourceSearch, err)
	}
	if answer, err := got.Ask(context.Background(), AskRequest{Query: "why"}); err != nil || answer.Answer != "because" {
		t.Fatalf("resource ask: %#v %v", answer, err)
	}
	if job, err := got.IngestSource(context.Background(), "src1", "pipe1"); err != nil || job.JobID != "job1" {
		t.Fatalf("resource ingest source: %#v %v", job, err)
	}
	add, err := created.AddTexts(context.Background(), AddTextsRequest{Texts: []TextDocument{{ID: StringID("a"), Text: "one"}, {ID: StringID("b"), Text: "two", Metadata: Metadata{"kind": "note"}}}})
	if err != nil || add.Inserted != 2 || add.Embeddings != 2 {
		t.Fatalf("add texts: %#v %v", add, err)
	}
	if err := created.Delete(context.Background()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, k := range []string{"list", "create", "get", "search", "embed", "insert", "delete", "ask", "ingestSource"} {
		if !seen[k] {
			t.Fatalf("did not see %s", k)
		}
	}
}

func TestDatasetDocumentsListAndDownload(t *testing.T) {
	bytes := []byte{0x00, 0x01, 0xff, 'V', 'A'}
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/raw/doc.bin" {
			t.Fatalf("unexpected raw download path: %s", r.URL.Path)
		}
		w.Write(bytes)
	}))
	defer fileServer.Close()

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/datasets/ds1/documents":
			if r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("cursor") != "cur1" || r.URL.Query().Get("status") != "ready" {
				t.Fatalf("bad document cursor query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"documents":[{"id":"doc1","file_name":"a.pdf","status":"ready","download_available":true}],"next_cursor":"cur2","limit":2}`))
		case r.Method == "GET" && r.URL.Path == "/datasets/ds1/documents/doc1/download":
			http.Redirect(w, r, fileServer.URL+"/raw/doc.bin", http.StatusFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	page, err := c.Datasets.ListDocuments(context.Background(), "ds1", DocumentListOptions{Limit: 2, Cursor: "cur1", Status: "ready"})
	if err != nil || page.NextCursor != "cur2" || len(page.Documents) != 1 || !page.Documents[0].DownloadAvailable {
		t.Fatalf("list documents: %#v %v", page, err)
	}
	got, err := c.Datasets.DownloadDocument(context.Background(), "ds1", "doc1")
	if err != nil || !reflect.DeepEqual(got, bytes) {
		t.Fatalf("download bytes = %#v err=%v", got, err)
	}
}

func TestIngestionSourcesJobsAndFilesystemUpload(t *testing.T) {
	uploadHit := false
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadHit = true
		if r.Method != "PUT" || r.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("bad upload request %s %q", r.Method, r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadServer.Close()

	tmp := t.TempDir() + "/doc.txt"
	if err := osWriteFile(tmp, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/ingestion/sources":
			w.Write([]byte(`{"sources":[{"id":"src1","name":"web","type":"web"}],"total":1,"limit":2,"offset":1}`))
		case r.Method == "GET" && r.URL.Path == "/ingestion/sources/src1":
			w.Write([]byte(`{"id":"src1","name":"web","type":"web"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources":
			body := decodeBody(t, r)
			if body["source_type"] != "file_upload" {
				t.Fatalf("bad source body: %#v", body)
			}
			name, _ := body["name"].(string)
			if !strings.HasPrefix(name, "go-sdk-file-upload-ds1-") {
				t.Fatalf("file upload source name should be generated from dataset id, got %#v body=%#v", name, body)
			}
			w.Write([]byte(`{"source_id":"src1","name":"upload","source_type":"file_upload"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources/src1/upload/init":
			w.Write([]byte(`{"job_id":"job1","uploads":[{"file_id":"file1","file_name":"doc.txt","upload_url":"` + uploadServer.URL + `"}]}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources/src1/upload/complete":
			w.Write([]byte(`{"job_id":"job1","status":"pending"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/jobs":
			w.Write([]byte(`{"job_id":"job2","status":"pending"}`))
		case r.Method == "GET" && r.URL.Path == "/ingestion/jobs":
			if r.URL.Query().Get("dataset_id") != "ds1" {
				t.Fatalf("missing dataset filter: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"jobs":[{"job_id":"job2","status":"completed"}],"total":1,"limit":5,"offset":0}`))
		case r.Method == "GET" && r.URL.Path == "/ingestion/jobs/job2":
			w.Write([]byte(`{"job_id":"job2","status":"completed"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/jobs/job2/retry":
			w.Write([]byte(`{"job_id":"job3","status":"pending"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	if sources, err := c.Ingestion.ListSources(context.Background(), 2, 1); err != nil || sources.Total != 1 || sources.Pagination().Limit != 2 {
		t.Fatalf("list sources: %#v %v", sources, err)
	}
	if _, err := c.Ingestion.GetSource(context.Background(), "src1"); err != nil {
		t.Fatalf("get source: %v", err)
	}
	if job, err := c.Ingestion.StartJob(context.Background(), StartIngestionRequest{SourceID: "src1", DatasetID: "ds1"}); err != nil || job.JobID != "job2" {
		t.Fatalf("start job: %#v %v", job, err)
	}
	if jobs, err := c.Ingestion.ListJobs(context.Background(), "ds1", 5, 0); err != nil || jobs.Total != 1 {
		t.Fatalf("list jobs: %#v %v", jobs, err)
	}
	if _, err := c.Ingestion.GetJob(context.Background(), "job2"); err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job, err := c.Ingestion.RetryJob(context.Background(), "job2"); err != nil || job.JobID != "job3" {
		t.Fatalf("retry job: %#v %v", job, err)
	}
	if job, err := c.Ingestion.IngestFiles(context.Background(), "ds1", []string{tmp}, nil); err != nil || job.JobID != "job1" || !uploadHit {
		t.Fatalf("ingest files: %#v upload=%v err=%v", job, uploadHit, err)
	}
}

func TestSearchTextAliasNormalizesToQueryText(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/datasets/ds1/search" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		body := decodeBody(t, r)
		if body["query_text"] != "rare zebra quokka" || body["top_k"].(float64) != 3 {
			t.Fatalf("bad search_text alias body: %#v", body)
		}
		if _, ok := body["sparse_query"]; ok {
			t.Fatalf("client should not require sparse_query for easy hybrid search: %#v", body)
		}
		w.Write([]byte(`{"results":[]}`))
	}))

	if _, err := c.Datasets.Search(context.Background(), "ds1", SearchRequest{SearchText: "rare zebra quokka"}, WithSearchTopK(3)); err != nil {
		t.Fatalf("search: %v", err)
	}
}

func TestMinimalConvenienceInputs(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/search":
			seen["search"] = true
			body := decodeBody(t, r)
			if body["query_text"] != "hello" || body["top_k"].(float64) != 3 || body["include_metadata"] != false || body["rerank"].(map[string]interface{})["enabled"] != true {
				t.Fatalf("bad convenience search body: %#v", body)
			}
			w.Write([]byte(`{"results":[]}`))
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/embed":
			seen["embed"] = true
			body := decodeBody(t, r)
			if !reflect.DeepEqual(body["texts"], []interface{}{"one", "two"}) || body["embedding_provider"] != "openai" || body["embedding_model"] != "text-embedding-3-small" {
				t.Fatalf("bad convenience embed body: %#v", body)
			}
			w.Write([]byte(`{"embeddings":[[0.1],[0.2]]}`))
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/insert":
			seen["insert"] = true
			body := decodeBody(t, r)
			vectors := body["vectors"].([]interface{})
			first := vectors[0].(map[string]interface{})
			if first["id"] != "text-1" || first["metadata"].(map[string]interface{})["text"] != "one" {
				t.Fatalf("bad convenience insert body: %#v", body)
			}
			w.Write([]byte(`{"inserted":2}`))
		case r.Method == "POST" && r.URL.Path == "/intelligence/query":
			seen["ask"] = true
			body := decodeBody(t, r)
			if body["dataset_id"] != "ds1" || body["query"] != "why" || body["top_k"].(float64) != 4 {
				t.Fatalf("bad convenience ask body: %#v", body)
			}
			w.Write([]byte(`{"answer":"because"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	ds := &Dataset{ID: "ds1", client: c}
	if _, err := ds.Search(context.Background(), "hello", WithSearchTopK(3), WithSearchMetadata(false), WithSearchRerankConfig(RerankConfig{Enabled: true})); err != nil {
		t.Fatalf("convenience search: %v", err)
	}
	if _, err := ds.AddTexts(context.Background(), []string{"one", "two"}, WithEmbedding("openai", "text-embedding-3-small")); err != nil {
		t.Fatalf("convenience add texts: %v", err)
	}
	if answer, err := ds.Ask(context.Background(), "why", WithTopK(4)); err != nil || answer.Answer != "because" {
		t.Fatalf("convenience ask: %#v %v", answer, err)
	}
	for _, k := range []string{"search", "embed", "insert", "ask"} {
		if !seen[k] {
			t.Fatalf("did not see %s", k)
		}
	}
}

func TestIntelligenceAskAndStream(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/intelligence/query" || r.Method != "POST" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if stream, _ := body["stream"].(bool); stream {
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Fatalf("missing SSE accept: %q", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("event: message\ndata: {\"chunk_type\":\"text\",\"content\":\"hi\"}\n\n"))
			w.Write([]byte("data: {\"chunk_type\":\"done\",\"content\":\"\"}\n\n"))
			return
		}
		if body["query"] != "what" || body["dataset_id"] != "all" {
			t.Fatalf("bad ask body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"answer":"42","metadata":{"model":"test"}}`))
	}))

	answer, err := c.Ask(context.Background(), "what", WithAllDatasets(), WithTopK(3))
	if err != nil || answer.Answer != "42" {
		t.Fatalf("ask: %#v %v", answer, err)
	}
	stream, err := c.Intelligence.Stream(context.Background(), AskRequest{Query: "stream"})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer stream.Close()
	first, ok := stream.Next()
	if !ok || first.Event != "message" || first.ChunkType != "text" || first.Content != "hi" {
		t.Fatalf("first event: %#v ok=%v", first, ok)
	}
	second, ok := stream.Next()
	if !ok || second.ChunkType != "done" {
		t.Fatalf("second event: %#v ok=%v", second, ok)
	}
	if _, ok := stream.Next(); ok || stream.Err() != nil {
		t.Fatalf("unexpected extra event/err: %v", stream.Err())
	}
}

func TestCustomTransport(t *testing.T) {
	tr := roundTripFunc(func(ctx context.Context, req *Request) (*Response, error) {
		if req.Path != "/datasets" {
			t.Fatalf("bad path: %s", req.Path)
		}
		return &Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: ioNopCloser(`{"datasets":[],"total":0,"limit":0,"offset":0}`)}, nil
	})
	c := NewClient("", WithTransport(tr))
	if _, err := c.Datasets.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("custom transport: %v", err)
	}
}

type roundTripFunc func(context.Context, *Request) (*Response, error)

func (f roundTripFunc) Do(ctx context.Context, req *Request) (*Response, error) { return f(ctx, req) }

type stringReadCloser struct{ *strings.Reader }

func (s stringReadCloser) Close() error          { return nil }
func ioNopCloser(s string) stringReadCloser      { return stringReadCloser{strings.NewReader(s)} }
func osWriteFile(name string, data []byte) error { return os.WriteFile(name, data, 0600) }

func TestTypedSourceBuildersAndHelpers(t *testing.T) {
	createdBodies := []map[string]interface{}{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources":
			body := decodeBody(t, r)
			createdBodies = append(createdBodies, body)
			w.Write([]byte(`{"id":"src` + string(rune('1'+len(createdBodies)-1)) + `","name":"created"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/jobs":
			body := decodeBody(t, r)
			if body["dataset_id"] != "ds1" || body["source_id"] == "" || body["pipeline_id"] != "pipe1" {
				t.Fatalf("bad typed ingest source job body: %#v", body)
			}
			w.Write([]byte(`{"job_id":"job1","status":"pending"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	robots := false
	if _, err := c.Sources.CreateWeb(context.Background(), WebSource{
		StartURLs:        []string{"https://example.com/docs"},
		MaxDepth:         2,
		AllowedDomains:   []string{"example.com"},
		RespectRobotsTxt: &robots,
		ConfigExtra:      map[string]interface{}{"max_pages": 25},
	}); err != nil {
		t.Fatalf("create web: %v", err)
	}
	if _, err := c.Sources.CreateS3(context.Background(), S3Source{
		Bucket:          "docs",
		Region:          "us-west-2",
		AccessKeyID:     "AKIA",
		SecretAccessKey: "secret",
		FilePatterns:    []string{"*.pdf"},
	}); err != nil {
		t.Fatalf("create s3: %v", err)
	}
	if _, err := c.Sources.CreateGoogleDrive(context.Background(), GoogleDriveSource{
		AuthMode:           "service_account",
		ServiceAccountJSON: `{"client_email":"svc@example.com"}`,
		FolderIDs:          []string{"folder1"},
	}); err != nil {
		t.Fatalf("create gdrive: %v", err)
	}
	if _, err := c.Sources.CreateFileUpload(context.Background(), FileUploadSource{Name: "upload", MaxFilesPerJob: 5}); err != nil {
		t.Fatalf("create file upload: %v", err)
	}
	if _, err := c.Sources.Create(context.Background(), GenericSource{SourceType: "custom", Name: "custom", Config: map[string]interface{}{"type": "custom"}}); err != nil {
		t.Fatalf("create generic: %v", err)
	}
	ds := &Dataset{ID: "ds1", client: c}
	if job, err := ds.IngestSource(context.Background(), WebSource{Name: "typed-web", StartURLs: []string{"https://example.com"}}, "pipe1"); err != nil || job.JobID != "job1" {
		t.Fatalf("typed ingest source: %#v %v", job, err)
	}

	wantTypes := []string{"web", "s3", "gdrive", "file_upload", "custom", "web"}
	if len(createdBodies) != len(wantTypes) {
		t.Fatalf("created %d sources, want %d: %#v", len(createdBodies), len(wantTypes), createdBodies)
	}
	for i, want := range wantTypes {
		if createdBodies[i]["source_type"] != want {
			t.Fatalf("source %d type = %v, want %s body=%#v", i, createdBodies[i]["source_type"], want, createdBodies[i])
		}
	}
	if createdBodies[0]["name"] != "web-example-com" || createdBodies[1]["name"] != "s3-docs" || createdBodies[2]["name"] != "gdrive-folder1" {
		t.Fatalf("bad default names: %#v", createdBodies)
	}
	webConfig := createdBodies[0]["config"].(map[string]interface{})
	if webConfig["type"] != "web" || webConfig["max_pages"].(float64) != 25 || webConfig["respect_robots_txt"] != false {
		t.Fatalf("bad web config: %#v", webConfig)
	}
	s3Config := createdBodies[1]["config"].(map[string]interface{})
	if s3Config["type"] != "s3" || s3Config["bucket"] != "docs" || s3Config["sync_mode"] != "incremental" {
		t.Fatalf("bad s3 config: %#v", s3Config)
	}
	gdriveConfig := createdBodies[2]["config"].(map[string]interface{})
	if gdriveConfig["type"] != "gdrive" || gdriveConfig["service_account_json"] == "" {
		t.Fatalf("bad gdrive config: %#v", gdriveConfig)
	}
	fileConfig := createdBodies[3]["config"].(map[string]interface{})
	if fileConfig["type"] != "file_upload" || fileConfig["storage_provider"] != "s3" || fileConfig["max_files_per_job"].(float64) != 5 {
		t.Fatalf("bad file upload config: %#v", fileConfig)
	}
}

func TestSchedulesCRUDAndTrigger(t *testing.T) {
	var lastBody map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/ingestion/schedules":
			if r.URL.Query().Get("limit") != "10" {
				t.Fatalf("missing limit: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"schedules":[{"id":"sch_1","cron":"0 * * * *","enabled":true}],"total":1,"limit":10,"offset":0}`))
		case r.Method == "GET" && r.URL.Path == "/ingestion/schedules/sch_1":
			w.Write([]byte(`{"id":"sch_1","cron":"0 * * * *","enabled":true}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/schedules":
			lastBody = decodeBody(t, r)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"sch_2","cron":"0 0 * * *","enabled":true}`))
		case r.Method == "PATCH" && r.URL.Path == "/ingestion/schedules/sch_2":
			lastBody = decodeBody(t, r)
			w.Write([]byte(`{"id":"sch_2","cron":"0 0 * * *","enabled":false}`))
		case r.Method == "DELETE" && r.URL.Path == "/ingestion/schedules/sch_2":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"deleted":true}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/schedules/sch_1/trigger":
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"job_id":"job_42"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	page, err := c.Schedules.List(context.Background(), 10, 0)
	if err != nil || page.Total != 1 || page.Schedules[0].ID != "sch_1" {
		t.Fatalf("list: %#v err=%v", page, err)
	}
	if got, err := c.Schedules.Get(context.Background(), "sch_1"); err != nil || got.ID != "sch_1" {
		t.Fatalf("get: %#v err=%v", got, err)
	}
	created, err := c.Schedules.Create(context.Background(), CreateScheduleRequest{
		SourceID:  "src_1",
		DatasetID: "ds_1",
		Cron:      "0 0 * * *",
		Timezone:  "UTC",
	})
	if err != nil || created.ID != "sch_2" {
		t.Fatalf("create: %#v err=%v", created, err)
	}
	if lastBody["source_id"] != "src_1" || lastBody["dataset_id"] != "ds_1" || lastBody["cron"] != "0 0 * * *" || lastBody["timezone"] != "UTC" {
		t.Fatalf("bad create body: %#v", lastBody)
	}
	disabled := false
	updated, err := c.Schedules.Update(context.Background(), "sch_2", UpdateScheduleRequest{Enabled: &disabled})
	if err != nil || updated.Enabled {
		t.Fatalf("update: %#v err=%v", updated, err)
	}
	if v, ok := lastBody["enabled"].(bool); !ok || v {
		t.Fatalf("update body did not send enabled=false: %#v", lastBody)
	}
	if err := c.Schedules.Delete(context.Background(), "sch_2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	trig, err := c.Schedules.Trigger(context.Background(), "sch_1")
	if err != nil || trig.JobID != "job_42" {
		t.Fatalf("trigger: %#v err=%v", trig, err)
	}
}

func TestIntelligenceSessionsAndMessages(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/intelligence/sessions":
			seen["create"] = true
			body := decodeBody(t, r)
			if body["workspace_id"] != "ws1" || body["dataset_id"] != "ds1" || body["title"] != "Planning" {
				t.Fatalf("bad create session body: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"sess1","title":"Planning"}`))
		case r.Method == "GET" && r.URL.Path == "/intelligence/sessions":
			seen["list"] = true
			if r.URL.Query().Get("limit") != "25" {
				t.Fatalf("bad sessions query: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"sessions":[{"id":"sess1"}]}`))
		case r.Method == "GET" && r.URL.Path == "/intelligence/sessions/sess%2F1":
			seen["get"] = true
			w.Write([]byte(`{"id":"sess1"}`))
		case r.Method == "POST" && r.URL.Path == "/intelligence/sessions/sess%2F1/messages":
			seen["append"] = true
			body := decodeBody(t, r)
			if body["role"] != "user" || body["content"] != "hello" {
				t.Fatalf("bad append body: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"msg1","role":"user","content":"hello"}`))
		case r.Method == "GET" && r.URL.Path == "/intelligence/sessions/sess%2F1/messages":
			seen["messages"] = true
			if r.URL.Query().Get("limit") != "50" {
				t.Fatalf("bad messages query: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"messages":[{"id":"msg1","role":"user","content":"hello"}]}`))
		case r.Method == "DELETE" && r.URL.Path == "/intelligence/sessions/sess%2F1":
			seen["delete"] = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	ctx := context.Background()
	if session, err := c.Intelligence.CreateSession(ctx, SessionCreateRequest{Title: "Planning", WorkspaceID: "ws1", DatasetID: "ds1", Metadata: Metadata{"team": "eng"}}); err != nil || session.ID != "sess1" {
		t.Fatalf("create session: %#v %v", session, err)
	}
	if sessions, err := c.Intelligence.ListSessions(ctx, 25); err != nil || len(sessions.Sessions) != 1 {
		t.Fatalf("list sessions: %#v %v", sessions, err)
	}
	if session, err := c.Intelligence.GetSession(ctx, "sess/1"); err != nil || session.ID != "sess1" {
		t.Fatalf("get session: %#v %v", session, err)
	}
	if msg, err := c.Intelligence.AppendMessage(ctx, "sess/1", SessionMessageCreateRequest{Role: "user", Content: "hello", Metadata: Metadata{"turn": 1}}); err != nil || msg.ID != "msg1" {
		t.Fatalf("append message: %#v %v", msg, err)
	}
	if messages, err := c.Intelligence.ListMessages(ctx, "sess/1", 50); err != nil || len(messages.Messages) != 1 {
		t.Fatalf("list messages: %#v %v", messages, err)
	}
	if err := c.Intelligence.DeleteSession(ctx, "sess/1"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	for _, k := range []string{"create", "list", "get", "append", "messages", "delete"} {
		if !seen[k] {
			t.Fatalf("did not see %s", k)
		}
	}
}

// TestMinimalDatasetCreateAppliesDefaults verifies that creating a dataset with
// only a name infers dim 2560, defaults the embedding to the VectorAmp 4B model,
// defaults the metric to cosine, and forces index_type=sable.
func TestMinimalDatasetCreateAppliesDefaults(t *testing.T) {
	var body map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/datasets" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"ds1","name":"docs","dim":2560,"metric":"cosine","index_type":"sable"}`))
	}))

	ds, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{Name: "docs"})
	if err != nil || ds.ID != "ds1" {
		t.Fatalf("minimal create: %#v %v", ds, err)
	}
	if body["name"] != "docs" {
		t.Fatalf("name not sent: %#v", body)
	}
	if body["dim"].(float64) != 2560 {
		t.Fatalf("dim not inferred to 2560: %#v", body)
	}
	if body["metric"] != "cosine" {
		t.Fatalf("metric not defaulted to cosine: %#v", body)
	}
	if body["index_type"] != "sable" {
		t.Fatalf("index_type not forced to sable: %#v", body)
	}
	emb, ok := body["embedding"].(map[string]interface{})
	if !ok || emb["provider"] != "vectoramp" || emb["model"] != "VectorAmp-Embedding-4B" {
		t.Fatalf("embedding not defaulted to vectoramp 4B: %#v", body)
	}
	if _, sent := body["hybrid"]; sent {
		t.Fatalf("hybrid should be omitted when false: %#v", body)
	}
}

// TestDatasetCreateOpenAIEmbeddingInfersDim verifies that an OpenAI embedding
// helper drives dim inference without an explicit Dim.
func TestDatasetCreateOpenAIEmbeddingInfersDim(t *testing.T) {
	var body map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ds1","name":"docs","dim":3072,"index_type":"sable"}`))
	}))

	if _, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{Name: "docs", Embedding: OpenAIEmbedding("large")}); err != nil {
		t.Fatalf("create with openai embedding: %v", err)
	}
	if body["dim"].(float64) != 3072 {
		t.Fatalf("dim not inferred for openai large: %#v", body)
	}
	emb := body["embedding"].(map[string]interface{})
	if emb["provider"] != "openai" || emb["model"] != "text-embedding-3-large" {
		t.Fatalf("openai embedding not sent: %#v", body)
	}
}

// TestDatasetCreateUnknownModelRequiresDim verifies that an unknown embedding
// model without an explicit Dim is rejected locally before any request is sent.
func TestDatasetCreateUnknownModelRequiresDim(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be sent for unknown model: %s %s", r.Method, r.URL.Path)
	}))
	_, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{
		Name:      "docs",
		Embedding: &EmbeddingConfig{Provider: "acme", Model: "mystery-embed"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot infer dim") {
		t.Fatalf("expected dim inference error, got %v", err)
	}

	// An explicit Dim makes a custom model work.
	var body map[string]interface{}
	c = testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ds1","index_type":"sable"}`))
	}))
	if _, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{
		Name:      "docs",
		Dim:       512,
		Embedding: &EmbeddingConfig{Provider: "acme", Model: "mystery-embed"},
	}); err != nil {
		t.Fatalf("custom model with explicit dim: %v", err)
	}
	if body["dim"].(float64) != 512 {
		t.Fatalf("explicit dim not honored: %#v", body)
	}
}

// TestHybridDatasetCreate verifies that the Hybrid option maps to hybrid:true.
func TestHybridDatasetCreate(t *testing.T) {
	var body map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/datasets" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ds1","name":"docs","index_type":"sable"}`))
	}))

	if _, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{Name: "docs", Hybrid: true}); err != nil {
		t.Fatalf("hybrid create: %v", err)
	}
	if hybrid, ok := body["hybrid"].(bool); !ok || !hybrid {
		t.Fatalf("hybrid:true not sent: %#v", body)
	}
	if body["index_type"] != "sable" {
		t.Fatalf("index_type not forced to sable on hybrid create: %#v", body)
	}
}

// TestNumericVectorIDsPreserved verifies that integer vector ids serialize as
// JSON numbers (not quoted strings) so the API does not rewrite them, while
// string ids stay strings. This exercises the /datasets/{id}/insert endpoint.
func TestNumericVectorIDsPreserved(t *testing.T) {
	var body map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/datasets/ds1/insert" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		// Decode the raw JSON so we can inspect the on-the-wire id types.
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"inserted":3}`))
	}))

	ds := &Dataset{ID: "ds1", client: c}
	_, err := ds.Insert(context.Background(), []Vector{
		{ID: IntID(42), Values: []float64{0.1, 0.2}},
		{ID: StringID("doc-7"), Values: []float64{0.3, 0.4}},
		{Values: []float64{0.5, 0.6}}, // no id -> omitted, API assigns one
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	vectors := body["vectors"].([]interface{})
	if len(vectors) != 3 {
		t.Fatalf("expected 3 vectors: %#v", vectors)
	}
	first := vectors[0].(map[string]interface{})
	// JSON numbers decode to float64; a quoted string would decode to string.
	numID, ok := first["id"].(float64)
	if !ok || numID != 42 {
		t.Fatalf("integer id was not serialized as a JSON number: %#v", first["id"])
	}
	second := vectors[1].(map[string]interface{})
	strID, ok := second["id"].(string)
	if !ok || strID != "doc-7" {
		t.Fatalf("string id was not serialized as a JSON string: %#v", second["id"])
	}
	third := vectors[2].(map[string]interface{})
	if _, present := third["id"]; present {
		t.Fatalf("unset id should be omitted so the API assigns one: %#v", third)
	}
}

// TestNumericVectorIDRoundTrip verifies that a numeric id round-trips through
// marshal/unmarshal without becoming a string.
func TestNumericVectorIDRoundTrip(t *testing.T) {
	v := Vector{ID: IntID(987654321), Values: []float64{1}}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"id":987654321`) {
		t.Fatalf("expected unquoted numeric id, got %s", data)
	}
	var got Vector
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID.String() != "987654321" {
		t.Fatalf("round-trip id string = %q", got.ID.String())
	}
	if iv, ok := got.ID.Value().(int64); !ok || iv != 987654321 {
		t.Fatalf("round-trip id lost its numeric type: %#v", got.ID.Value())
	}
}

// TestAddTextsNumericIDsPreserved verifies that AddTexts preserves numeric
// document ids through the embed+insert flow.
func TestAddTextsNumericIDsPreserved(t *testing.T) {
	var insertBody map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/datasets/ds1/embed":
			w.Write([]byte(`{"embeddings":[[0.1],[0.2]]}`))
		case "/datasets/ds1/insert":
			insertBody = decodeBody(t, r)
			w.Write([]byte(`{"inserted":2}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))

	ds := &Dataset{ID: "ds1", client: c}
	_, err := ds.AddTexts(context.Background(), AddTextsRequest{Texts: []TextDocument{
		{ID: IntID(100), Text: "one"},
		{Text: "two"}, // generated id
	}})
	if err != nil {
		t.Fatalf("add texts: %v", err)
	}
	vectors := insertBody["vectors"].([]interface{})
	first := vectors[0].(map[string]interface{})
	if numID, ok := first["id"].(float64); !ok || numID != 100 {
		t.Fatalf("numeric add-texts id not preserved: %#v", first["id"])
	}
	second := vectors[1].(map[string]interface{})
	if second["id"] != "text-2" {
		t.Fatalf("generated id should be text-2: %#v", second["id"])
	}
}

// TestConfluenceSourceHelper verifies the Confluence source builder and the
// CreateConfluence convenience method produce the correct source_type and config.
func TestConfluenceSourceHelper(t *testing.T) {
	var body map[string]interface{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/ingestion/sources" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"src1","source_type":"confluence"}`))
	}))

	src, err := c.Sources.CreateConfluence(context.Background(), ConfluenceSource{
		CloudID:  "cloud-123",
		Username: "user@example.com",
		APIToken: "token",
		Spaces:   []string{"ENG", "DOCS"},
	})
	if err != nil || src.ID != "src1" {
		t.Fatalf("create confluence: %#v %v", src, err)
	}
	if body["source_type"] != "confluence" {
		t.Fatalf("source_type not confluence: %#v", body)
	}
	if body["name"] != "confluence-cloud-123" {
		t.Fatalf("default confluence name not derived from cloud id: %#v", body["name"])
	}
	config := body["config"].(map[string]interface{})
	if config["type"] != "confluence" || config["cloud_id"] != "cloud-123" {
		t.Fatalf("bad confluence config: %#v", config)
	}
	if config["auth_mode"] != "basic" || config["sync_mode"] != "incremental" {
		t.Fatalf("confluence defaults missing: %#v", config)
	}
	if config["username"] != "user@example.com" || config["api_token"] != "token" {
		t.Fatalf("confluence basic-auth fields missing: %#v", config)
	}
	spaces, ok := config["spaces"].([]interface{})
	if !ok || len(spaces) != 2 || spaces[0] != "ENG" {
		t.Fatalf("confluence spaces not sent: %#v", config["spaces"])
	}
	if _, present := config["include_attachments"]; present {
		t.Fatalf("include_attachments should be omitted by default: %#v", config)
	}
}

// TestConfluenceIngestSource verifies a ConfluenceSource builder can be passed
// straight into IngestSource, which creates the source then starts a job.
func TestConfluenceIngestSource(t *testing.T) {
	steps := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources":
			steps["create"] = true
			body := decodeBody(t, r)
			if body["source_type"] != "confluence" {
				t.Fatalf("bad confluence source: %#v", body)
			}
			w.Write([]byte(`{"id":"src1","source_type":"confluence"}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/jobs":
			steps["job"] = true
			body := decodeBody(t, r)
			if body["source_id"] != "src1" || body["dataset_id"] != "ds1" {
				t.Fatalf("bad confluence job: %#v", body)
			}
			w.Write([]byte(`{"job_id":"job1","status":"pending"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	ds := &Dataset{ID: "ds1", client: c}
	job, err := ds.IngestSource(context.Background(), ConfluenceSource{BaseURL: "https://co.atlassian.net"})
	if err != nil || job.JobID != "job1" {
		t.Fatalf("confluence ingest source: %#v %v", job, err)
	}
	if !steps["create"] || !steps["job"] {
		t.Fatalf("confluence ingest did not create source and start job: %#v", steps)
	}
}

// TestInsertEndpointPath asserts the SDK inserts at /datasets/{id}/insert
// (the verified endpoint) rather than a /vectors path.
func TestInsertEndpointPath(t *testing.T) {
	hit := false
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/datasets/ds1/vectors" {
			t.Fatalf("SDK used the deprecated /vectors insert path")
		}
		if r.Method == "POST" && r.URL.Path == "/datasets/ds1/insert" {
			hit = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"inserted":1}`))
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	if _, err := c.Datasets.Insert(context.Background(), "ds1", []Vector{{ID: StringID("a"), Values: []float64{0.1}}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if !hit {
		t.Fatal("insert endpoint /datasets/ds1/insert was not called")
	}
}

// TestEndpointPathsAreUnprefixed verifies that core methods route to unprefixed
// paths (no /v1 or /api/v1) as required by the verified API surface.
func TestEndpointPathsAreUnprefixed(t *testing.T) {
	paths := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/api/") {
			t.Fatalf("client used a prefixed path: %s", r.URL.Path)
		}
		paths[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/datasets":
			w.Write([]byte(`{"datasets":[],"total":0}`))
		case r.URL.Path == "/datasets/ds1/search":
			w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/intelligence/query":
			w.Write([]byte(`{"answer":"ok"}`))
		case r.URL.Path == "/ingestion/sources":
			w.Write([]byte(`{"sources":[],"total":0}`))
		case r.URL.Path == "/ingestion/schedules":
			w.Write([]byte(`{"schedules":[],"total":0}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))

	ctx := context.Background()
	if _, err := c.Datasets.List(ctx, 0, 0); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, err := c.Datasets.Search(ctx, "ds1", "hi"); err != nil {
		t.Fatalf("search: %v", err)
	}
	if _, err := c.Ask(ctx, "hi", WithAllDatasets()); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if _, err := c.Ingestion.ListSources(ctx, 0, 0); err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if _, err := c.Schedules.List(ctx, 0, 0); err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	for _, p := range []string{"/datasets", "/datasets/ds1/search", "/intelligence/query", "/ingestion/sources", "/ingestion/schedules"} {
		if !paths[p] {
			t.Fatalf("expected unprefixed path %s to be hit", p)
		}
	}
}
