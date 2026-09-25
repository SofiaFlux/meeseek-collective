package ghissue

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func tokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validIssueJSON = `{"number":7,"title":"t","body":"","state":"open","user":{"login":"maint"},"assignees":[],"labels":[],"updated_at":"2026-09-24T10:00:00Z","html_url":"https://github.com/o/r/issues/7"}`

func TestListIssuesRequestsOpenIssuesPageWithBearerToken(t *testing.T) {
	var seen *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		_, _ = w.Write([]byte("[" + validIssueJSON + "]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit\n")})
	if err != nil {
		t.Fatal(err)
	}
	items, bad, next, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(bad) != 0 || next != "" {
		t.Fatalf("items=%d bad=%d next=%q", len(items), len(bad), next)
	}
	if seen.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", seen.Method)
	}
	if seen.URL.Path != "/repos/o/r/issues" {
		t.Fatalf("path = %q", seen.URL.Path)
	}
	if seen.URL.Query().Get("state") != "open" || seen.URL.Query().Get("per_page") != "100" {
		t.Fatalf("query = %q", seen.URL.RawQuery)
	}
	if seen.Header.Get("Authorization") != "Bearer sekrit" {
		t.Fatalf("authorization = %q", seen.Header.Get("Authorization"))
	}
	if seen.Header.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("accept = %q", seen.Header.Get("Accept"))
	}
	if client.Name() != "o/r" {
		t.Fatalf("name = %q", client.Name())
	}
	var lister IssueLister = client
	if lister.Name() != "o/r" {
		t.Fatal("client does not satisfy IssueLister")
	}
}

func TestListIssuesFollowsLinkPagination(t *testing.T) {
	var paths []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		w.Header().Set("Link", `<`+server.URL+`/repos/o/r/issues?state=open&per_page=100&page=2>; rel="next"`)
		_, _ = w.Write([]byte("[" + validIssueJSON + "]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, next, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "page=2") {
		t.Fatalf("next = %q, want page-2 cursor", next)
	}
	if _, _, last, err := client.ListIssues(context.Background(), next); err != nil || last != "" {
		t.Fatalf("last=%q err=%v", last, err)
	}
	if len(paths) != 2 || !strings.Contains(paths[1], "page=2") {
		t.Fatalf("paths = %v", paths)
	}
}

func TestListIssuesSplitsUnparseableItems(t *testing.T) {
	malformedLabels := `{"number":8,"title":"t","state":"open","user":{"login":"maint"},` +
		`"updated_at":"2026-09-24T10:00:00Z","html_url":"https://github.com/o/r/issues/8","labels":"bug"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[" + validIssueJSON + `,{"number":8},` + malformedLabels + `]`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	items, bad, _, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(bad) != 2 {
		t.Fatalf("items=%d bad=%d", len(items), len(bad))
	}
	for _, entry := range bad {
		if entry.Raw == nil {
			t.Fatal("unparseable entry lost its raw item")
		}
	}
}

func TestListIssuesRejectsNonArrayBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("null"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil {
		t.Fatal("expected error for null body")
	}
}

func TestListIssuesRejectsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/repos/o/r/issues", http.StatusFound)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil {
		t.Fatal("expected error for redirect response")
	}
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	token := tokenFile(t, "sekrit")
	cases := []Config{
		{BaseURL: "https://api.github.com", Repository: "o", TokenFile: token},
		{BaseURL: "https://api.github.com", Repository: "../x", TokenFile: token},
		{BaseURL: "https://api.github.com", Repository: "o/r/extra", TokenFile: token},
		{BaseURL: "https://user:pass@api.github.com", Repository: "o/r", TokenFile: token},
		{BaseURL: "http://api.github.com", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com", Repository: "o/r", TokenFile: " "},
		{BaseURL: "https://evil.example", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com/v3", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com?x=1", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com#frag", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://ghe.example", Repository: "o/r", TokenFile: token},
	}
	for i, cfg := range cases {
		if _, err := New(cfg); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestListIssuesRejectsUnsafeTokenFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	loose := filepath.Join(t.TempDir(), "loose")
	if err := os.WriteFile(loose, []byte("sekrit"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"empty":             tokenFile(t, "  \n"),
		"loose permissions": loose,
		"missing":           filepath.Join(t.TempDir(), "absent"),
		"directory":         t.TempDir(),
	}
	for name, path := range paths {
		client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: path})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestListIssuesSanitizesHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusBadGateway} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"token sekrit rejected"}`))
		}))
		client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, err = client.ListIssues(context.Background(), "")
		server.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", status)
		}
		if !strings.Contains(err.Error(), strconv.Itoa(status)) {
			t.Fatalf("status %d: err = %v, want status in message", status, err)
		}
		if strings.Contains(err.Error(), "sekrit") {
			t.Fatalf("status %d leaked token: %v", status, err)
		}
	}
}

func TestListIssuesEnforcesResponseCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit"), MaxResponseBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "128") {
		t.Fatalf("err = %v, want response cap error", err)
	}
}

func TestListIssuesRejectsUnsafeLinkHeaders(t *testing.T) {
	links := map[string]func(origin string) string{
		"malformed":       func(string) string { return `<>; rel="next"` },
		"cross origin":    func(string) string { return `<http://other.example/repos/o/r/issues?page=2>; rel="next"` },
		"unquoted rel":    func(origin string) string { return `<` + origin + `/repos/o/r/issues?page=2>; rel=next` },
		"empty section":   func(string) string { return `,` },
		"relative target": func(string) string { return `</repos/o/r/issues?page=2>; rel="next"` },
		"userinfo target": func(origin string) string {
			return `<http://user:pass@` + strings.TrimPrefix(origin, "http://") + `/repos/o/r/issues?page=2>; rel="next"`
		},
		"fragment target":  func(origin string) string { return `<` + origin + `/repos/o/r/issues?page=2#frag>; rel="next"` },
		"foreign resource": func(origin string) string { return `<` + origin + `/repos/o/other/issues?page=2>; rel="next"` },
	}
	for name, build := range links {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Link", build(server.URL))
			_, _ = w.Write([]byte("[]"))
		}))
		client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, err = client.ListIssues(context.Background(), "")
		server.Close()
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestListIssuesFollowsQuotedNextLinkAndIgnoresPrev(t *testing.T) {
	var nextCalls int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "page=2") {
			nextCalls++
			_, _ = w.Write([]byte("[" + validIssueJSON + "]"))
			return
		}
		w.Header().Set("Link", `<`+server.URL+`/repos/o/r/issues?page=2>; rel="next", <`+server.URL+`/repos/o/r/issues?page=9>; rel="prev"`)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, cursor, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cursor, "page=2") {
		t.Fatalf("cursor = %q, want page 2", cursor)
	}
	if _, _, _, err := client.ListIssues(context.Background(), cursor); err != nil {
		t.Fatal(err)
	}
	if nextCalls != 1 {
		t.Fatalf("next page fetches = %d, want 1", nextCalls)
	}
}

func TestListIssuesPropagatesContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := client.ListIssues(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestListIssuesHonorsClientTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit"), Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestClientExposesOnlyReadOperations(t *testing.T) {
	var methods []string
	for _, name := range []string{"POST", "PATCH", "PUT", "DELETE", "Create", "Update", "Delete", "Comment", "Merge", "ListIssues", "Name"} {
		if _, ok := reflect.TypeOf(&Client{}).MethodByName(name); ok {
			methods = append(methods, name)
		}
	}
	if !reflect.DeepEqual(methods, []string{"ListIssues", "Name"}) {
		t.Fatalf("methods = %v, want only ListIssues and Name", methods)
	}
}
