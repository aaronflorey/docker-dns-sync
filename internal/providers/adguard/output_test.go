package adguard

import (
	"context"
	"testing"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
)

func TestAdGuardListVisibleRewriteList(t *testing.T) {
	t.Parallel()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: "http://127.0.0.1:3000", Username: "admin", Password: "supersecret"})

	_, err := provider.ListVisible(context.Background())
	if err != nil {
		t.Fatalf("ListVisible returned error: %v", err)
	}
}

func TestAdGuardCreateRewriteAdd(t *testing.T) {
	t.Parallel()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: "http://127.0.0.1:3000", Username: "admin", Password: "supersecret"})

	err := provider.Create(context.Background(), contracts.DesiredRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
}

func TestAdGuardUpdateRewriteUpdate(t *testing.T) {
	t.Parallel()

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: "http://127.0.0.1:3000", Username: "admin", Password: "supersecret"})

	err := provider.Update(
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

	provider := New(config.OutputConfig{Type: "adguard", Name: "primary", URL: "http://127.0.0.1:3000", Username: "admin", Password: "supersecret"})

	err := provider.Delete(context.Background(), contracts.VisibleRecord{Hostname: "app.local", Answer: "10.0.0.10"})
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
}
