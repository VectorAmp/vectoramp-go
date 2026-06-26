package vectoramp

import (
	"context"
	"net/http"
	"testing"
)

// TestSourceManagementMethods asserts method, path, query, and body for the
// source-management additions on the Ingestion/Sources service.
func TestSourceManagementMethods(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "DELETE" && r.URL.Path == "/ingestion/sources/src1":
			seen["delete"] = true
			if r.URL.Query().Get("force") != "" {
				t.Fatalf("plain delete should not send force: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "DELETE" && r.URL.Path == "/ingestion/sources/src2":
			seen["deleteForce"] = true
			if r.URL.Query().Get("force") != "true" {
				t.Fatalf("force delete should send force=true: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "GET" && r.URL.Path == "/ingestion/sources/unused":
			seen["unused"] = true
			if r.URL.Query().Get("limit") != "5" || r.URL.Query().Get("offset") != "2" {
				t.Fatalf("bad unused pagination: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"sources":[{"id":"src9","name":"old"}],"total":1,"limit":5,"offset":2}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources/cleanup":
			seen["cleanup"] = true
			w.Write([]byte(`{"deleted":[{"id":"src9","name":"old","type":"web"},{"id":"src10","name":"older","type":"s3"}],"count":2}`))
		case r.Method == "GET" && r.URL.Path == "/ingestion/sources/src1/references":
			seen["references"] = true
			w.Write([]byte(`{"schedules":[{"id":"sch1","name":"daily"}],"schedule_count":1,"active_job_count":0,"in_use":true}`))
		case r.Method == "POST" && r.URL.Path == "/ingestion/sources/validate":
			seen["validate"] = true
			body := decodeBody(t, r)
			if body["source_type"] != "s3" {
				t.Fatalf("bad validate source_type: %#v", body)
			}
			cfg, ok := body["config"].(map[string]interface{})
			if !ok || cfg["bucket"] != "docs" {
				t.Fatalf("bad validate config: %#v", body)
			}
			w.Write([]byte(`{"success":true,"message":"ok","warnings":["region not set"]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	ctx := context.Background()
	if err := c.Sources.DeleteSource(ctx, "src1"); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	if err := c.Sources.DeleteSource(ctx, "src2", WithForce()); err != nil {
		t.Fatalf("force delete source: %v", err)
	}
	if list, err := c.Sources.ListUnusedSources(ctx, 5, 2); err != nil || list.Total != 1 || len(list.Sources) != 1 {
		t.Fatalf("list unused: %#v %v", list, err)
	}
	cleanup, err := c.Sources.CleanupUnusedSources(ctx)
	if err != nil || cleanup.Count != 2 || len(cleanup.Deleted) != 2 || cleanup.Deleted[0].ID != "src9" {
		t.Fatalf("cleanup: %#v %v", cleanup, err)
	}
	refs, err := c.Sources.GetSourceReferences(ctx, "src1")
	if err != nil || !refs.InUse || refs.ScheduleCount != 1 || len(refs.Schedules) != 1 || refs.Schedules[0].Name != "daily" {
		t.Fatalf("references: %#v %v", refs, err)
	}
	res, err := c.Sources.ValidateSource(ctx, "s3", map[string]interface{}{"bucket": "docs"})
	if err != nil || !res.Success || len(res.Warnings) != 1 {
		t.Fatalf("validate: %#v %v", res, err)
	}
	for _, k := range []string{"delete", "deleteForce", "unused", "cleanup", "references", "validate"} {
		if !seen[k] {
			t.Fatalf("did not see %s", k)
		}
	}
}

// TestConnectionsService asserts the root-level /connections CRUD surface.
func TestConnectionsService(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing api key header on connections request: %q", r.Header.Get("X-API-Key"))
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/connections":
			seen["list"] = true
			if r.URL.Query().Get("provider") != "google" {
				t.Fatalf("missing provider filter: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"connections":[{"id":"conn1","provider":"google","status":"active"}]}`))
		case r.Method == "POST" && r.URL.Path == "/connections":
			seen["create"] = true
			body := decodeBody(t, r)
			if body["provider"] != "google" || body["source_type"] != "gdrive" {
				t.Fatalf("bad create connection body: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"conn2","provider":"google","status":"pending","authorization_url":"https://auth.example.com/go"}`))
		case r.Method == "GET" && r.URL.Path == "/connections/conn1":
			seen["get"] = true
			w.Write([]byte(`{"id":"conn1","provider":"google","status":"active"}`))
		case r.Method == "DELETE" && r.URL.Path == "/connections/conn1":
			seen["delete"] = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	ctx := context.Background()
	list, err := c.Connections.List(ctx, WithConnectionProvider("google"))
	if err != nil || len(list.Connections) != 1 || list.Connections[0].ID != "conn1" {
		t.Fatalf("list connections: %#v %v", list, err)
	}
	conn, err := c.Connections.Create(ctx, "google", WithConnectionSourceType("gdrive"))
	if err != nil || conn.ID != "conn2" || conn.AuthorizationURL == "" {
		t.Fatalf("create connection: %#v %v", conn, err)
	}
	got, err := c.Connections.Get(ctx, "conn1")
	if err != nil || got.ID != "conn1" {
		t.Fatalf("get connection: %#v %v", got, err)
	}
	if err := c.Connections.Delete(ctx, "conn1"); err != nil {
		t.Fatalf("delete connection: %v", err)
	}
	for _, k := range []string{"list", "create", "get", "delete"} {
		if !seen[k] {
			t.Fatalf("did not see %s", k)
		}
	}
}

// TestOAuthBuildersConnectionID verifies that ConnectionID on the OAuth source
// builders serializes into config["connection_id"] when set and is omitted otherwise.
func TestOAuthBuildersConnectionID(t *testing.T) {
	cases := []struct {
		name    string
		builder SourceBuilder
		want    string
	}{
		{"gdrive", GoogleDriveSource{ConnectionID: "conn-gd", FolderIDs: []string{"f1"}}, "conn-gd"},
		{"gcs", GCSSource{ConnectionID: "conn-gcs", Bucket: "b"}, "conn-gcs"},
		{"confluence", ConfluenceSource{ConnectionID: "conn-cf", CloudID: "c"}, "conn-cf"},
		{"jira", JiraSource{ConnectionID: "conn-jira", CloudID: "c"}, "conn-jira"},
	}
	for _, tc := range cases {
		req := tc.builder.ToCreateSourceRequest()
		if got := req.Config["connection_id"]; got != tc.want {
			t.Fatalf("%s connection_id = %#v, want %s", tc.name, got, tc.want)
		}
	}

	// When ConnectionID is empty, connection_id must be omitted from config.
	for _, b := range []SourceBuilder{
		GoogleDriveSource{FolderIDs: []string{"f1"}},
		GCSSource{Bucket: "b"},
		ConfluenceSource{CloudID: "c"},
		JiraSource{CloudID: "c"},
	} {
		req := b.ToCreateSourceRequest()
		if _, present := req.Config["connection_id"]; present {
			t.Fatalf("%T connection_id should be omitted when empty: %#v", b, req.Config)
		}
	}
}
