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
		{"gitlab", GitLabSource{ConnectionID: "conn-gl", Groups: []string{"g"}}, "conn-gl"},
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
		GitLabSource{Groups: []string{"g"}},
	} {
		req := b.ToCreateSourceRequest()
		if _, present := req.Config["connection_id"]; present {
			t.Fatalf("%T connection_id should be omitted when empty: %#v", b, req.Config)
		}
	}
}

// TestGitHubSourceBuilder verifies the GitHub builder emits the config keys the
// ingestion service's GitHubSourceConfig expects, defaults the name from the
// first repository, and omits optional fields that were never set so the
// server applies its own defaults.
func TestGitHubSourceBuilder(t *testing.T) {
	req := GitHubSource{
		InstallationID:   4242,
		Repositories:     []string{"VectorAmp/Ingestion", "VectorAmp/Web"},
		RefMode:          "explicit",
		Refs:             []string{"main", "release"},
		ExcludedRefs:     []string{"wip"},
		ActiveBranchDays: 30,
		IncludeGlobs:     []string{"**/*.go"},
		ExcludeGlobs:     []string{"vendor/**"},
		MaxFileSizeBytes: 2_000_000,
	}.ToCreateSourceRequest()

	if req.SourceType != SourceTypeGitHub {
		t.Fatalf("source_type = %q, want %q", req.SourceType, SourceTypeGitHub)
	}
	if req.Name != "github-vectoramp-ingestion" {
		t.Fatalf("default name = %q", req.Name)
	}
	if req.Config["type"] != SourceTypeGitHub {
		t.Fatalf("config type = %#v", req.Config["type"])
	}
	if req.Config["installation_id"] != int64(4242) {
		t.Fatalf("installation_id = %#v", req.Config["installation_id"])
	}
	repos, ok := req.Config["repositories"].([]string)
	if !ok || len(repos) != 2 || repos[0] != "VectorAmp/Ingestion" {
		t.Fatalf("repositories = %#v", req.Config["repositories"])
	}
	if req.Config["ref_mode"] != "explicit" {
		t.Fatalf("ref_mode = %#v", req.Config["ref_mode"])
	}
	if req.Config["active_branch_days"] != 30 {
		t.Fatalf("active_branch_days = %#v", req.Config["active_branch_days"])
	}
	if req.Config["max_file_size_bytes"] != 2_000_000 {
		t.Fatalf("max_file_size_bytes = %#v", req.Config["max_file_size_bytes"])
	}
	if req.Config["sync_mode"] != "incremental" {
		t.Fatalf("sync_mode = %#v", req.Config["sync_mode"])
	}
	// The three include_* toggles default to true server-side, so an unset
	// pointer must not be serialized as false.
	for _, key := range []string{"include_pull_requests", "include_review_threads", "include_direct_commits"} {
		if _, present := req.Config[key]; present {
			t.Fatalf("%s should be omitted when unset: %#v", key, req.Config)
		}
	}
	// GitHub authenticates via the App installation, never a token.
	for _, key := range []string{"access_token", "connection_id"} {
		if _, present := req.Config[key]; present {
			t.Fatalf("%s must not be sent for github sources: %#v", key, req.Config)
		}
	}
}

// TestGitHubSourceBuilderOptionalFields checks explicit false toggles are sent,
// that ConfigExtra wins, and that the name falls back when no repository is set.
func TestGitHubSourceBuilderOptionalFields(t *testing.T) {
	no := false
	req := GitHubSource{
		InstallationID:       7,
		Repositories:         []string{"o/r"},
		IncludePullRequests:  &no,
		IncludeReviewThreads: &no,
		IncludeDirectCommits: &no,
		SyncMode:             "full",
		ConfigExtra:          map[string]interface{}{"ref_mode": "default", "custom": 1},
	}.ToCreateSourceRequest()

	for _, key := range []string{"include_pull_requests", "include_review_threads", "include_direct_commits"} {
		if req.Config[key] != false {
			t.Fatalf("%s = %#v, want false", key, req.Config[key])
		}
	}
	if req.Config["sync_mode"] != "full" {
		t.Fatalf("sync_mode = %#v", req.Config["sync_mode"])
	}
	// ConfigExtra is merged last and therefore overrides derived values.
	if req.Config["ref_mode"] != "default" || req.Config["custom"] != 1 {
		t.Fatalf("ConfigExtra not merged last: %#v", req.Config)
	}

	empty := GitHubSource{InstallationID: 1}.ToCreateSourceRequest()
	if empty.Name != "go-sdk-github-source" {
		t.Fatalf("fallback name = %q", empty.Name)
	}
	if repos, ok := empty.Config["repositories"].([]string); !ok || len(repos) != 0 {
		t.Fatalf("repositories should be an empty slice, got %#v", empty.Config["repositories"])
	}
}

