package cloudflare

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"strings"
	"testing"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	cloudflareapi "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"
)

func TestCloudflareListVisibleCachesRecordMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r)
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/zones/zone-123/dns_records") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") == "2" {
			writeCloudflareJSON(t, w, map[string]any{
				"result":      []map[string]any{},
				"success":     true,
				"errors":      []any{},
				"messages":    []any{},
				"result_info": map[string]any{"page": 2, "per_page": 100},
			})
			return
		}

		writeCloudflareJSON(t, w, map[string]any{
			"result": []map[string]any{
				{"id": "rec-a", "type": "A", "name": "APP.example.com", "content": "10.0.0.10"},
				{"id": "rec-cname", "type": "CNAME", "name": "edge.example.com", "content": "origin.internal"},
				{"id": "rec-txt", "type": "TXT", "name": "ignored.example.com", "content": "ignored"},
			},
			"success":     true,
			"errors":      []any{},
			"messages":    []any{},
			"result_info": map[string]any{"page": 1, "per_page": 100},
		})
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	visible, err := provider.ListVisible(context.Background())
	if err != nil {
		t.Fatalf("ListVisible returned error: %v", err)
	}

	if len(visible) != 2 {
		t.Fatalf("expected 2 visible records, got %d", len(visible))
	}
	if visible[0].Hostname != "app.example.com" || visible[0].Answer != "10.0.0.10" {
		t.Fatalf("unexpected first record: %+v", visible[0])
	}
	if meta, ok := provider.lookupVisibleRecord("app.example.com", "10.0.0.10"); !ok || meta.id != "rec-a" {
		t.Fatalf("expected cached record metadata, got %+v ok=%v", meta, ok)
	}
}

func TestCloudflareListVisibleStripsZoneSuffix(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r)
		if !strings.HasSuffix(r.URL.Path, "/zones/zone-123/dns_records") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") == "2" {
			writeCloudflareJSON(t, w, map[string]any{
				"result":      []map[string]any{},
				"success":     true,
				"errors":      []any{},
				"messages":    []any{},
				"result_info": map[string]any{"page": 2, "per_page": 100},
			})
			return
		}

		writeCloudflareJSON(t, w, map[string]any{
			"result": []map[string]any{{"id": "rec-a", "type": "A", "name": "whoami.test.jcaks.net", "content": "127.0.0.1"}},
			"success":     true,
			"errors":      []any{},
			"messages":    []any{},
			"result_info": map[string]any{"page": 1, "per_page": 100},
		})
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	provider.zoneName = "jcaks.net"
	visible, err := provider.ListVisible(context.Background())
	if err != nil {
		t.Fatalf("ListVisible returned error: %v", err)
	}
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible record, got %d", len(visible))
	}
	if visible[0].Hostname != "whoami.test" {
		t.Fatalf("expected zone-relative hostname, got %+v", visible[0])
	}
}

func TestCloudflareCreateInfersRecordType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		desired     contracts.DesiredRecord
		wantType    string
		wantContent string
	}{
		{name: "ipv4 becomes A", desired: contracts.DesiredRecord{Hostname: "app.example.com", Answer: "10.0.0.10"}, wantType: "A", wantContent: "10.0.0.10"},
		{name: "ipv6 becomes AAAA", desired: contracts.DesiredRecord{Hostname: "app.example.com", Answer: "2001:db8::1"}, wantType: "AAAA", wantContent: "2001:db8::1"},
		{name: "hostname becomes CNAME", desired: contracts.DesiredRecord{Hostname: "app.example.com", Answer: "origin.internal"}, wantType: "CNAME", wantContent: "origin.internal"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertBearerToken(t, r)
				if r.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", r.Method)
				}
				if !strings.HasSuffix(r.URL.Path, "/zones/zone-123/dns_records") {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				body := decodeBody(t, r)
				if body["type"] != tt.wantType || body["content"] != tt.wantContent || body["name"] != tt.desired.Hostname {
					t.Fatalf("unexpected create body: %+v", body)
				}
				writeCloudflareJSON(t, w, map[string]any{"result": map[string]any{"id": "rec-new"}, "success": true, "errors": []any{}, "messages": []any{}})
			}))
			defer server.Close()

			provider := newTestProvider(server.URL)
			if err := provider.Create(context.Background(), tt.desired); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
		})
	}
}

