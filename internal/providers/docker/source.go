package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
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
	ref         contracts.ProviderRef
	endpoint    string
	defaultHost string
	baseDomain  string
	client      apiClient
	logger      *slog.Logger
}

func New(cfg config.SourceConfig, logger *slog.Logger) (*Provider, error) {
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
		endpoint:    cfg.Endpoint,
		defaultHost: defaultAnswerTarget(cfg),
		baseDomain:  normalizeName(cfg.BaseDomain),
		client:      cli,
		logger:      logger,
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
		requestErr := fmt.Errorf("list containers: %w", err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, requestErr
		}
		return nil, temporaryError{err: requestErr}
	}

	logDockerTrace(ctx, p.logger, "listed docker containers for desired record derivation",
		"provider", providerKey(p.ref),
	)

	desired := make([]contracts.DesiredRecord, 0)
	for _, container := range result.Items {
		if container.State != containertypes.StateRunning {
			logDockerTrace(ctx, p.logger, "skipped non-running docker container during desired record derivation",
				"provider", providerKey(p.ref),
				"container_id", strings.TrimSpace(container.ID),
				"display_name", containerDisplayName(container),
			)
			continue
		}

		records, diagnostics := deriveDesiredRecordsDetailed(p.ref, p.defaultHost, p.baseDomain, container)
		for _, diagnostic := range diagnostics {
			args := []any{
				"provider", providerKey(p.ref),
				"container_id", strings.TrimSpace(container.ID),
				"display_name", containerDisplayName(container),
				"reason", diagnostic.reason,
			}
			if diagnostic.alias != "" {
				args = append(args, "alias", diagnostic.alias)
			}
			if diagnostic.hostname != "" {
				args = append(args, "hostname", diagnostic.hostname)
			}
			if diagnostic.hint != "" {
				args = append(args, "hint", diagnostic.hint)
			}
			logDockerDebug(ctx, p.logger, "skipped docker DNS record derivation", args...)
		}

		logDockerTrace(ctx, p.logger, "derived desired records from docker container",
			"provider", providerKey(p.ref),
			"container_id", strings.TrimSpace(container.ID),
			"display_name", containerDisplayName(container),
		)

		desired = append(desired, records...)
	}

	sort.Slice(desired, func(i, j int) bool {
		return desiredRecordLess(desired[i], desired[j])
	})

	return desired, nil
}

const dockerTraceLevel slog.Level = slog.LevelDebug - 4

func providerKey(provider contracts.ProviderRef) string {
	return provider.Type + "/" + provider.Name
}

func logDockerDebug(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil || !logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	logger.DebugContext(ctx, msg, args...)
}

func logDockerTrace(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil || !logger.Enabled(ctx, dockerTraceLevel) {
		return
	}
	logger.Log(ctx, dockerTraceLevel, msg, args...)
}

func defaultAnswerTarget(cfg config.SourceConfig) string {
	if hostIP := normalizeAnswerTarget(cfg.HostIP); hostIP != "" {
		return hostIP
	}

	if isLocalEndpoint(cfg.Endpoint) {
		return ""
	}

	return endpointHost(cfg.Endpoint)
}

type temporaryError struct {
	err error
}

func (e temporaryError) Error() string {
	return e.err.Error()
}

func (e temporaryError) Unwrap() error {
	return e.err
}

func (e temporaryError) Temporary() bool {
	return true
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
		// Docker network events do not include the container labels we need for a
		// label-aware filter. We intentionally treat any container attach/detach as
		// a broad reconcile hint and rely on runtime debounce to coalesce bursts.
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
