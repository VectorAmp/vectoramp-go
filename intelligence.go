package vectoramp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// IntelligenceService runs retrieval-augmented question answering.
type IntelligenceService struct{ client *Client }

// AskOption customizes an AskRequest built from convenience inputs.
type AskOption func(*AskRequest)

// WithDataset scopes an intelligence query to one dataset ID.
func WithDataset(datasetID string) AskOption { return func(r *AskRequest) { r.DatasetID = datasetID } }

// WithAllDatasets scopes an intelligence query across all accessible datasets.
func WithAllDatasets() AskOption { return func(r *AskRequest) { r.DatasetID = "all" } }

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
// req.Query is required. DatasetID, TopK, ConversationHistory, and
// IncludeSources are optional. The SDK forces req.Stream to true.
func (s *IntelligenceService) Stream(ctx context.Context, req AskRequest) (*AskStream, error) {
	req.Stream = true
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
	return req, nil
}
