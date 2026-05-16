package vectoramp

import (
	"context"
	"fmt"
)

// ScheduleService manages recurring ingestion schedules.
//
// A schedule pairs a source with a target dataset and a cron expression. The
// server's ingestion scheduler daemon polls for due schedules and creates jobs
// as they fire.
type ScheduleService struct{ client *Client }

// Schedule is a recurring ingestion schedule returned by the API.
type Schedule struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id,omitempty"`
	SourceID       string                 `json:"source_id,omitempty"`
	DatasetID      string                 `json:"dataset_id,omitempty"`
	PipelineID     string                 `json:"pipeline_id,omitempty"`
	Cron           string                 `json:"cron,omitempty"`
	Timezone       string                 `json:"timezone,omitempty"`
	Enabled        bool                   `json:"enabled"`
	NextRunAt      string                 `json:"next_run_at,omitempty"`
	LastRunAt      string                 `json:"last_run_at,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      string                 `json:"created_at,omitempty"`
	UpdatedAt      string                 `json:"updated_at,omitempty"`
}

// ScheduleList is a paginated collection of schedules.
type ScheduleList struct {
	Schedules []Schedule `json:"schedules"`
	Total     int        `json:"total"`
	Limit     int        `json:"limit"`
	Offset    int        `json:"offset"`
}

// Pagination returns the list pagination metadata as a common struct.
func (p ScheduleList) Pagination() Pagination {
	return Pagination{Total: p.Total, Limit: p.Limit, Offset: p.Offset}
}

// CreateScheduleRequest is the request body for creating a schedule.
//
// SourceID, DatasetID, and Cron are required by the API. Timezone defaults to
// UTC server-side. PipelineID is optional; omit to use the default ingestion
// pipeline. Enabled defaults to true server-side; set explicitly via Enabled to
// override.
type CreateScheduleRequest struct {
	SourceID   string                 `json:"source_id"`
	DatasetID  string                 `json:"dataset_id"`
	Cron       string                 `json:"cron"`
	Timezone   string                 `json:"timezone,omitempty"`
	PipelineID string                 `json:"pipeline_id,omitempty"`
	Enabled    *bool                  `json:"enabled,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateScheduleRequest is a partial update payload. Only non-zero fields are
// sent; pass Enabled as a pointer to disambiguate "set false" from "leave alone".
type UpdateScheduleRequest struct {
	Cron       string                 `json:"cron,omitempty"`
	Timezone   string                 `json:"timezone,omitempty"`
	PipelineID string                 `json:"pipeline_id,omitempty"`
	Enabled    *bool                  `json:"enabled,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// TriggerScheduleResponse describes the run started by an immediate trigger.
type TriggerScheduleResponse struct {
	JobID string `json:"job_id,omitempty"`
}

// List returns ingestion schedules using optional limit and offset pagination.
//
// Pass zero for limit or offset to omit that query parameter.
func (s *ScheduleService) List(ctx context.Context, limit, offset int) (*ScheduleList, error) {
	var out ScheduleList
	err := s.client.do(ctx, "GET", "/ingestion/schedules", paginationQuery(limit, offset), nil, &out)
	return &out, err
}

// Get returns one schedule by id.
func (s *ScheduleService) Get(ctx context.Context, scheduleID string) (*Schedule, error) {
	var out Schedule
	err := s.client.do(ctx, "GET", fmt.Sprintf("/ingestion/schedules/%s", scheduleID), nil, nil, &out)
	return &out, err
}

// Create creates a schedule.
func (s *ScheduleService) Create(ctx context.Context, req CreateScheduleRequest) (*Schedule, error) {
	var out Schedule
	err := s.client.do(ctx, "POST", "/ingestion/schedules", nil, req, &out)
	return &out, err
}

// Update applies a partial update to a schedule.
func (s *ScheduleService) Update(ctx context.Context, scheduleID string, req UpdateScheduleRequest) (*Schedule, error) {
	var out Schedule
	err := s.client.do(ctx, "PATCH", fmt.Sprintf("/ingestion/schedules/%s", scheduleID), nil, req, &out)
	return &out, err
}

// Delete deletes a schedule.
func (s *ScheduleService) Delete(ctx context.Context, scheduleID string) error {
	return s.client.do(ctx, "DELETE", fmt.Sprintf("/ingestion/schedules/%s", scheduleID), nil, nil, nil)
}

// Trigger requests an immediate run for a schedule, outside its cron cadence.
func (s *ScheduleService) Trigger(ctx context.Context, scheduleID string) (*TriggerScheduleResponse, error) {
	var out TriggerScheduleResponse
	err := s.client.do(ctx, "POST", fmt.Sprintf("/ingestion/schedules/%s/trigger", scheduleID), nil, nil, &out)
	return &out, err
}
