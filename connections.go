package vectoramp

import (
	"context"
	"fmt"
	"net/url"
)

// ConnectionService manages OAuth/provider connections used to authorize
// ingestion sources.
//
// Connections live at the gateway root (/connections), not under the
// /ingestion namespace, but share the same transport, base URL, and X-Api-Key
// auth as the rest of the client.
type ConnectionService struct{ client *Client }

// Connection is an OAuth/provider connection returned by the API.
//
// AuthorizationURL, when present on a freshly created connection, is the URL the
// end user must visit to authorize the connection before it becomes active.
type Connection struct {
	ID               string `json:"id"`
	Provider         string `json:"provider"`
	Status           string `json:"status,omitempty"`
	AuthorizationURL string `json:"authorization_url,omitempty"`
	SourceType       string `json:"source_type,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// ConnectionList is the collection of connections returned by List.
type ConnectionList struct {
	Connections []Connection `json:"connections"`
}

// ListConnectionsOption customizes a connection List request.
type ListConnectionsOption func(*listConnectionsOptions)

type listConnectionsOptions struct {
	provider string
}

// WithConnectionProvider filters listed connections by provider.
func WithConnectionProvider(provider string) ListConnectionsOption {
	return func(o *listConnectionsOptions) { o.provider = provider }
}

// List returns connections, optionally filtered by provider.
func (s *ConnectionService) List(ctx context.Context, opts ...ListConnectionsOption) (*ConnectionList, error) {
	var o listConnectionsOptions
	for _, opt := range opts {
		opt(&o)
	}
	var q url.Values
	if o.provider != "" {
		q = url.Values{}
		q.Set("provider", o.provider)
	}
	var out ConnectionList
	err := s.client.do(ctx, "GET", "/connections", q, nil, &out)
	return &out, err
}

// CreateConnectionOption customizes a connection Create request.
type CreateConnectionOption func(*createConnectionOptions)

type createConnectionOptions struct {
	sourceType string
}

// WithConnectionSourceType associates the new connection with a source type.
func WithConnectionSourceType(sourceType string) CreateConnectionOption {
	return func(o *createConnectionOptions) { o.sourceType = sourceType }
}

// Create starts a new connection for provider and returns it.
//
// The returned Connection includes an AuthorizationURL the end user must visit
// to authorize the connection when the provider requires interactive consent.
func (s *ConnectionService) Create(ctx context.Context, provider string, opts ...CreateConnectionOption) (*Connection, error) {
	var o createConnectionOptions
	for _, opt := range opts {
		opt(&o)
	}
	body := map[string]interface{}{"provider": provider}
	if o.sourceType != "" {
		body["source_type"] = o.sourceType
	}
	var out Connection
	err := s.client.do(ctx, "POST", "/connections", nil, body, &out)
	return &out, err
}

// Get returns one connection by ID.
func (s *ConnectionService) Get(ctx context.Context, id string) (*Connection, error) {
	var out Connection
	err := s.client.do(ctx, "GET", fmt.Sprintf("/connections/%s", id), nil, nil, &out)
	return &out, err
}

// Delete deletes a connection by ID.
func (s *ConnectionService) Delete(ctx context.Context, id string) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/connections/%s", id), nil, nil, nil)
}
