package vectoramp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// IntelligenceService runs retrieval-augmented question answering.
type IntelligenceService struct{ client *Client }

// AskOption customizes an AskRequest built from convenience inputs.
type AskOption func(*AskRequest)

// WithDataset adds one dataset ID to an intelligence query's scope. Repeating it
// widens the scope rather than replacing it.
func WithDataset(datasetID string) AskOption {
	return func(r *AskRequest) { r.DatasetIDs = append(r.DatasetIDs, datasetID) }
}

// WithDatasets sets the exact dataset scope of an intelligence query, replacing
// anything set before it.
func WithDatasets(datasetIDs ...string) AskOption {
	return func(r *AskRequest) { r.DatasetIDs = append([]string(nil), datasetIDs...) }
}

// WithAllDatasets clears the scope so the query reaches every accessible
// dataset. That is what an absent dataset_ids means to the API; the old "all"
// sentinel is retired.
func WithAllDatasets() AskOption { return func(r *AskRequest) { r.DatasetIDs = nil } }

// WithTopK sets the number of retrieval chunks considered by intelligence.
func WithTopK(k int) AskOption { return func(r *AskRequest) { r.TopK = k } }

// WithSources controls whether source citations are included in the response.
func WithSources(include bool) AskOption { return func(r *AskRequest) { r.IncludeSources = &include } }

// WithHistory attaches prior conversation turns to an intelligence query.
func WithHistory(h []ConversationMessage) AskOption {
	return func(r *AskRequest) { r.ConversationHistory = h }
}

// Ask runs a non-streaming intelligence query.
//
// input may be a string query, AskRequest, or *AskRequest. Options override the
// normalized request. It returns the answer plus optional source citations,
// chunks, message, and metadata supplied by the API.
func (s *IntelligenceService) Ask(ctx context.Context, input interface{}, opts ...AskOption) (*AskResponse, error) {
	req, err := normalizeAskRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	req.Stream = false
	var out AskResponse
	err = s.client.do(ctx, "POST", "/intelligence/query", nil, req, &out)
	return &out, err
}

// Stream runs a streaming intelligence query and returns an SSE reader.
//
// req.Query is required. DatasetIDs, TopK, ConversationHistory, and
// IncludeSources are optional. The SDK forces req.Stream to true.
func (s *IntelligenceService) Stream(ctx context.Context, req AskRequest) (*AskStream, error) {
	req.Stream = true
	req.DatasetIDs = normalizeDatasetIDs(req.DatasetIDs)
	rc, err := s.client.stream(ctx, "POST", "/intelligence/query", req)
	if err != nil {
		return nil, err
	}
	return &AskStream{rc: rc, scanner: bufio.NewScanner(rc)}, nil
}

// AskStream reads server-sent events from a streaming intelligence response.
type AskStream struct {
	rc      io.ReadCloser
	scanner *bufio.Scanner
	event   string
	pending []byte
	err     error
}

// Close closes the underlying streaming response body.
func (s *AskStream) Close() error { return s.rc.Close() }

// Err returns the first decode or scanner error observed by the stream.
func (s *AskStream) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.scanner.Err()
}

// Next returns the next stream event.
//
// The boolean is false when the stream is exhausted. Call Err after Next returns
// false to distinguish EOF from a scan error.
func (s *AskStream) Next() (*StreamEvent, bool) {
	var data []string
	event := ""
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if line == "" {
			if len(data) > 0 {
				return decodeStreamEvent(event, []byte(strings.Join(data, "\n")))
			}
			event = ""
			data = nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(data) > 0 {
		return decodeStreamEvent(event, []byte(strings.Join(data, "\n")))
	}
	return nil, false
}
func decodeStreamEvent(event string, raw []byte) (*StreamEvent, bool) {
	se := &StreamEvent{Event: event, Raw: raw}
	_ = json.Unmarshal(raw, se)
	return se, true
}

func normalizeAskRequest(input interface{}, opts ...AskOption) (AskRequest, error) {
	var req AskRequest
	switch v := input.(type) {
	case AskRequest:
		req = v
	case *AskRequest:
		if v == nil {
			return AskRequest{}, fmt.Errorf("vectoramp: ask request is nil")
		}
		req = *v
	case string:
		req.Query = v
	default:
		return AskRequest{}, fmt.Errorf("vectoramp: unsupported ask input %T", input)
	}
	for _, opt := range opts {
		opt(&req)
	}
	req.DatasetIDs = normalizeDatasetIDs(req.DatasetIDs)
	return req, nil
}

// normalizeDatasetIDs drops blanks and the retired "all" sentinel. An empty
// scope is omitted from the request, which is how the API says "every dataset
// the caller can see".
func normalizeDatasetIDs(ids []string) []string {
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == "all" {
			continue
		}
		out = append(out, id)
	}
	return out
}

// CreateSession creates a persistent Intelligence session.
func (s *IntelligenceService) CreateSession(ctx context.Context, req SessionCreateRequest) (*IntelligenceSession, error) {
	var out IntelligenceSession
	err := s.client.do(ctx, "POST", "/intelligence/sessions", nil, req, &out)
	return &out, err
}

// ListSessions lists persistent Intelligence sessions. Pass limit <= 0 to use the API default.
func (s *IntelligenceService) ListSessions(ctx context.Context, limit int) (*SessionList, error) {
	q := makeLimitQuery(limit)
	var out SessionList
	err := s.client.do(ctx, "GET", "/intelligence/sessions", q, nil, &out)
	return &out, err
}

// GetSession fetches one persistent Intelligence session.
func (s *IntelligenceService) GetSession(ctx context.Context, id string) (*IntelligenceSession, error) {
	var out IntelligenceSession
	err := s.client.do(ctx, "GET", "/intelligence/sessions/"+urlPathEscape(id), nil, nil, &out)
	return &out, err
}

// DeleteSession deletes a persistent Intelligence session.
func (s *IntelligenceService) DeleteSession(ctx context.Context, id string) error {
	return s.client.do(ctx, "DELETE", "/intelligence/sessions/"+urlPathEscape(id), nil, nil, nil)
}

// AppendMessage appends a message to a persistent Intelligence session.
func (s *IntelligenceService) AppendMessage(ctx context.Context, sessionID string, req SessionMessageCreateRequest) (*SessionMessage, error) {
	var out SessionMessage
	err := s.client.do(ctx, "POST", "/intelligence/sessions/"+urlPathEscape(sessionID)+"/messages", nil, req, &out)
	return &out, err
}

// ListMessages lists messages in a persistent Intelligence session. Pass limit <= 0 to use the API default.
func (s *IntelligenceService) ListMessages(ctx context.Context, sessionID string, limit int) (*MessageList, error) {
	q := makeLimitQuery(limit)
	var out MessageList
	err := s.client.do(ctx, "GET", "/intelligence/sessions/"+urlPathEscape(sessionID)+"/messages", q, nil, &out)
	return &out, err
}

func makeLimitQuery(limit int) url.Values {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return q
}

func urlPathEscape(value string) string { return url.PathEscape(value) }
