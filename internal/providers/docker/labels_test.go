package docker

import (
	"net/netip"
	"testing"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

func TestDockerLabelsSubset(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}

	tests := []struct {
		name          string
		defaultTarget string
		container     containertypes.Summary
		want          []contracts.DesiredRecord
	}{
		{
			name:          "omits local records without a derived target",
			defaultTarget: "",
			container: containerSummary("ctr-local-no-ip", "/edge", map[string]string{
				"proxy.aliases":   "edge",
				"proxy.edge.port": "8080",
			}, "running", "172.18.0.10"),
			want: nil,
		},
		{
			name:          "keeps explicit host overrides when the default target is unavailable",
			defaultTarget: "",
			container: containerSummary("ctr-local-host", "/edge", map[string]string{
				"proxy.aliases":   "edge",
				"proxy.edge.port": "8080",
				"proxy.edge.host": "edge.internal",
			}, "running", ""),
			want: []contracts.DesiredRecord{{
				Hostname: "edge",
				Answer:   "edge.internal",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-local-host", DisplayName: "edge"},
			}},
		},
		{
			name:          "falls back to configured default target when aliases are absent",
			defaultTarget: "edge.example",
			container: containerSummary("ctr-1", "/app", map[string]string{
				"proxy.*.port": "8080",
			}, "running", "172.18.0.10"),
			want: []contracts.DesiredRecord{{
				Hostname: "app",
				Answer:   "edge.example",
				Source:   contracts.SourceObjectRef{Provider: provider, ID: "ctr-1", DisplayName: "app"},
			}},
		},
		{
			name:          "keeps explicit overrides and omits local aliases without a host target",
			defaultTarget: "",
			container: containerSummary("ctr-2", "/svc", map[string]string{
				"proxy.aliases": "api, admin",
				"proxy.#1.port": "8080",
				"proxy.#2.port": "9090",
				"proxy.#2.host": "admin.internal",
			}, "running", "172.18.0.20"),
			want: []contracts.DesiredRecord{
				{Hostname: "admin", Answer: "admin.internal", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-2", DisplayName: "svc"}},
			},
		},
		{
			name:          "preserves original alias positions for indexed host overrides after filtering",
			defaultTarget: "",
			container: containerSummary("ctr-2b", "/svc", map[string]string{
				"proxy.aliases": "skip, admin",
				"proxy.#2.port": "9090",
				"proxy.#2.host": "admin.internal",
			}, "running", "172.18.0.21"),
			want: []contracts.DesiredRecord{
				{Hostname: "admin", Answer: "admin.internal", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-2b", DisplayName: "svc"}},
			},
		},
		{
			name:          "supports named aliases with wildcard and explicit host precedence",
			defaultTarget: "docker.example",
			container: containerSummary("ctr-3", "/frontend", map[string]string{
				"proxy.app.port": "8080",
				"proxy.www.port": "8080",
				"proxy.*.host":   "shared.internal",
				"proxy.www.host": "www.internal",
			}, "running", "172.18.0.30"),
			want: []contracts.DesiredRecord{
				{Hostname: "app", Answer: "shared.internal", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-3", DisplayName: "frontend"}},
				{Hostname: "www", Answer: "www.internal", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-3", DisplayName: "frontend"}},
			},
		},
		{
			name:          "uses shared source host ip for unlabeled answers",
			defaultTarget: "192.168.1.50",
			container: containerSummary("ctr-4b", "/svc", map[string]string{
				"proxy.aliases": "app, admin",
				"proxy.#1.port": "8080",
				"proxy.#2.port": "9090",
			}, "running", "172.18.0.31"),
			want: []contracts.DesiredRecord{
				{Hostname: "admin", Answer: "192.168.1.50", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-4b", DisplayName: "svc"}},
				{Hostname: "app", Answer: "192.168.1.50", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-4b", DisplayName: "svc"}},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := deriveDesiredRecords(provider, tt.defaultTarget, "", tt.container)
			assertDesiredRecordsEqual(t, got, tt.want)
		})
	}
}

func TestDesiredRecordDerivation(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	container := containerSummary("ctr-4", "/edge", map[string]string{
		"proxy.aliases":  "api, edge",
		"proxy.*.port":   "8080",
		"proxy.api.host": "api.internal",
	}, "running", "172.18.0.40")

	got := deriveDesiredRecords(provider, "edge.example", "", container)
	want := []contracts.DesiredRecord{
		{Hostname: "api", Answer: "api.internal", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-4", DisplayName: "edge"}},
		{Hostname: "edge", Answer: "edge.example", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-4", DisplayName: "edge"}},
	}

	assertDesiredRecordsEqual(t, got, want)
}

func TestExcludedContainersProduceNoDesiredRecords(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	container := containerSummary("ctr-5", "/db", map[string]string{
		"proxy.exclude": "true",
		"proxy.aliases": "db",
		"proxy.db.port": "5432",
	}, "running", "172.18.0.50")

	got := deriveDesiredRecords(provider, "", "", container)
	if len(got) != 0 {
		t.Fatalf("expected no desired records, got %+v", got)
	}
}

func TestUnsupportedContainersProduceNoDesiredRecords(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	tests := []struct {
		name      string
		labels    map[string]string
		wantCount int
	}{
		{
			name: "aliases without supported port labels are ignored",
			labels: map[string]string{
				"proxy.aliases": "api, admin",
			},
		},
		{
			name: "host-only labels are ignored",
			labels: map[string]string{
				"proxy.api.host": "api.internal",
			},
		},
		{
			name: "exclude label alone does not trigger fallback hostname",
			labels: map[string]string{
				"proxy.exclude": "false",
			},
		},
		{
			name: "local aliases with matching port labels still need a host target",
			labels: map[string]string{
				"proxy.aliases": "api, admin",
				"proxy.#2.port": "8080",
			},
		},
	}

	for _, tt := range tests {
		caseData := tt
		t.Run(caseData.name, func(t *testing.T) {
			t.Parallel()

			got := deriveDesiredRecords(provider, "", "", containerSummary("ctr-6", "/svc", caseData.labels, "running", "172.18.0.60"))
			if len(got) != caseData.wantCount {
				t.Fatalf("expected %d desired records, got %d (%+v)", caseData.wantCount, len(got), got)
			}
			if caseData.wantCount == 1 && got[0].Hostname != "admin" {
				t.Fatalf("expected only admin alias to remain eligible, got %+v", got)
			}
		})
	}
}

func TestDesiredRecordDerivationAppendsBaseDomain(t *testing.T) {
	t.Parallel()

	provider := contracts.ProviderRef{Type: "docker", Name: "local"}
	container := containerSummary("ctr-7", "/edge", map[string]string{
		"proxy.aliases":  "api, app.example.com",
		"proxy.*.port":   "8080",
		"proxy.api.host": "api.internal",
	}, "running", "172.18.0.40")

	got := deriveDesiredRecords(provider, "edge.example", "bar.bz", container)
	want := []contracts.DesiredRecord{
		{Hostname: "api.bar.bz", Answer: "api.internal", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-7", DisplayName: "edge"}},
		{Hostname: "app.example.com", Answer: "edge.example", Source: contracts.SourceObjectRef{Provider: provider, ID: "ctr-7", DisplayName: "edge"}},
	}

	assertDesiredRecordsEqual(t, got, want)
}

func containerSummary(id, name string, labels map[string]string, state containertypes.ContainerState, ip string) containertypes.Summary {
	var settings *containertypes.NetworkSettingsSummary
	if ip != "" {
		settings = &containertypes.NetworkSettingsSummary{
			Networks: map[string]*network.EndpointSettings{
				"default": {IPAddress: netip.MustParseAddr(ip)},
			},
		}
	}

	return containertypes.Summary{
		ID:              id,
		Names:           []string{name},
		Labels:          labels,
		State:           state,
		NetworkSettings: settings,
	}
}

func assertDesiredRecordsEqual(t *testing.T, got, want []contracts.DesiredRecord) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d desired records, got %d (%+v)", len(want), len(got), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected desired record at %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}
