package runtime

import (
	"strings"

	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
)

func normalizeHostname(hostname string) string {
	value := strings.TrimSpace(strings.ToLower(hostname))
	return strings.TrimSuffix(value, ".")
}

func normalizeAnswer(answer string) string {
	return strings.TrimSpace(strings.ToLower(answer))
}

func visibleRecordKey(hostname, answer string) string {
	return normalizeHostname(hostname) + "|" + normalizeAnswer(answer)
}

func ownedLineageKey(providerType, providerName, sourceType, sourceName, sourceID, hostname string) string {
	return strings.TrimSpace(strings.ToLower(providerType)) + "|" +
		strings.TrimSpace(strings.ToLower(providerName)) + "|" +
		strings.TrimSpace(strings.ToLower(sourceType)) + "|" +
		strings.TrimSpace(strings.ToLower(sourceName)) + "|" +
		strings.TrimSpace(strings.ToLower(sourceID)) + "|" +
		normalizeHostname(hostname)
}

func ownedDisplayLineageKey(output contracts.ProviderRef, source contracts.SourceObjectRef, hostname string) string {
	return strings.TrimSpace(strings.ToLower(output.Type)) + "|" +
		strings.TrimSpace(strings.ToLower(output.Name)) + "|" +
		strings.TrimSpace(strings.ToLower(source.Provider.Type)) + "|" +
		strings.TrimSpace(strings.ToLower(source.Provider.Name)) + "|" +
		strings.TrimSpace(strings.ToLower(source.DisplayName)) + "|" +
		normalizeHostname(hostname)
}
