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
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	list, err := c.Datasets.List(context.Background(), 10, 20)
	if err != nil || list.Total != 1 || list.Pagination().Offset != 20 {
		t.Fatalf("list: %#v %v", list, err)
	}
	created, err := c.Datasets.Create(context.Background(), CreateDatasetRequest{Name: "docs", Dim: 3, Metric: "cosine"})
	if err != nil || created.IndexType != "sable" {
		t.Fatalf("create: %#v %v", created, err)
	}
	if _, err := c.Datasets.Get(context.Background(), "ds1"); err != nil {
		t.Fatalf("get: %v", err)
	}
	includeMetadata := false
	search, err := c.Datasets.Search(context.Background(), "ds1", SearchRequest{QueryText: "hello", TopK: 5, IncludeMetadata: &includeMetadata})
	if err != nil || len(search.Results) != 1 {
		t.Fatalf("search: %#v %v", search, err)
	}
	add, err := c.Datasets.AddTexts(context.Background(), "ds1", AddTextsRequest{Texts: []TextDocument{{ID: "a", Text: "one"}, {ID: "b", Text: "two", Metadata: Metadata{"kind": "note"}}}})
	if err != nil || add.Inserted != 2 || add.Embeddings != 2 {
		t.Fatalf("add texts: %#v %v", add, err)
	}
	if err := c.Datasets.Delete(context.Background(), "ds1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, k := range []string{"list", "create", "get", "search", "embed", "insert", "delete"} {
		if !seen[k] {
			t.Fatalf("did not see %s", k)
		}
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
		case r.Method == "POST" && r.URL.Path == "/v1/sources":
			body := decodeBody(t, r)
			if body["source_type"] != "file_upload" {
				t.Fatalf("bad source body: %#v", body)
			}
			w.Write([]byte(`{"id":"src1","name":"upload","source_type":"file_upload"}`))
		case r.Method == "POST" && r.URL.Path == "/v1/sources/src1/upload/init":
			w.Write([]byte(`{"job_id":"job1","uploads":[{"file_id":"file1","file_name":"doc.txt","upload_url":"` + uploadServer.URL + `"}]}`))
		case r.Method == "POST" && r.URL.Path == "/v1/sources/src1/upload/complete":
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
	if job, err := c.Ingestion.IngestFiles(context.Background(), "ds1", []string{tmp}, nil); err != nil || job.JobID != "job1" || !uploadHit {
		t.Fatalf("ingest files: %#v upload=%v err=%v", job, uploadHit, err)
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
