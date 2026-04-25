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

type Client struct {
	rt           Transport
	transport    *RESTTransport
	Datasets     *DatasetService
	Ingestion    *IngestionService
	Intelligence *IntelligenceService
}

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
	c.Intelligence = &IntelligenceService{client: c}
	return c
}

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