func TestCloudflareCreateRecoversDuplicateRecord(t *testing.T) {
	t.Parallel()

	var listCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r)
		if !strings.HasSuffix(r.URL.Path, "/zones/zone-123/dns_records") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
			writeCloudflareJSON(t, w, map[string]any{
				"result":   nil,
				"success":  false,
				"errors":   []map[string]any{{"code": 81058, "message": "An identical record already exists."}},
				"messages": []any{},
			})
		case http.MethodGet:
			listCalls.Add(1)
			if r.URL.Query().Get("page") == "2" {
				writeCloudflareJSON(t, w, map[string]any{
					"result":      []map[string]any{},
					"success":     true,
					"errors":      []any{},
					"messages":    []any{},
					"result_info": map[string]any{"page": 2, "per_page": 100},
				})
				return
			}
			writeCloudflareJSON(t, w, map[string]any{
				"result": []map[string]any{{"id": "rec-a", "type": "A", "name": "whoami.test.jcaks.net", "content": "127.0.0.1"}},
				"success":     true,
				"errors":      []any{},
				"messages":    []any{},
				"result_info": map[string]any{"page": 1, "per_page": 100},
			})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	provider.zoneName = "jcaks.net"
	err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "whoami.test", Answer: "127.0.0.1"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got := listCalls.Load(); got != 2 {
		t.Fatalf("expected two recovery list calls across pagination, got %d", got)
	}
	if meta, ok := provider.lookupVisibleRecord("whoami.test", "127.0.0.1"); !ok || meta.id != "rec-a" {
		t.Fatalf("expected cached record metadata after recovery, got %+v ok=%v", meta, ok)
	}
}

func TestCloudflareCreateDuplicateWithoutVisibleMatchFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r)
		if !strings.HasSuffix(r.URL.Path, "/zones/zone-123/dns_records") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
			writeCloudflareJSON(t, w, map[string]any{
				"result":   nil,
				"success":  false,
				"errors":   []map[string]any{{"code": 81058, "message": "An identical record already exists."}},
				"messages": []any{},
			})
		case http.MethodGet:
			if r.URL.Query().Get("page") == "2" {
				writeCloudflareJSON(t, w, map[string]any{
					"result":      []map[string]any{},
					"success":     true,
					"errors":      []any{},
					"messages":    []any{},
					"result_info": map[string]any{"page": 2, "per_page": 100},
				})
				return
			}
			writeCloudflareJSON(t, w, map[string]any{
				"result": []map[string]any{{"id": "rec-other", "type": "A", "name": "other.example.com", "content": "10.0.0.11"}},
				"success":     true,
				"errors":      []any{},
				"messages":    []any{},
				"result_info": map[string]any{"page": 1, "per_page": 100},
			})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	provider.zoneName = "jcaks.net"
	err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.example.com", Answer: "10.0.0.10"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create cloudflare dns record") {
		t.Fatalf("expected wrapped create error, got %v", err)
	}
}

func TestCloudflareUpdateAndDeleteUseCachedRecordID(t *testing.T) {
	t.Parallel()

	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r)
		if !strings.HasSuffix(r.URL.Path, "/zones/zone-123/dns_records/rec-a") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		switch r.Method {
		case http.MethodPut:
			requests <- decodeBody(t, r)
		case http.MethodDelete:
			requests <- map[string]any{"method": r.Method}
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}

		writeCloudflareJSON(t, w, map[string]any{"result": map[string]any{"id": "rec-a"}, "success": true, "errors": []any{}, "messages": []any{}})
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	provider.visible[visibleRecordKey("app.example.com", "10.0.0.10")] = visibleRecordMeta{id: "rec-a"}

	visible := contracts.VisibleRecord{Hostname: "app.example.com", Answer: "10.0.0.10"}
	if err := provider.Update(context.Background(), visible, contracts.DesiredRecord{Hostname: "app.example.com", Answer: "origin.internal"}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if err := provider.Delete(context.Background(), visible); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	updateBody := <-requests
	if updateBody["type"] != "CNAME" || updateBody["content"] != "origin.internal" {
		t.Fatalf("unexpected update body: %+v", updateBody)
	}
	deleteReq := <-requests
	if deleteReq["method"] != http.MethodDelete {
		t.Fatalf("expected delete request marker, got %+v", deleteReq)
	}
}

func TestCloudflareErrorsDoNotLeakAPIKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.example.com", Answer: "10.0.0.10"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "cf-secret-token") {
		t.Fatalf("error leaked api key: %q", err.Error())
	}
}

func newTestProvider(baseURL string) *Provider {
	provider := New(config.OutputConfig{Type: "cloudflare", Name: "primary", ZoneID: "zone-123", APIKey: "cf-secret-token"})
	provider.client = cloudflareapi.NewClient(
		option.WithAPIToken("cf-secret-token"),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(0),
	)
	provider.zoneName = "unit.test"
	return provider
}

func assertBearerToken(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer cf-secret-token" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	defer r.Body.Close()

	if len(body) == 0 {
		return map[string]any{}
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return payload
}

func writeCloudflareJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
