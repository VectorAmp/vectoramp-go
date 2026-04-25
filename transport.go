package vectoramp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.vectoramp.com"

type Transport interface {
	Do(ctx context.Context, req *Request) (*Response, error)
}

type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   interface{}
	Header http.Header
	Stream bool
}
type Response struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type RESTTransport struct {
	BaseURL    *url.URL
	APIKey     string
	HTTPClient *http.Client
	UserAgent  string
}

type Option func(*Client)

func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.transport.BaseURL = mustURL(baseURL) }
}
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.transport.HTTPClient = h
		}
	}
}
func WithTransport(t Transport) Option {
	return func(c *Client) {
		if t != nil {
			c.rt = t
		}
	}
}
func WithUserAgent(ua string) Option { return func(c *Client) { c.transport.UserAgent = ua } }

func mustURL(s string) *url.URL {
	u, err := url.Parse(strings.TrimRight(s, "/"))
	if err != nil {
		panic(err)
	}
	return u
}

func (t *RESTTransport) Do(ctx context.Context, r *Request) (*Response, error) {
	if t.HTTPClient == nil {
		t.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	base := *t.BaseURL
	path := strings.TrimLeft(r.Path, "/")
	base.Path = strings.TrimRight(base.Path, "/") + "/" + path
	q := base.Query()
	for k, vs := range r.Query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	base.RawQuery = q.Encode()
	var body io.Reader
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, base.String(), body)
	if err != nil {
		return nil, err
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if t.UserAgent != "" {
		req.Header.Set("User-Agent", t.UserAgent)
	}
	if t.APIKey != "" {
		req.Header.Set("X-API-Key", t.APIKey)
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	hresp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: hresp.StatusCode, Header: hresp.Header, Body: hresp.Body}, nil
}
