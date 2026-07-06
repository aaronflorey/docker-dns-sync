package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
	cloudflare "github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zones"
)

type Provider struct {
	ref    contracts.ProviderRef
	zoneID string
	client *cloudflare.Client

	zoneName string
	mu       sync.RWMutex
	visible  map[string]visibleRecordMeta
}

type visibleRecordMeta struct {
	id string
}

type temporaryError struct {
	err error
}

type sanitizedError struct {
	err     error
	message string
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

func (e sanitizedError) Error() string {
	return e.message
}

func (e sanitizedError) Unwrap() error {
	return e.err
}

func New(cfg config.OutputConfig) *Provider {
	return &Provider{
		ref:     contracts.ProviderRef{Type: cfg.Type, Name: cfg.Name},
		zoneID:  cfg.ZoneID,
		client:  cloudflare.NewClient(option.WithAPIToken(cfg.APIKey)),
		visible: make(map[string]visibleRecordMeta),
	}
}

func (p *Provider) Provider() contracts.ProviderRef {
	return p.ref
}

func (p *Provider) ListVisible(ctx context.Context) ([]contracts.VisibleRecord, error) {
	if strings.TrimSpace(p.zoneID) == "" {
		return nil, errors.New("cloudflare zone ID is required")
	}

	zoneName, err := p.ensureZoneName(ctx)
	if err != nil {
		return nil, err
	}

	iter := p.client.DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{ZoneID: cloudflare.F(p.zoneID)})
	visible := make([]contracts.VisibleRecord, 0)
	nextVisible := make(map[string]visibleRecordMeta)

	for iter.Next() {
		record := iter.Current()
		if !isSupportedRecordType(record.Type) {
			continue
		}

		hostname := normalizeHostname(record.Name)
		answer := normalizeAnswer(record.Content)
		visible = append(visible, contracts.VisibleRecord{
			Output:   p.ref,
			Hostname: hostname,
			Answer:   answer,
			Provenance: &contracts.RecordProvenance{
				RemoteID: record.ID,
			},
		})
		for _, key := range visibleRecordKeys(hostname, answer, zoneName) {
			nextVisible[key] = visibleRecordMeta{id: record.ID}
		}
	}

	if err := iter.Err(); err != nil {
		return nil, wrapCloudflareTemporary(fmt.Errorf("list cloudflare dns records: %w", err))
	}

	p.mu.Lock()
	p.visible = nextVisible
	p.mu.Unlock()

	return visible, nil
}

func (p *Provider) Create(ctx context.Context, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	body, err := buildRecordNewBody(desired.Hostname, desired.Answer)
	if err != nil {
		return nil, err
	}

	created, err := p.client.DNS.Records.New(ctx, dns.RecordNewParams{
		ZoneID: cloudflare.F(p.zoneID),
		Body:   body,
	})
	if err != nil {
		createErr := wrapCloudflareTemporary(fmt.Errorf("create cloudflare dns record: %w", err))
		if !isRecoverableDuplicateRecordError(err) {
			return nil, createErr
		}

		visible, listErr := p.ListVisible(ctx)
		if listErr != nil {
			return nil, listErr
		}

		zoneName := ""
		p.mu.RLock()
		zoneName = p.zoneName
		p.mu.RUnlock()

		matches := make([]contracts.VisibleRecord, 0, 1)
		for _, record := range visible {
			if hostnamesEquivalent(record.Hostname, desired.Hostname, zoneName) {
				matches = append(matches, record)
			}
		}
		if len(matches) != 1 {
			return nil, createErr
		}

		if normalizeAnswer(matches[0].Answer) == normalizeAnswer(desired.Answer) {
			return copyProvenance(matches[0].Provenance), nil
		}

		updated, updateErr := p.Update(ctx, matches[0], desired)
		if updateErr != nil {
			return nil, updateErr
		}

		return updated, nil
	}

	return &contracts.RecordProvenance{RemoteID: created.ID}, nil
}

func (p *Provider) Update(ctx context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) (*contracts.RecordProvenance, error) {
	recordID, err := p.visibleRemoteID(visible)
	if err != nil {
		return nil, err
	}

	body, err := buildRecordUpdateBody(desired.Hostname, desired.Answer)
	if err != nil {
		return nil, err
	}

	updated, err := p.client.DNS.Records.Update(ctx, recordID, dns.RecordUpdateParams{
		ZoneID: cloudflare.F(p.zoneID),
		Body:   body,
	})
	if err != nil {
		return nil, wrapCloudflareTemporary(fmt.Errorf("update cloudflare dns record: %w", err))
	}

	return &contracts.RecordProvenance{RemoteID: updated.ID}, nil
}

