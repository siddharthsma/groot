package delivery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

type StoreReader interface {
	ListDeliveryJobs(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, int) ([]Job, error)
	ListDeliveryJobsAdmin(context.Context, tenant.ID, string, *time.Time, *time.Time, int) ([]Job, error)
	GetDeliveryJobForTenant(context.Context, tenant.ID, uuid.UUID) (Job, error)
	ResetDeliveryJob(context.Context, tenant.ID, uuid.UUID) (Job, error)
}

type RetryMetrics interface {
	IncDeliveryRetries()
}

type Service struct {
	store   StoreReader
	metrics RetryMetrics
}

func NewService(store StoreReader, metrics RetryMetrics) *Service {
	return &Service{store: store, metrics: metrics}
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID, status string, subscriptionID, eventID *uuid.UUID, limit int) ([]Job, error) {
	jobs, err := s.store.ListDeliveryJobs(ctx, tenantID, strings.TrimSpace(status), subscriptionID, eventID, normalizeListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list delivery jobs: %w", err)
	}
	return jobs, nil
}

func (s *Service) AdminList(ctx context.Context, tenantID tenant.ID, status string, from, to *time.Time, limit int) ([]Job, error) {
	jobs, err := s.store.ListDeliveryJobsAdmin(ctx, tenantID, strings.TrimSpace(status), from, to, normalizeListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list admin delivery jobs: %w", err)
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
	if s.metrics != nil {
		s.metrics.IncDeliveryRetries()
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
