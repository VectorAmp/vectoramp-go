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

// DefaultBaseURL is the production VectorAmp API endpoint.
const DefaultBaseURL = "https://api.vectoramp.com"

// Transport sends SDK requests and returns raw SDK responses.
//
// Implement this interface to plug in a custom HTTP stack or test transport.
type Transport interface {
	Do(ctx context.Context, req *Request) (*Response, error)
}

// Request is the transport-level representation of an API request.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   interface{}
	Header http.Header
	Stream bool
}

// Response is the transport-level representation of an API response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// RESTTransport is the default JSON-over-HTTP VectorAmp transport.
type RESTTransport struct {
	BaseURL    *url.URL
	APIKey     string
	HTTPClient *http.Client
	UserAgent  string
}

// Option customizes a Client during construction.
type Option func(*Client)

// WithBaseURL sets the API base URL. The value is trimmed of trailing slashes.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.transport.BaseURL = mustURL(baseURL) }
}

// WithHTTPClient sets the HTTP client used by the default REST transport.
//
// Passing nil leaves the existing default unchanged.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.transport.HTTPClient = h
		}
	}
}

// WithTransport replaces the default REST transport.
//
// Passing nil leaves the existing transport unchanged.
func WithTransport(t Transport) Option {
	return func(c *Client) {
		if t != nil {
			c.rt = t
		}
	}
}

// WithUserAgent sets the User-Agent header used by the default REST transport.
func WithUserAgent(ua string) Option { return func(c *Client) { c.transport.UserAgent = ua } }

func mustURL(s string) *url.URL {
	u, err := url.Parse(strings.TrimRight(s, "/"))
	if err != nil {
		panic(err)
	}
	return u
}

// Do sends one HTTP request and returns the raw response.
//
// JSON bodies are marshaled automatically, X-API-Key is set from APIKey, and
// Stream requests use Accept: text/event-stream. If HTTPClient is nil, a client
// with a 30-second timeout is created.
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
