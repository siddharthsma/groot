package delivery

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

type StoreReader interface {
	ListDeliveryJobs(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, int) ([]Job, error)
	GetDeliveryJobForTenant(context.Context, tenant.ID, uuid.UUID) (Job, error)
	ResetDeliveryJob(context.Context, tenant.ID, uuid.UUID) (Job, error)
}

type Service struct {
	store StoreReader
}

func NewService(store StoreReader) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID, status string, subscriptionID, eventID *uuid.UUID, limit int) ([]Job, error) {
	jobs, err := s.store.ListDeliveryJobs(ctx, tenantID, strings.TrimSpace(status), subscriptionID, eventID, normalizeListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list delivery jobs: %w", err)
	}
	return jobs, nil
}

func (s *Service) Get(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (Job, error) {
	job, err := s.store.GetDeliveryJobForTenant(ctx, tenantID, jobID)
	if err != nil {
		return Job{}, fmt.Errorf("get delivery job: %w", err)
	}
	return job, nil
}

func (s *Service) Retry(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (Job, error) {
	current, err := s.store.GetDeliveryJobForTenant(ctx, tenantID, jobID)
	if err != nil {
		return Job{}, fmt.Errorf("get delivery job: %w", err)
	}
	if current.Status != StatusDeadLetter && current.Status != StatusFailed {
		return Job{}, ErrRetryNotAllowed
	}

	job, err := s.store.ResetDeliveryJob(ctx, tenantID, jobID)
	if err != nil {
		return Job{}, fmt.Errorf("retry delivery job: %w", err)
	}
	return job, nil
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
