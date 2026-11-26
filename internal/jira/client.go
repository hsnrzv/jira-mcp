package jira

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hsnrzv/jira-mcp/internal/config"
)

// ErrWriteDisabled is returned by any mutating call when write tools are disabled.
var ErrWriteDisabled = errors.New("write operations are disabled (set JIRA_ENABLE_WRITE_TOOLS=true to enable)")

const (
	userAgent    = "jira-mcp/1.0"
	maxBodyBytes = 10 << 20
)

// Client talks to Jira Data Center.
type Client struct {
	cfg  *config.Config
	http *http.Client
}

// NewClient builds a hardened HTTP client from cfg.
func NewClient(cfg *config.Config) (*Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CustomCACertPath != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		pem, err := os.ReadFile(cfg.CustomCACertPath)
		if err != nil {
			return nil, fmt.Errorf("read custom CA cert: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("custom CA cert file contains no valid PEM certificates")
		}
		tlsCfg.RootCAs = pool
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   time.Duration(cfg.HTTPTimeoutSec) * time.Second,
			Transport: tr,
			// Refuse redirects so the bearer token is never forwarded elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, q url.Values, body []byte) (*http.Request, error) {
	u := c.cfg.JiraURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.PersonalToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop URL, keep cause
		}
		return fmt.Errorf("jira request %s %s failed: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("read jira response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &JiraAPIError{}
		_ = json.Unmarshal(data, apiErr)
		apiErr.StatusCode = resp.StatusCode
		return apiErr
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode jira response: %w", err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) doWrite(ctx context.Context, method, path string, in, out any) error {
	if !c.cfg.EnableWriteTools {
		return ErrWriteDisabled
	}
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	req, err := c.newRequest(ctx, method, path, nil, body)
	if err != nil {
		return err
	}
	return c.do(req, out)
}
