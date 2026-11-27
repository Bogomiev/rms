package messagequeue

import (
	"context"
	"errors"
	"log/slog"

	"rms/internal/domain/models"
	"rms/internal/services"
)

type Storage interface {
	AddMessage(context.Context, models.Message) (models.Message, error)
	SaveMessage(context.Context, models.Message) (models.Message, error)
	PendingMessages(context.Context, string) ([]models.Message, error)
	Message(context.Context, int64) (models.Message, error)
	SetMessageStatus(context.Context, int64, models.MessageStatus) error
}

type Service struct{ Storage }

func New(storage Storage) *Service { return &Service{storage} }

type Handler func(context.Context, int64) error

func (s *Service) Job(log *slog.Logger, handlers map[string]Handler) func(context.Context) error {
	return func(ctx context.Context) error {
		messages, err := s.PendingMessages(ctx, "")
		if err != nil {
			return err
		}
		for _, message := range messages {
			if err := ctx.Err(); err != nil {
				return err
			}
			handler := handlers[message.Handler]
			if handler == nil {
				log.Error("не задан обработчик для сообщения", "handler", message.Handler, "id", message.ID)
				continue
			}
			if err := handler(ctx, message.ID); err != nil && !errors.Is(err, services.ErrConflict) {
				log.Error("ошибка обработки сообщения", "handler", message.Handler, "id", message.ID, "error", err)
			}
		}
		return nil
	}
}
