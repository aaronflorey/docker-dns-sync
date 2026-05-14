package docker

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/aaronlmathis/docker-dns-sync/internal/contracts"
	containertypes "github.com/moby/moby/api/types/container"
)

func deriveDesiredRecords(provider contracts.ProviderRef, endpoint string, container containertypes.Summary) []contracts.DesiredRecord {
	if isExcluded(container.Labels) {
		return nil
	}

	aliases := collectAliases(container)
	if len(aliases) == 0 {
		return nil
	}

	answer := defaultAnswerTarget(endpoint)

	source := contracts.SourceObjectRef{
		Provider:    provider,
		ID:          strings.TrimSpace(container.ID),
		DisplayName: containerDisplayName(container),
	}

	records := make([]contracts.DesiredRecord, 0, len(aliases))
	for _, alias := range aliases {
		hostname := alias.name
		if hostname == "" {
			continue
		}

		target := explicitHostOverride(container.Labels, hostname, alias.position)
		if target == "" {
			target = answer
		}
		if target == "" {
			continue
		}

		records = append(records, contracts.DesiredRecord{
			Hostname: hostname,
			Answer:   normalizeAnswerTarget(target),
			Source:   source,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return desiredRecordLess(records[i], records[j])
	})

	return records
}

type aliasRef struct {
	name     string
	position int
}

func collectAliases(container containertypes.Summary) []aliasRef {
	labels := container.Labels
	ordered := make([]aliasRef, 0)
	seen := make(map[string]struct{})

	for i, alias := range strings.Split(labels["proxy.aliases"], ",") {
		alias = normalizeName(alias)
		if alias == "" {
			continue
		}
		if !hasPortForAlias(labels, alias, i+1) {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		ordered = append(ordered, aliasRef{name: alias, position: i + 1})
	}

	discovered := make([]string, 0)
	for key := range labels {
		alias, ok := supportedNamedPortAlias(key)
		if !ok {
			continue
		}
		if normalizePortValue(labels[key]) == "" {
			continue
		}
		if _, exists := seen[alias]; exists {
			continue
		}
		seen[alias] = struct{}{}
		discovered = append(discovered, alias)
	}
	sort.Strings(discovered)
	for _, alias := range discovered {
		ordered = append(ordered, aliasRef{name: alias})
	}

	if len(ordered) == 0 && hasWildcardOrIndexedPort(labels) {
		fallback := normalizeName(containerDisplayName(container))
		if fallback != "" {
			ordered = append(ordered, aliasRef{name: fallback})
		}
	}

	return ordered
}

func explicitHostOverride(labels map[string]string, alias string, index int) string {
	if value := normalizeAnswerTarget(labels["proxy."+alias+".host"]); value != "" {
		return value
	}
	if index > 0 {
		if value := normalizeAnswerTarget(labels["proxy.#"+itoa(index)+".host"]); value != "" {
			return value
		}
	}
	return normalizeAnswerTarget(labels["proxy.*.host"])
}

func hasPortForAlias(labels map[string]string, alias string, index int) bool {
	if normalizePortValue(labels["proxy."+alias+".port"]) != "" {
		return true
	}
	if index > 0 {
		if normalizePortValue(labels["proxy.#"+itoa(index)+".port"]) != "" {
			return true
		}
	}
	return normalizePortValue(labels["proxy.*.port"]) != ""
}

func hasWildcardOrIndexedPort(labels map[string]string) bool {
	for key := range labels {
		if strings.HasPrefix(key, "proxy.*.") || strings.HasPrefix(key, "proxy.#") {
			if strings.HasSuffix(key, ".port") && normalizePortValue(labels[key]) != "" {
				return true
			}
		}
	}

	return false
}

func supportedNamedPortAlias(key string) (string, bool) {
	if !strings.HasPrefix(key, "proxy.") {
		return "", false
	}

	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "proxy" {
		return "", false
	}
	if parts[1] == "" || parts[1] == "*" || strings.HasPrefix(parts[1], "#") {
		return "", false
	}
	if parts[2] != "port" {
		return "", false
	}

	return normalizeName(parts[1]), true
}

func isExcluded(labels map[string]string) bool {
	value := strings.TrimSpace(strings.ToLower(labels["proxy.exclude"]))
	return value == "true"
}

func containerDisplayName(container containertypes.Summary) string {
	for _, name := range container.Names {
		trimmed := strings.TrimSpace(strings.TrimPrefix(name, "/"))
		if trimmed != "" {
			return trimmed
		}
	}

	return strings.TrimSpace(container.ID)
}

func defaultAnswerTarget(endpoint string) string {
	if isLocalEndpoint(endpoint) {
		return ""
	}

	return endpointHost(endpoint)
}

func isLocalEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "npipe://") || endpoint == ""
}

func endpointHost(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}

	return normalizeAnswerTarget(parsed.Hostname())
}

func desiredRecordLess(left, right contracts.DesiredRecord) bool {
	leftKey := left.Hostname + "|" + left.Answer + "|" + left.Source.ID
	rightKey := right.Hostname + "|" + right.Answer + "|" + right.Source.ID
	return leftKey < rightKey
}

func normalizeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.TrimSuffix(value, ".")
}

func normalizeAnswerTarget(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.TrimSuffix(value, ".")
}

func normalizePortValue(value string) string {
	return strings.TrimSpace(value)
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
