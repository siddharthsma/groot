package delivery

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusDeadLetter = "dead_letter"
)

var (
	ErrJobNotFound     = errors.New("delivery job not found")
	ErrRetryNotAllowed = errors.New("delivery job retry not allowed")
)

type Job struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	SubscriptionID  uuid.UUID
	EventID         uuid.UUID
	WorkflowRunID   *uuid.UUID
	WorkflowNodeID  string
	IsReplay        bool
	ReplayOfEventID *uuid.UUID
	ResultEventID   *uuid.UUID
	Status          string
	Attempts        int
	LastError       *string
	ExternalID      *string
	LastStatusCode  *int
	CompletedAt     *time.Time
	CreatedAt       time.Time
}

type JobRecord struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	SubscriptionID  uuid.UUID
	EventID         uuid.UUID
	WorkflowRunID   *uuid.UUID
	WorkflowNodeID  string
	IsReplay        bool
	ReplayOfEventID *uuid.UUID
	Status          string
	CreatedAt       time.Time
}