func (p *Provider) Delete(ctx context.Context, visible contracts.VisibleRecord) error {
	recordID, err := p.visibleRemoteID(visible)
	if err != nil {
		return err
	}

	_, err = p.client.DNS.Records.Delete(ctx, recordID, dns.RecordDeleteParams{ZoneID: cloudflare.F(p.zoneID)})
	if err != nil {
		return wrapCloudflareTemporary(fmt.Errorf("delete cloudflare dns record: %w", err))
	}

	return nil
}

func (p *Provider) visibleRemoteID(visible contracts.VisibleRecord) (string, error) {
	if visible.Provenance == nil || strings.TrimSpace(visible.Provenance.RemoteID) == "" {
		return "", fmt.Errorf("cloudflare visible record %s is missing remote provenance", visibleRecordKey(visible.Hostname, visible.Answer))
	}

	remoteID := strings.TrimSpace(visible.Provenance.RemoteID)
	if meta, ok := p.lookupVisibleRecord(visible.Hostname, visible.Answer); ok && meta.id != remoteID {
		return "", fmt.Errorf("cloudflare visible record %s remote provenance does not match cached metadata", visibleRecordKey(visible.Hostname, visible.Answer))
	}

	return remoteID, nil
}

func (p *Provider) lookupVisibleRecord(hostname, answer string) (visibleRecordMeta, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	meta, ok := p.visible[visibleRecordKey(hostname, answer)]
	return meta, ok
}

func (p *Provider) ensureZoneName(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.zoneName != "" {
		zoneName := p.zoneName
		p.mu.RUnlock()
		return zoneName, nil
	}
	p.mu.RUnlock()

	zone, err := p.client.Zones.Get(ctx, zones.ZoneGetParams{ZoneID: cloudflare.F(p.zoneID)})
	if err != nil {
		return "", wrapCloudflareTemporary(fmt.Errorf("get cloudflare zone details: %w", err))
	}

	zoneName := normalizeHostname(zone.Name)
	p.mu.Lock()
	p.zoneName = zoneName
	p.mu.Unlock()

	return zoneName, nil
}

func wrapCloudflareTemporary(err error) error {
	sanitized := sanitizeCloudflareError(err)
	if !isTemporaryCloudflareError(err) {
		return sanitized
	}

	return temporaryError{err: sanitized}
}

func sanitizeCloudflareError(err error) error {
	if err == nil {
		return nil
	}

	original := errorMessage(err)
	message := redactSecretMaterial(original)
	if message == original {
		return err
	}

	return sanitizedError{err: err, message: message}
}

func errorMessage(err error) (message string) {
	defer func() {
		if recover() != nil {
			message = fmt.Sprintf("%T", err)
		}
	}()

	return err.Error()
}

var cloudflareSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+[^\s,;]+`),
	regexp.MustCompile(`(?i)bearer\s+[^\s,;]+`),
}

func redactSecretMaterial(message string) string {
	redacted := message
	for _, pattern := range cloudflareSecretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[redacted]")
	}

	return redacted
}

func isTemporaryCloudflareError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *cloudflare.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 408 || apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isRecoverableDuplicateRecordError(err error) bool {
	var apiErr *cloudflare.Error
	if !errors.As(err, &apiErr) {
		return false
	}

	for _, code := range cloudflareErrorCodes(apiErr) {
		switch code {
		case 81053, 81054, 81057, 81058:
			return true
		}
	}

	return false
}

func cloudflareErrorCodes(apiErr *cloudflare.Error) []int64 {
	if apiErr == nil {
		return nil
	}

	codes := make([]int64, 0, len(apiErr.Errors))
	seen := make(map[int64]struct{}, len(apiErr.Errors))
	for _, item := range apiErr.Errors {
		appendCloudflareErrorCode(&codes, seen, item.Code)

		var raw struct {
			ErrorChain []struct {
				Code int64 `json:"code"`
			} `json:"error_chain"`
		}
		if err := json.Unmarshal([]byte(item.JSON.RawJSON()), &raw); err != nil {
			continue
		}
		for _, nested := range raw.ErrorChain {
			appendCloudflareErrorCode(&codes, seen, nested.Code)
		}
	}

	return codes
}

func appendCloudflareErrorCode(codes *[]int64, seen map[int64]struct{}, code int64) {
	if code == 0 {
		return
	}
	if _, ok := seen[code]; ok {
		return
	}
	seen[code] = struct{}{}
	*codes = append(*codes, code)
}

func buildRecordNewBody(hostname, answer string) (dns.RecordNewParamsBodyUnion, error) {
	name := normalizeHostname(hostname)
	content := normalizeAnswer(answer)
	if name == "" {
		return nil, errors.New("cloudflare record hostname is required")
	}
	if content == "" {
		return nil, errors.New("cloudflare record answer is required")
	}

	if ip := net.ParseIP(content); ip != nil {
		if ip.To4() != nil {
			return dns.ARecordParam{
				Name:    cloudflare.F(name),
				Content: cloudflare.F(content),
				TTL:     cloudflare.F(dns.TTL(1)),
				Type:    cloudflare.F(dns.ARecordTypeA),
			}, nil
		}

		return dns.AAAARecordParam{
			Name:    cloudflare.F(name),
			Content: cloudflare.F(content),
			TTL:     cloudflare.F(dns.TTL(1)),
			Type:    cloudflare.F(dns.AAAARecordTypeAAAA),
		}, nil
	}

	return dns.CNAMERecordParam{
		Name:    cloudflare.F(name),
		Content: cloudflare.F(content),
		TTL:     cloudflare.F(dns.TTL(1)),
		Type:    cloudflare.F(dns.CNAMERecordTypeCNAME),
	}, nil
}

func buildRecordUpdateBody(hostname, answer string) (dns.RecordUpdateParamsBodyUnion, error) {
	name := normalizeHostname(hostname)
	content := normalizeAnswer(answer)
	if name == "" {
		return nil, errors.New("cloudflare record hostname is required")
	}
	if content == "" {
		return nil, errors.New("cloudflare record answer is required")
	}

	if ip := net.ParseIP(content); ip != nil {
		if ip.To4() != nil {
			return dns.ARecordParam{
				Name:    cloudflare.F(name),
				Content: cloudflare.F(content),
				TTL:     cloudflare.F(dns.TTL(1)),
				Type:    cloudflare.F(dns.ARecordTypeA),
			}, nil
		}

		return dns.AAAARecordParam{
			Name:    cloudflare.F(name),
			Content: cloudflare.F(content),
			TTL:     cloudflare.F(dns.TTL(1)),
			Type:    cloudflare.F(dns.AAAARecordTypeAAAA),
		}, nil
	}

	return dns.CNAMERecordParam{
		Name:    cloudflare.F(name),
		Content: cloudflare.F(content),
		TTL:     cloudflare.F(dns.TTL(1)),
		Type:    cloudflare.F(dns.CNAMERecordTypeCNAME),
	}, nil
}

func isSupportedRecordType(value dns.RecordResponseType) bool {
	switch strings.ToUpper(strings.TrimSpace(string(value))) {
	case "A", "AAAA", "CNAME":
		return true
	default:
		return false
	}
}

func normalizeHostname(hostname string) string {
	value := strings.TrimSpace(strings.ToLower(hostname))
	return strings.TrimSuffix(value, ".")
}

func normalizeAnswer(answer string) string {
	value := strings.TrimSpace(strings.ToLower(answer))
	return strings.TrimSuffix(value, ".")
}

func relativeVisibleHostname(hostname, zoneName string) string {
	normalizedHostname := normalizeHostname(hostname)
	normalizedZone := normalizeHostname(zoneName)
	if normalizedHostname == "" || normalizedZone == "" || normalizedHostname == normalizedZone {
		return normalizedHostname
	}

	zoneSuffix := "." + normalizedZone
	if !strings.HasSuffix(normalizedHostname, zoneSuffix) {
		return normalizedHostname
	}

	// Collapse single-label records within the zone back to their short form so
	// reconcile keys match desired hostnames created without a base domain.
	relative := strings.TrimSuffix(normalizedHostname, zoneSuffix)
	if relative == "" || strings.Contains(relative, ".") {
		return normalizedHostname
	}

	return relative
}

func visibleRecordKeys(hostname, answer, zoneName string) []string {
	keys := []string{visibleRecordKey(hostname, answer)}
	relativeHostname := relativeVisibleHostname(hostname, zoneName)
	if relativeHostname != "" && relativeHostname != normalizeHostname(hostname) {
		keys = append(keys, visibleRecordKey(relativeHostname, answer))
	}

	return keys
}

func hostnamesEquivalent(left, right, zoneName string) bool {
	normalizedLeft := normalizeHostname(left)
	normalizedRight := normalizeHostname(right)
	if normalizedLeft == normalizedRight {
		return true
	}

	normalizedZone := normalizeHostname(zoneName)
	if normalizedZone == "" {
		return false
	}

	qualify := func(hostname string) string {
		if hostname == "" || hostname == normalizedZone || strings.Contains(hostname, ".") {
			return hostname
		}
		return hostname + "." + normalizedZone
	}

	return qualify(normalizedLeft) == qualify(normalizedRight)
}

func visibleRecordKey(hostname, answer string) string {
	return normalizeHostname(hostname) + "|" + normalizeAnswer(answer)
}

func copyProvenance(provenance *contracts.RecordProvenance) *contracts.RecordProvenance {
	if provenance == nil {
		return nil
	}

	copy := *provenance
	return &copy
}
