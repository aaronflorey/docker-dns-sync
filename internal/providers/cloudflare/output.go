package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

		hostname := normalizeVisibleHostname(record.Name, zoneName)
		answer := normalizeAnswer(record.Content)
		visible = append(visible, contracts.VisibleRecord{
			Output:   p.ref,
			Hostname: hostname,
			Answer:   answer,
		})
		nextVisible[visibleRecordKey(hostname, answer)] = visibleRecordMeta{id: record.ID}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("list cloudflare dns records: %w", err)
	}

	p.mu.Lock()
	p.visible = nextVisible
	p.mu.Unlock()

	return visible, nil
}

func (p *Provider) Create(ctx context.Context, desired contracts.DesiredRecord) error {
	body, err := buildRecordNewBody(desired.Hostname, desired.Answer)
	if err != nil {
		return err
	}

	_, err = p.client.DNS.Records.New(ctx, dns.RecordNewParams{
		ZoneID: cloudflare.F(p.zoneID),
		Body:   body,
	})
	if err != nil {
		if isRecoverableDuplicateRecordError(err) {
			visible, listErr := p.findVisibleHostname(ctx, desired.Hostname)
			if listErr == nil {
				if normalizeAnswer(visible.Answer) == normalizeAnswer(desired.Answer) {
					return nil
				}
				if updateErr := p.Update(ctx, visible, desired); updateErr == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("create cloudflare dns record: %w", err)
	}

	return nil
}

func (p *Provider) Update(ctx context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) error {
	meta, ok := p.lookupVisibleRecord(visible.Hostname, visible.Answer)
	if !ok {
		return fmt.Errorf("cloudflare visible record %s is missing cached metadata", visibleRecordKey(visible.Hostname, visible.Answer))
	}

	body, err := buildRecordUpdateBody(desired.Hostname, desired.Answer)
	if err != nil {
		return err
	}

	_, err = p.client.DNS.Records.Update(ctx, meta.id, dns.RecordUpdateParams{
		ZoneID: cloudflare.F(p.zoneID),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("update cloudflare dns record: %w", err)
	}

	return nil
}

func (p *Provider) Delete(ctx context.Context, visible contracts.VisibleRecord) error {
	meta, ok := p.lookupVisibleRecord(visible.Hostname, visible.Answer)
	if !ok {
		return fmt.Errorf("cloudflare visible record %s is missing cached metadata", visibleRecordKey(visible.Hostname, visible.Answer))
	}

	_, err := p.client.DNS.Records.Delete(ctx, meta.id, dns.RecordDeleteParams{ZoneID: cloudflare.F(p.zoneID)})
	if err != nil {
		return fmt.Errorf("delete cloudflare dns record: %w", err)
	}

	return nil
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
		return "", fmt.Errorf("get cloudflare zone details: %w", err)
	}

	zoneName := normalizeHostname(zone.Name)
	p.mu.Lock()
	p.zoneName = zoneName
	p.mu.Unlock()

	return zoneName, nil
}

func (p *Provider) findVisibleRecord(ctx context.Context, hostname, answer string) (visibleRecordMeta, error) {
	if _, err := p.ListVisible(ctx); err != nil {
		return visibleRecordMeta{}, err
	}

	meta, ok := p.lookupVisibleRecord(hostname, answer)
	if !ok {
		return visibleRecordMeta{}, fmt.Errorf("cloudflare duplicate create recovery could not find %s", visibleRecordKey(hostname, answer))
	}

	return meta, nil
}

func (p *Provider) findVisibleHostname(ctx context.Context, hostname string) (contracts.VisibleRecord, error) {
	visible, err := p.ListVisible(ctx)
	if err != nil {
		return contracts.VisibleRecord{}, err
	}

	matches := make([]contracts.VisibleRecord, 0, 1)
	for _, record := range visible {
		if normalizeHostname(record.Hostname) == normalizeHostname(hostname) {
			matches = append(matches, record)
		}
	}
	if len(matches) != 1 {
		return contracts.VisibleRecord{}, fmt.Errorf("cloudflare duplicate create recovery could not uniquely find hostname %s", normalizeHostname(hostname))
	}

	return matches[0], nil
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

func normalizeVisibleHostname(hostname, zoneName string) string {
	_ = zoneName
	return normalizeHostname(hostname)
}

func visibleRecordKey(hostname, answer string) string {
	return normalizeHostname(hostname) + "|" + normalizeAnswer(answer)
}
