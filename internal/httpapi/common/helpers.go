package common

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"groot/internal/delivery"
	"groot/internal/schema"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
)

func OptionalTime(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func OptionalLimit(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	return strconv.Atoi(trimmed)
}

func OptionalUUID(value string) (*uuid.UUID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func OptionalUUIDValue(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func OptionalAdminLimit(value string, max int) (int, error) {
	limit, err := OptionalLimit(value)
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 50, nil
	}
	if limit > max {
		return max, nil
	}
	return limit, nil
}

func OptionalPositiveInt(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	if parsed < 1 {
		return 0, errors.New("must be at least 1")
	}
	return parsed, nil
}

func SubscriptionResponse(result subscription.Result) map[string]any {
	response := map[string]any{"subscription": result.Subscription}
	if len(result.Warnings) > 0 {
		response["warnings"] = result.Warnings
	}
	return response
}

func IsFilterValidationError(err error) bool {
	var filterErr subscriptionfilter.ValidationError
	return errors.As(err, &filterErr)
}

func IsSchemaReject(err error) bool {
	var rejectErr schema.RejectError
	return errors.As(err, &rejectErr)
}

func BearerToken(header string) string {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func MapJobs(jobs []delivery.Job) []map[string]any {
	result := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, MapJob(job))
	}
	return result
}

func MapJob(job delivery.Job) map[string]any {
	var replayOfEventID any
	if job.ReplayOfEventID != nil {
		replayOfEventID = job.ReplayOfEventID.String()
	}
	return map[string]any{
		"id":                 job.ID.String(),
		"subscription_id":    job.SubscriptionID.String(),
		"event_id":           job.EventID.String(),
		"is_replay":          job.IsReplay,
		"replay_of_event_id": replayOfEventID,
		"status":             job.Status,
		"attempts":           job.Attempts,
		"last_error":         job.LastError,
		"external_id":        job.ExternalID,
		"last_status_code":   job.LastStatusCode,
		"result_event_id":    OptionalUUIDValue(job.ResultEventID),
		"created_at":         job.CreatedAt,
		"completed_at":       job.CompletedAt,
	}
}

func ParseSubscriptionRequestFields(connectedAppIDRaw, functionDestinationIDRaw, connectionIDRaw, agentIDRaw, operationRaw string) (*uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, error) {
	var appID *uuid.UUID
	if strings.TrimSpace(connectedAppIDRaw) != "" {
		parsed, err := uuid.Parse(connectedAppIDRaw)
		if err != nil {
			return nil, nil, nil, nil, nil, errors.New("invalid connected_app_id")
		}
		appID = &parsed
	}
	var functionID *uuid.UUID
	if strings.TrimSpace(functionDestinationIDRaw) != "" {
		parsed, err := uuid.Parse(functionDestinationIDRaw)
		if err != nil {
			return nil, nil, nil, nil, nil, errors.New("invalid function_destination_id")
		}
		functionID = &parsed
	}
	var connectionID *uuid.UUID
	if strings.TrimSpace(connectionIDRaw) != "" {
		parsed, err := uuid.Parse(connectionIDRaw)
		if err != nil {
			return nil, nil, nil, nil, nil, errors.New("invalid connection_id")
		}
		connectionID = &parsed
	}
	var agentID *uuid.UUID
	if strings.TrimSpace(agentIDRaw) != "" {
		parsed, err := uuid.Parse(agentIDRaw)
		if err != nil {
			return nil, nil, nil, nil, nil, errors.New("invalid agent_id")
		}
		agentID = &parsed
	}
	var operation *string
	if strings.TrimSpace(operationRaw) != "" {
		trimmed := strings.TrimSpace(operationRaw)
		operation = &trimmed
	}
	return appID, functionID, connectionID, agentID, operation, nil
}

func MustRawMessage(input json.RawMessage) json.RawMessage {
	return input
}
