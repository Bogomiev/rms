package task

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"rms/internal/services"
	"time"
)

type runtimeStorage interface {
	TaskCurrent(context.Context, string, time.Time) ([]json.RawMessage, error)
	TaskTransition(context.Context, int64, string, int64) (json.RawMessage, error)
	TaskGenerate(context.Context, time.Time) error
}

func (s *Service) Current(ctx context.Context, store string, now time.Time) ([]json.RawMessage, error) {
	id, e := uuid.Parse(store)
	if e != nil || id == uuid.Nil {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.(runtimeStorage).TaskCurrent(ctx, id.String(), now)
	return v, services.PersonnelError(e)
}
func (s *Service) Transition(ctx context.Context, id int64, action string, user int64) (json.RawMessage, error) {
	target := map[string]string{"start": "in_progress", "cancel": "cancelled", "complete": "completed"}[action]
	if id <= 0 || user <= 0 || target == "" {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.(runtimeStorage).TaskTransition(ctx, id, target, user)
	return v, services.PersonnelError(e)
}
func (s *Service) Generate(ctx context.Context, now time.Time) error {
	return s.storage.(runtimeStorage).TaskGenerate(ctx, now)
}

// GenerateType reconciles today's tasks for a saved type only.
func (s *Service) GenerateType(ctx context.Context, id int64, now time.Time) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	err := s.storage.(interface {
		TaskGenerateType(context.Context, time.Time, int64) error
	}).TaskGenerateType(ctx, now, id)
	return services.PersonnelError(err)
}
