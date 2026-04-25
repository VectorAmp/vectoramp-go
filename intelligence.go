package vectoramp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type IntelligenceService struct{ client *Client }

type AskOption func(*AskRequest)

func WithDataset(datasetID string) AskOption { return func(r *AskRequest) { r.DatasetID = datasetID } }
func WithAllDatasets() AskOption             { return func(r *AskRequest) { r.DatasetID = "all" } }
func WithTopK(k int) AskOption               { return func(r *AskRequest) { r.TopK = k } }
func WithSources(include bool) AskOption     { return func(r *AskRequest) { r.IncludeSources = &include } }
func WithHistory(h []ConversationMessage) AskOption {
	return func(r *AskRequest) { r.ConversationHistory = h }
}

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
func (s *IntelligenceService) Stream(ctx context.Context, req AskRequest) (*AskStream, error) {
	req.Stream = true
	rc, err := s.client.stream(ctx, "POST", "/intelligence/query", req)
	if err != nil {
		return nil, err
	}
	return &AskStream{rc: rc, scanner: bufio.NewScanner(rc)}, nil
}

type AskStream struct {
	rc      io.ReadCloser
	scanner *bufio.Scanner
	event   string
	pending []byte
	err     error
}

func (s *AskStream) Close() error { return s.rc.Close() }
func (s *AskStream) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.scanner.Err()
}
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
