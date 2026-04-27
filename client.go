package vectoramp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Client is the root VectorAmp API client.
//
// Use NewClient to construct one with an API key. The Datasets, Ingestion,
// Sources, and Intelligence fields expose the public service APIs; Sources is
// an alias for Ingestion for source-management convenience.
type Client struct {
	rt           Transport
	transport    *RESTTransport
	Datasets     *DatasetService
	Ingestion    *IngestionService
	Sources      *IngestionService
	Intelligence *IntelligenceService
}

// NewClient returns a VectorAmp client configured with apiKey.
//
// By default the client uses DefaultBaseURL, http.DefaultClient, and the SDK
// user agent. Options such as WithBaseURL, WithHTTPClient, WithTransport, and
// WithUserAgent override those defaults. The returned client is ready to use.
func NewClient(apiKey string, opts ...Option) *Client {
	rest := &RESTTransport{BaseURL: mustURL(DefaultBaseURL), APIKey: apiKey, HTTPClient: http.DefaultClient, UserAgent: "vectoramp-go/0.1.0"}
	c := &Client{transport: rest, rt: rest}
	for _, opt := range opts {
		opt(c)
	}
	if c.rt == nil {
		c.rt = c.transport
	}
	c.Datasets = &DatasetService{client: c}
	c.Ingestion = &IngestionService{client: c}
	c.Sources = c.Ingestion
	c.Intelligence = &IntelligenceService{client: c}
	return c
}

// Ask runs an intelligence query across the configured dataset scope.
//
// query is the natural-language question. Options may set a dataset, use all
// datasets, adjust top_k, include source citations, or attach conversation
// history. It returns the generated answer and any citations/chunks included by
// the API.
func (c *Client) Ask(ctx context.Context, query string, opts ...AskOption) (*AskResponse, error) {
	req := AskRequest{Query: query, Stream: false}
	for _, opt := range opts {
		opt(&req)
	}
	return c.Intelligence.Ask(ctx, req)
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body interface{}, out interface{}) error {
	resp, err := c.rt.Do(ctx, &Request{Method: method, Path: path, Query: q, Body: body})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) download(ctx context.Context, method, path string, q url.Values) ([]byte, error) {
	resp, err := c.rt.Do(ctx, &Request{Method: method, Path: path, Query: q})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) stream(ctx context.Context, method, path string, body interface{}) (io.ReadCloser, error) {
	resp, err := c.rt.Do(ctx, &Request{Method: method, Path: path, Body: body, Stream: true})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, decodeAPIError(resp)
	}
	return resp.Body, nil
}

func decodeAPIError(resp *Response) error {
	b, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(b))
	var obj map[string]interface{}
	if json.Unmarshal(b, &obj) == nil {
		for _, k := range []string{"error", "message", "detail"} {
			if v, ok := obj[k].(string); ok {
				msg = v
				break
			}
		}
	}
	return &APIError{StatusCode: resp.StatusCode, Header: resp.Header, Body: b, Message: msg}
}

func paginationQuery(limit, offset int) url.Values {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	return q
}

func documentListQuery(opts DocumentListOptions) url.Values {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	return q
}
