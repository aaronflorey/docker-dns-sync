package runtime

import "strings"

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
