package delivery

import (
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

type Job struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	EventID        uuid.UUID
	Status         string
	Attempts       int
	LastError      *string
	CompletedAt    *time.Time
	CreatedAt      time.Time
}

type JobRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	EventID        uuid.UUID
	Status         string
	CreatedAt      time.Time
}
