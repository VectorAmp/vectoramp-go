package vectoramp

import (
	"context"
	"net/url"
)

// OpenAIAPIKeySecretRef is the organization secret name used by the API for
// OpenAI embedding credentials.
const OpenAIAPIKeySecretRef = "emb:openai:api_key"

// OrgSecretService manages organization-scoped provider secrets.
type OrgSecretService struct{ client *Client }

// Put stores or updates an organization secret by name.
func (s *OrgSecretService) Put(ctx context.Context, name, value string) error {
	return s.client.do(ctx, "PUT", "/org-secrets/"+url.PathEscape(name), nil, map[string]string{"value": value}, nil)
}

// Has reports whether an organization secret exists.
func (s *OrgSecretService) Has(ctx context.Context, name string) error {
	return s.client.do(ctx, "GET", "/org-secrets/"+url.PathEscape(name), nil, nil, nil)
}

// PutOpenAIAPIKey stores or updates the organization OpenAI API key.
func (s *OrgSecretService) PutOpenAIAPIKey(ctx context.Context, apiKey string) error {
	return s.Put(ctx, OpenAIAPIKeySecretRef, apiKey)
}

// UpdateOpenAIAPIKey is an alias for PutOpenAIAPIKey; the API upserts the key.
func (s *OrgSecretService) UpdateOpenAIAPIKey(ctx context.Context, apiKey string) error {
	return s.PutOpenAIAPIKey(ctx, apiKey)
}

// HasOpenAIAPIKey reports whether an organization OpenAI API key is present.
func (s *OrgSecretService) HasOpenAIAPIKey(ctx context.Context) error {
	return s.Has(ctx, OpenAIAPIKeySecretRef)
}
