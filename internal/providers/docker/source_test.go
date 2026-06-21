package docker

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	mobyclient "github.com/moby/moby/client"
)

func TestListDesired(t *testing.T) {
	t.Parallel()

	providerRef := contracts.ProviderRef{Type: "docker", Name: "local"}
	tests := []struct {
		name        string
		endpoint    string
		defaultHost string
		baseDomain  string
		containers  []containertypes.Summary
		want        []contracts.DesiredRecord
	}{
		{
			name:       "returns desired records for eligible running containers only",
			endpoint:   "tcp://edge.example:2375",
			baseDomain: "bar.bz",
			containers: []containertypes.Summary{
				containerSummary("ctr-z", "/skip-excluded", map[string]string{"proxy.exclude": "true", "proxy.aliases": "skip", "proxy.skip.port": "8080"}, "running", "172.18.0.99"),
				containerSummary("ctr-b", "/frontend", map[string]string{"proxy.aliases": "app, www", "proxy.*.port": "8080", "proxy.www.host": "www.internal"}, "running", "172.18.0.22"),
				containerSummary("ctr-a", "/api", map[string]string{"proxy.api.port": "9090"}, "running", "172.18.0.21"),
				containerSummary("ctr-c", "/stopped", map[string]string{"proxy.aliases": "old", "proxy.old.port": "7070"}, "exited", "172.18.0.23"),
				containerSummary("ctr-d", "/plain", map[string]string{"com.example.role": "noop"}, "running", "172.18.0.24"),
			},
			want: []contracts.DesiredRecord{
				{Hostname: "api.bar.bz", Answer: "edge.example", Source: contracts.SourceObjectRef{Provider: providerRef, ID: "ctr-a", DisplayName: "api"}},
				{Hostname: "app.bar.bz", Answer: "edge.example", Source: contracts.SourceObjectRef{Provider: providerRef, ID: "ctr-b", DisplayName: "frontend"}},
				{Hostname: "www.bar.bz", Answer: "www.internal", Source: contracts.SourceObjectRef{Provider: providerRef, ID: "ctr-b", DisplayName: "frontend"}},
			},
		},
		{
			name:     "uses configured remote endpoint as fallback answer target",
			endpoint: "tcp://docker.example:2375",
			containers: []containertypes.Summary{
				containerSummary("ctr-remote", "/edge", map[string]string{"proxy.aliases": "edge", "proxy.edge.port": "8080"}, "running", ""),
			},
			want: []contracts.DesiredRecord{
				{Hostname: "edge", Answer: "docker.example", Source: contracts.SourceObjectRef{Provider: providerRef, ID: "ctr-remote", DisplayName: "edge"}},
			},
		},
		{
			name:        "uses configured host ip for all unlabeled answers",
			endpoint:    "unix:///var/run/docker.sock",
			defaultHost: "192.168.1.50",
			containers: []containertypes.Summary{
				containerSummary("ctr-host-ip", "/edge", map[string]string{"proxy.aliases": "edge", "proxy.edge.port": "8080"}, "running", "172.18.0.44"),
			},
			want: []contracts.DesiredRecord{
				{Hostname: "edge", Answer: "192.168.1.50", Source: contracts.SourceObjectRef{Provider: providerRef, ID: "ctr-host-ip", DisplayName: "edge"}},
			},
		},
		{
			name:        "passes through proxy dns output targeting",
			endpoint:    "unix:///var/run/docker.sock",
			defaultHost: "192.168.1.50",
			containers: []containertypes.Summary{
				containerSummary("ctr-targeted", "/edge", map[string]string{"proxy.dns": "adguard", "proxy.aliases": "edge", "proxy.edge.port": "8080"}, "running", "172.18.0.44"),
			},
			want: []contracts.DesiredRecord{
				{Hostname: "edge", Answer: "192.168.1.50", Source: contracts.SourceObjectRef{Provider: providerRef, ID: "ctr-targeted", DisplayName: "edge"}, Output: "adguard"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := &Provider{
				ref:         providerRef,
				endpoint:    tt.endpoint,
				defaultHost: tt.defaultHost,
				baseDomain:  tt.baseDomain,
				client: &fakeDockerClient{
					host:       tt.endpoint,
					containers: tt.containers,
				},
			}
			if provider.defaultHost == "" {
				provider.defaultHost = defaultAnswerTarget(config.SourceConfig{Endpoint: tt.endpoint})
			}

			got, err := provider.ListDesired(context.Background())
			if err != nil {
				t.Fatalf("ListDesired returned error: %v", err)
			}

			assertDesiredRecordsEqual(t, got, tt.want)
		})
	}
}

func TestDefaultAnswerTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.SourceConfig
		want string
	}{
		{
			name: "uses configured host ip when set",
			cfg:  config.SourceConfig{Endpoint: "unix:///var/run/docker.sock", HostIP: "192.168.1.50"},
			want: "192.168.1.50",
		},
		{
			name: "uses remote endpoint host when host ip is unset",
			cfg:  config.SourceConfig{Endpoint: "tcp://docker.example:2375"},
			want: "docker.example",
		},
		{
			name: "keeps local endpoint empty when host ip is unset",
			cfg:  config.SourceConfig{Endpoint: "unix:///var/run/docker.sock"},
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := defaultAnswerTarget(tt.cfg); got != tt.want {
				t.Fatalf("defaultAnswerTarget(%+v) = %q, want %q", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestListDesiredReturnsClientErrors(t *testing.T) {
	t.Parallel()

	provider := &Provider{
		ref:      contracts.ProviderRef{Type: "docker", Name: "local"},
		endpoint: "unix:///var/run/docker.sock",
		client:   &fakeDockerClient{host: "unix:///var/run/docker.sock", err: errors.New("boom")},
	}

	_, err := provider.ListDesired(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWatchEmitsHintsForRelevantContainerEvents(t *testing.T) {
	t.Parallel()

	messageCh := make(chan events.Message, 4)
	errCh := make(chan error, 1)
	provider := &Provider{
		ref:      contracts.ProviderRef{Type: "docker", Name: "local"},
		endpoint: "unix:///var/run/docker.sock",
		client: &fakeDockerClient{
			host:          "unix:///var/run/docker.sock",
			eventMessages: messageCh,
			eventErrs:     errCh,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watch := provider.Watch(ctx)
	messageCh <- events.Message{Type: events.ContainerEventType, Action: "exec_start"}

	select {
	case <-watch.Hints:
		t.Fatal("received unexpected hint for irrelevant event")
	case <-time.After(50 * time.Millisecond):
	}

	messageCh <- events.Message{Type: events.ContainerEventType, Action: "start"}
	select {
	case <-watch.Hints:
		t.Fatal("received unexpected hint for unlabeled container event")
	case <-time.After(50 * time.Millisecond):
	}

	messageCh <- events.Message{Type: events.ContainerEventType, Action: "start", Actor: events.Actor{Attributes: map[string]string{"proxy.aliases": "app", "proxy.app.port": "8080"}}}
	select {
	case <-watch.Hints:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch hint for labeled container start")
	}

	messageCh <- events.Message{Type: events.NetworkEventType, Action: "connect"}
	select {
	case <-watch.Hints:
		t.Fatal("received unexpected hint for unrelated network connect")
	case <-time.After(50 * time.Millisecond):
	}

	messageCh <- events.Message{Type: events.NetworkEventType, Action: "connect", Actor: events.Actor{Attributes: map[string]string{"type": "container", "container": "ctr-1"}}}
	select {
	case <-watch.Hints:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch hint for network connect")
	}

	messageCh <- events.Message{Type: events.NetworkEventType, Action: "disconnect", Actor: events.Actor{Attributes: map[string]string{"type": "container", "container": "ctr-1"}}}
	select {
	case <-watch.Hints:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch hint for network disconnect")
	}

	messageCh <- events.Message{Type: events.ContainerEventType, Action: "start", Actor: events.Actor{Attributes: map[string]string{"proxy.api.port": "8080"}}}
	select {
	case <-watch.Hints:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch hint for labeled named-port container start")
	}
}

func TestShouldTriggerContainerWatchHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		labels map[string]string
		want   bool
	}{
		{
			name:   "ignores unlabeled container lifecycle events",
			action: "destroy",
			labels: map[string]string{"com.example.role": "noop"},
			want:   false,
		},
		{
			name:   "ignores explicitly excluded containers",
			action: "start",
			labels: map[string]string{"proxy.exclude": "true", "proxy.aliases": "app", "proxy.app.port": "8080"},
			want:   false,
		},
		{
			name:   "keeps alias-driven labeled container recovery",
			action: "start",
			labels: map[string]string{"proxy.aliases": "app", "proxy.app.port": "8080"},
			want:   true,
		},
		{
			name:   "keeps wildcard fallback labeled container recovery",
			action: "rename",
			labels: map[string]string{"proxy.*.port": "8080"},
			want:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shouldTriggerContainerWatchHint(events.Action(tt.action), tt.labels)
			if got != tt.want {
				t.Fatalf("shouldTriggerContainerWatchHint(%q, %#v) = %t, want %t", tt.action, tt.labels, got, tt.want)
			}
		})
	}
}

func TestShouldTriggerNetworkWatchHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     string
		attributes map[string]string
		want       bool
	}{
		{
			name:       "ignores unrelated network endpoint activity",
			action:     "connect",
			attributes: map[string]string{"type": "network", "name": "bridge"},
			want:       false,
		},
		{
			name:       "ignores container network events without container id",
			action:     "disconnect",
			attributes: map[string]string{"type": "container"},
			want:       false,
		},
		{
			name:       "keeps container attach recovery",
			action:     "connect",
			attributes: map[string]string{"type": "container", "container": "ctr-1"},
			want:       true,
		},
		{
			name:       "keeps container detach recovery",
			action:     "disconnect",
			attributes: map[string]string{"type": "container", "container": "ctr-1"},
			want:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shouldTriggerNetworkWatchHint(events.Action(tt.action), tt.attributes)
			if got != tt.want {
				t.Fatalf("shouldTriggerNetworkWatchHint(%q, %#v) = %t, want %t", tt.action, tt.attributes, got, tt.want)
			}
		})
	}
}

func TestWatchReturnsStreamErrors(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	provider := &Provider{
		ref:      contracts.ProviderRef{Type: "docker", Name: "local"},
		endpoint: "unix:///var/run/docker.sock",
		client: &fakeDockerClient{
			host:          "unix:///var/run/docker.sock",
			eventMessages: make(chan events.Message),
			eventErrs:     errCh,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watch := provider.Watch(ctx)
	errCh <- io.EOF

	select {
	case err := <-watch.Err:
		if err == nil {
			t.Fatal("expected watch error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch error")
	}
}

func TestWatchTreatsCleanStreamClosureAsDisconnect(t *testing.T) {
	t.Parallel()

	messageCh := make(chan events.Message)
	errCh := make(chan error)
	provider := &Provider{
		ref:      contracts.ProviderRef{Type: "docker", Name: "local"},
		endpoint: "unix:///var/run/docker.sock",
		client: &fakeDockerClient{
			host:          "unix:///var/run/docker.sock",
			eventMessages: messageCh,
			eventErrs:     errCh,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watch := provider.Watch(ctx)
	close(messageCh)
	close(errCh)

	select {
	case err := <-watch.Err:
		if err == nil {
			t.Fatal("expected watch disconnect error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch disconnect error")
	}
}

type fakeDockerClient struct {
	host          string
	containers    []containertypes.Summary
	err           error
	eventMessages <-chan events.Message
	eventErrs     <-chan error
}

func (f *fakeDockerClient) Close() error {
	return nil
}

func (f *fakeDockerClient) DaemonHost() string {
	return f.host
}

func (f *fakeDockerClient) ContainerList(context.Context, mobyclient.ContainerListOptions) (mobyclient.ContainerListResult, error) {
	if f.err != nil {
		return mobyclient.ContainerListResult{}, f.err
	}

	return mobyclient.ContainerListResult{Items: f.containers}, nil
}

func (f *fakeDockerClient) Events(context.Context, mobyclient.EventsListOptions) mobyclient.EventsResult {
	return mobyclient.EventsResult{Messages: f.eventMessages, Err: f.eventErrs}
}
