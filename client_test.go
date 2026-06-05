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
			if body["query_text"] != "hello" || body["top_k"].(float64) != 5 || body["include_metadata"] != false {
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
	search, err := c.Datasets.Search(context.Background(), "ds1", SearchRequest{QueryText: "hello", TopK: 5, IncludeMetadata: &includeMetadata})
	if err != nil || len(search.Results) != 1 {
		t.Fatalf("search: %#v %v", search, err)
	}
	resourceSearch, err := created.Search(context.Background(), SearchRequest{QueryText: "hello", TopK: 5, IncludeMetadata: &includeMetadata})
	if err != nil || len(resourceSearch.Results) != 1 {
		t.Fatalf("resource search: %#v %v", resourceSearch, err)
	}
	if answer, err := got.Ask(context.Background(), AskRequest{Query: "why"}); err != nil || answer.Answer != "because" {
		t.Fatalf("resource ask: %#v %v", answer, err)
	}
	if job, err := got.IngestSource(context.Background(), "src1", "pipe1"); err != nil || job.JobID != "job1" {
		t.Fatalf("resource ingest source: %#v %v", job, err)
	}
	add, err := created.AddTexts(context.Background(), AddTextsRequest{Texts: []TextDocument{{ID: "a", Text: "one"}, {ID: "b", Text: "two", Metadata: Metadata{"kind": "note"}}}})
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

func TestMinimalConvenienceInputs(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/datasets/ds1/search":
			seen["search"] = true
			body := decodeBody(t, r)
			if body["query_text"] != "hello" || body["top_k"].(float64) != 3 || body["include_metadata"] != false {
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
	if _, err := ds.Search(context.Background(), "hello", WithSearchTopK(3), WithSearchMetadata(false)); err != nil {
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
