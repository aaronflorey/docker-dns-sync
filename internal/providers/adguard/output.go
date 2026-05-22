package adguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aaronflorey/docker-dns-sync/internal/config"
	"github.com/aaronflorey/docker-dns-sync/internal/contracts"
)

type Provider struct {
	ref      contracts.ProviderRef
	baseURL  string
	username string
	password string
	client   *http.Client
}

func New(cfg config.OutputConfig) *Provider {
	return &Provider{
		ref: contracts.ProviderRef{
			Type: cfg.Type,
			Name: cfg.Name,
		},
		baseURL:  cfg.URL,
		username: cfg.Username,
		password: cfg.Password,
		client:   http.DefaultClient,
	}
}

func (p *Provider) Provider() contracts.ProviderRef {
	return p.ref
}

func (p *Provider) ListVisible(ctx context.Context) ([]contracts.VisibleRecord, error) {
	var rewrites []rewriteItem
	if err := p.requestJSON(ctx, http.MethodGet, "/control/rewrite/list", nil, &rewrites); err != nil {
		return nil, err
	}

	visible := make([]contracts.VisibleRecord, 0, len(rewrites))
	for _, rewrite := range rewrites {
		visible = append(visible, contracts.VisibleRecord{
			Output:   p.ref,
			Hostname: strings.TrimSpace(rewrite.Domain),
			Answer:   strings.TrimSpace(rewrite.Answer),
		})
	}

	return visible, nil
}

func (p *Provider) Create(ctx context.Context, desired contracts.DesiredRecord) error {
	payload := rewriteItem{Domain: desired.Hostname, Answer: desired.Answer}
	return p.requestJSON(ctx, http.MethodPost, "/control/rewrite/add", payload, nil)
}

func (p *Provider) Update(ctx context.Context, visible contracts.VisibleRecord, desired contracts.DesiredRecord) error {
	payload := rewriteUpdateRequest{
		Target: rewriteItem{Domain: visible.Hostname, Answer: visible.Answer},
		Update: rewriteItem{Domain: desired.Hostname, Answer: desired.Answer},
	}
	return p.requestJSON(ctx, http.MethodPut, "/control/rewrite/update", payload, nil)
}

func (p *Provider) Delete(ctx context.Context, visible contracts.VisibleRecord) error {
	payload := rewriteItem{Domain: visible.Hostname, Answer: visible.Answer}
	return p.requestJSON(ctx, http.MethodPost, "/control/rewrite/delete", payload, nil)
}

type rewriteItem struct {
	Domain string `json:"domain"`
	Answer string `json:"answer"`
}

type rewriteUpdateRequest struct {
	Target rewriteItem `json:"target"`
	Update rewriteItem `json:"update"`
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

func (p *Provider) requestJSON(ctx context.Context, method, path string, reqBody any, out any) error {
	if strings.TrimSpace(p.baseURL) == "" {
		return errors.New("adguard base URL is required")
	}

	var body io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode adguard request: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.baseURL, "/")+path, body)
	if err != nil {
		return fmt.Errorf("build adguard request: %w", err)
	}
	req.SetBasicAuth(p.username, p.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		requestErr := fmt.Errorf("adguard request %s %s failed: %w", method, path, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return requestErr
		}
		return temporaryError{err: requestErr}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		requestErr := fmt.Errorf("adguard request %s %s failed with status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(payload)))
		if isTemporaryStatus(resp.StatusCode) {
			return temporaryError{err: requestErr}
		}
		return requestErr
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode adguard response: %w", err)
		}
	}

	return nil
}

func isTemporaryStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout || statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}
