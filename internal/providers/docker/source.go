package docker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aaronlmathis/docker-dns-sync/internal/config"
	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	mobyclient "github.com/moby/moby/client"
)

type apiClient interface {
	Close() error
	DaemonHost() string
	ContainerList(context.Context, mobyclient.ContainerListOptions) (mobyclient.ContainerListResult, error)
	Events(context.Context, mobyclient.EventsListOptions) mobyclient.EventsResult
}

type Provider struct {
	ref      contracts.ProviderRef
	endpoint string
	client   apiClient
}

func New(cfg config.SourceConfig) (*Provider, error) {
	cli, err := mobyclient.NewClientWithOpts(
		mobyclient.WithHost(cfg.Endpoint),
		mobyclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return &Provider{
		ref: contracts.ProviderRef{
			Type: cfg.Type,
			Name: cfg.Name,
		},
		endpoint: cfg.Endpoint,
		client:   cli,
	}, nil
}

func (p *Provider) Provider() contracts.ProviderRef {
	return p.ref
}

func (p *Provider) Endpoint() string {
	return p.endpoint
}

func (p *Provider) ClientHost() string {
	if p.client == nil {
		return ""
	}

	return p.client.DaemonHost()
}

func (p *Provider) ListDesired(ctx context.Context) ([]contracts.DesiredRecord, error) {
	result, err := p.client.ContainerList(ctx, mobyclient.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	desired := make([]contracts.DesiredRecord, 0)
	for _, container := range result.Items {
		if container.State != containertypes.StateRunning {
			continue
		}

		desired = append(desired, deriveDesiredRecords(p.ref, p.endpoint, container)...)
	}

	sort.Slice(desired, func(i, j int) bool {
		return desiredRecordLess(desired[i], desired[j])
	})

	return desired, nil
}

func (p *Provider) Watch(ctx context.Context) contracts.SourceWatch {
	hints := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(hints)
		defer close(errCh)

		stream := p.client.Events(ctx, mobyclient.EventsListOptions{})
		for stream.Messages != nil || stream.Err != nil {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-stream.Messages:
				if !ok {
					stream.Messages = nil
					if stream.Err == nil && ctx.Err() == nil {
						errCh <- fmt.Errorf("docker event stream ended")
						return
					}
					continue
				}
				if !shouldTriggerWatchHint(event) {
					continue
				}
				select {
				case hints <- struct{}{}:
				default:
				}
			case err, ok := <-stream.Err:
				if !ok {
					stream.Err = nil
					if stream.Messages == nil && ctx.Err() == nil {
						errCh <- fmt.Errorf("docker event stream ended")
						return
					}
					continue
				}
				if err == nil || err == context.Canceled {
					return
				}
				if err == io.EOF {
					err = fmt.Errorf("docker event stream ended: %w", err)
				}
				errCh <- err
				return
			}
		}
	}()

	return contracts.SourceWatch{Hints: hints, Err: errCh}
}

func shouldTriggerWatchHint(event events.Message) bool {
	switch event.Type {
	case events.ContainerEventType:
		return shouldTriggerContainerWatchHint(event.Action, event.Actor.Attributes)
	case events.NetworkEventType:
		return shouldTriggerNetworkWatchHint(event.Action, event.Actor.Attributes)
	default:
		return false
	}
}

func shouldTriggerNetworkWatchHint(action events.Action, attributes map[string]string) bool {
	switch action {
	case "connect", "disconnect":
		return attributes["type"] == "container" && attributes["container"] != ""
	default:
		return false
	}
}

func shouldTriggerContainerWatchHint(action events.Action, labels map[string]string) bool {
	switch action {
	case "create", "start", "stop", "die", "destroy", "rename", "update":
		return hasRelevantWatchLabels(labels)
	default:
		return false
	}
}

func hasRelevantWatchLabels(labels map[string]string) bool {
	if len(labels) == 0 || isExcluded(labels) {
		return false
	}

	for i, alias := range strings.Split(labels["proxy.aliases"], ",") {
		alias = normalizeName(alias)
		if alias == "" {
			continue
		}
		if hasPortForAlias(labels, alias, i+1) {
			return true
		}
	}

	for key, value := range labels {
		if _, ok := supportedNamedPortAlias(key); ok && normalizePortValue(value) != "" {
			return true
		}
	}

	return hasWildcardOrIndexedPort(labels)
}
