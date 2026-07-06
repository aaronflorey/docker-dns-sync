package adguard

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
)

func TestAdGuardListVisibleRewriteList(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected method GET, got %s", r.Method)
		}
		if r.URL.Path != "/control/rewrite/list" {
			t.Fatalf("expected path /control/rewrite/list, got %s", r.URL.Path)
		}
		assertRequestHeaders(t, r)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"domain":" APP.local ","answer":" 10.0.0.10 "}]`))
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})

	visible, err := provider.ListVisible(context.Background())
	if err != nil {
		t.Fatalf("ListVisible returned error: %v", err)
	}

	if len(visible) != 1 {
		t.Fatalf("expected 1 visible record, got %d", len(visible))
	}
	if visible[0].Output != (contracts.ProviderRef{Type: "adguard", Name: "primary"}) {
		t.Fatalf("unexpected provider ref: %+v", visible[0].Output)
	}
	if visible[0].Hostname != "APP.local" || visible[0].Answer != "10.0.0.10" {
		t.Fatalf("unexpected record mapping: %+v", visible[0])
	}
	if visible[0].Provenance != nil {
		t.Fatalf("expected adguard visible rewrite provenance to be unavailable, got %+v", visible[0].Provenance)
	}
}

func TestAdGuardCreateRewriteAdd(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method POST, got %s", r.Method)
		}
		if r.URL.Path != "/control/rewrite/add" {
			t.Fatalf("expected path /control/rewrite/add, got %s", r.URL.Path)
		}
		assertRequestHeaders(t, r)
		assertBody(t, r, map[string]string{"domain": "app.local", "answer": "10.0.0.10"})
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})

	_, err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
}

func TestAdGuardUpdateRewriteUpdate(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected method PUT, got %s", r.Method)
		}
		if r.URL.Path != "/control/rewrite/update" {
			t.Fatalf("expected path /control/rewrite/update, got %s", r.URL.Path)
		}
		assertRequestHeaders(t, r)
		assertBody(t, r, map[string]map[string]string{
			"target": {"domain": "app.local", "answer": "10.0.0.10"},
			"update": {"domain": "app.local", "answer": "10.0.0.11"},
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})

	_, err := provider.Update(
		context.Background(),
		contracts.VisibleRecord{Hostname: "app.local", Answer: "10.0.0.10"},
		contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.11"},
	)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}

func TestAdGuardDeleteRewriteDelete(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method POST, got %s", r.Method)
		}
		if r.URL.Path != "/control/rewrite/delete" {
			t.Fatalf("expected path /control/rewrite/delete, got %s", r.URL.Path)
		}
		assertRequestHeaders(t, r)
		assertBody(t, r, map[string]string{"domain": "app.local", "answer": "10.0.0.10"})
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})

	err := provider.Delete(context.Background(), contracts.VisibleRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
}

func TestAdGuardErrorsDoNotLeakCredentials(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})
	_, err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err == nil {
		t.Fatalf("expected error")
	}

	if strings.Contains(err.Error(), "supersecret") || strings.Contains(err.Error(), "admin") {
		t.Fatalf("error leaked credentials: %q", err.Error())
	}
}

func TestAdGuardCreateMarksTemporaryServerFailuresRetryable(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})
	_, err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err == nil {
		t.Fatal("expected error")
	}

	var temporary interface{ Temporary() bool }
	if !errors.As(err, &temporary) || !temporary.Temporary() {
		t.Fatalf("expected temporary retryable error, got %T: %v", err, err)
	}
}

func TestAdGuardCreateLeavesBadRequestFailuresTerminal(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer testServer.Close()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: testServer.URL, Username: "admin", Password: "supersecret"})
	_, err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err == nil {
		t.Fatal("expected error")
	}

	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		t.Fatalf("expected terminal error for bad request, got %T: %v", err, err)
	}
}

func assertRequestHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", got)
	}

	user, pass, ok := r.BasicAuth()
	if !ok {
		t.Fatalf("expected basic auth credentials")
	}
	if user != "admin" || pass != "supersecret" {
		t.Fatalf("unexpected basic auth credentials")
	}

	wantHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:supersecret"))
	if got := r.Header.Get("Authorization"); got != wantHeader {
		t.Fatalf("unexpected authorization header")
	}
}

func assertBody(t *testing.T, r *http.Request, want any) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	defer r.Body.Close()

	var gotValue any
	if err := json.Unmarshal(body, &gotValue); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	wantBytes, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encode want body: %v", err)
	}
	var wantValue any
	if err := json.Unmarshal(wantBytes, &wantValue); err != nil {
		t.Fatalf("decode want body: %v", err)
	}

	if !jsonEqual(gotValue, wantValue) {
		t.Fatalf("unexpected request body: got %s want %s", string(body), string(wantBytes))
	}
}

func jsonEqual(a, b any) bool {
	aBytes, _ := json.Marshal(a)
	bBytes, _ := json.Marshal(b)
	return string(aBytes) == string(bBytes)
}