// TestGitLabSourceBuilder verifies the GitLab builder against GitLabSourceConfig,
// including the oauth auth_mode default and merge-request naming.
func TestGitLabSourceBuilder(t *testing.T) {
	req := GitLabSource{
		ConnectionID:     "conn-gl",
		Groups:           []string{"platform"},
		Projects:         []string{"platform/ingestion"},
		RefMode:          "active",
		ActiveBranchDays: 14,
		MaxFileSizeBytes: 500_000,
	}.ToCreateSourceRequest()

	if req.SourceType != SourceTypeGitLab {
		t.Fatalf("source_type = %q", req.SourceType)
	}
	// Projects take precedence over groups for the default name.
	if req.Name != "gitlab-platform-ingestion" {
		t.Fatalf("default name = %q", req.Name)
	}
	if req.Config["auth_mode"] != "oauth" {
		t.Fatalf("auth_mode = %#v, want oauth", req.Config["auth_mode"])
	}
	if req.Config["connection_id"] != "conn-gl" {
		t.Fatalf("connection_id = %#v", req.Config["connection_id"])
	}
	if req.Config["sync_mode"] != "incremental" {
		t.Fatalf("sync_mode = %#v", req.Config["sync_mode"])
	}
	groups, ok := req.Config["groups"].([]string)
	if !ok || len(groups) != 1 || groups[0] != "platform" {
		t.Fatalf("groups = %#v", req.Config["groups"])
	}
	// gitlab_url is omitted so the service default (https://gitlab.com) applies.
	if _, present := req.Config["gitlab_url"]; present {
		t.Fatalf("gitlab_url should be omitted when unset: %#v", req.Config)
	}
	// GitLab uses merge requests, not pull requests.
	if _, present := req.Config["include_pull_requests"]; present {
		t.Fatalf("include_pull_requests is not a gitlab config key: %#v", req.Config)
	}
	for _, key := range []string{"include_merge_requests", "include_review_threads", "include_direct_commits"} {
		if _, present := req.Config[key]; present {
			t.Fatalf("%s should be omitted when unset: %#v", key, req.Config)
		}
	}
}

// TestGitLabSourceBuilderTokenMode covers self-managed token auth and the
// group-only name fallback.
func TestGitLabSourceBuilderTokenMode(t *testing.T) {
	yes := true
	req := GitLabSource{
		AuthMode:             "token",
		GitLabURL:            "https://gitlab.example.com",
		AccessToken:          "glpat-secret",
		Groups:               []string{"infra/tools"},
		IncludeMergeRequests: &yes,
	}.ToCreateSourceRequest()

	if req.Config["auth_mode"] != "token" {
		t.Fatalf("auth_mode = %#v", req.Config["auth_mode"])
	}
	if req.Config["gitlab_url"] != "https://gitlab.example.com" {
		t.Fatalf("gitlab_url = %#v", req.Config["gitlab_url"])
	}
	if req.Config["access_token"] != "glpat-secret" {
		t.Fatalf("access_token = %#v", req.Config["access_token"])
	}
	if req.Config["include_merge_requests"] != true {
		t.Fatalf("include_merge_requests = %#v", req.Config["include_merge_requests"])
	}
	// No projects, so the name falls back to the first group.
	if req.Name != "gitlab-infra-tools" {
		t.Fatalf("group fallback name = %q", req.Name)
	}

	bare := GitLabSource{}.ToCreateSourceRequest()
	if bare.Name != "go-sdk-gitlab-source" {
		t.Fatalf("fallback name = %q", bare.Name)
	}
}

// TestSCMBuildersSatisfySourceBuilder pins both new types into the SourceBuilder
// interface so CreateSource/IngestSource accept them.
func TestSCMBuildersSatisfySourceBuilder(t *testing.T) {
	var _ SourceBuilder = GitHubSource{}
	var _ SourceBuilder = GitLabSource{}

	for _, tc := range []struct {
		builder SourceBuilder
		want    string
	}{
		{GitHubSource{InstallationID: 1, Repositories: []string{"o/r"}}, SourceTypeGitHub},
		{GitLabSource{Projects: []string{"g/p"}}, SourceTypeGitLab},
	} {
		if req, ok := normalizeCreateSourceRequest(tc.builder); !ok || req.SourceType != tc.want {
			t.Fatalf("normalize %T -> %#v ok=%v", tc.builder, req, ok)
		}
	}
}
