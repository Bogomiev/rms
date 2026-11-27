package returnrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
	"rms/internal/services"
)

const Handler = "return_request"

type Transaction interface {
	GetReturn(context.Context, uuid.UUID, uuid.UUID) (models.ReturnRequest, error)
	ValidateReturn(context.Context, models.ReturnRequest) error
	SaveReturn(context.Context, models.ReturnRequest, bool) error
	SetReturnNumber(context.Context, uuid.UUID, string) error
	SetReturnStatus(context.Context, uuid.UUID, models.ReturnStatus) error
}

type Storage interface {
	ActiveReturnIDs(context.Context) ([]uuid.UUID, error)
	SyncReturnStatuses(context.Context, map[uuid.UUID]models.ReturnStatus) error
	SyncReturnStatus(context.Context, uuid.UUID, uuid.UUID, models.ReturnStatus, models.ReturnStatus) error
	GetReturn(context.Context, uuid.UUID, uuid.UUID) (models.ReturnRequest, error)
	ListReturns(context.Context, uuid.UUID, time.Time, time.Time, bool) ([]models.ReturnRequest, error)
	SubmitReturn(context.Context, models.Message, func(Transaction) error) error
	ProcessReturnMessage(context.Context, int64, func(context.Context, models.Message, Transaction) error) error
}

type Queue interface {
	AddMessage(context.Context, models.Message) (models.Message, error)
}

type OneC interface {
	ReturnRequestsStatuses(context.Context, []uuid.UUID) (map[uuid.UUID]models.ReturnStatus, error)
	GetReturnRequestStatus(context.Context, uuid.UUID) (models.ReturnStatus, error)
	SendReturn(context.Context, models.ReturnRequest, bool) (models.ReturnResult, error)
}

type OneCError struct{ Err error }

func (e *OneCError) Error() string { return e.Err.Error() }
func (e *OneCError) Unwrap() error { return e.Err }

type Service struct {
	storage  Storage
	queue    Queue
	oneC     OneC
	location *time.Location
}

func New(storage Storage, queue Queue, oneC OneC, location *time.Location) *Service {
	return &Service{storage, queue, oneC, location}
}

func (s *Service) List(ctx context.Context, store uuid.UUID, historyDays int) ([]models.ReturnRequest, error) {
	if store == uuid.Nil || historyDays > 0 {
		return nil, services.ErrInvalid
	}
	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	from := time.Time{}
	if historyDays >= -today.Year()*366 {
		from = today.AddDate(0, 0, historyDays)
	}
	return s.storage.ListReturns(ctx, store, from, today.AddDate(0, 0, 1), historyDays < 0)
}

func (s *Service) Get(ctx context.Context, store, id uuid.UUID) (models.ReturnRequest, error) {
	return s.storage.GetReturn(ctx, store, id)
}

func (s *Service) GetReturnRequestStatus(ctx context.Context, store, id uuid.UUID) (models.ReturnStatus, error) {
	request, err := s.Get(ctx, store, id)
	if err != nil {
		return "", err
	}
	status, err := s.oneC.GetReturnRequestStatus(ctx, id)
	if err != nil {
		return "", &OneCError{Err: err}
	}
	if err := s.storage.SyncReturnStatus(ctx, store, id, request.Status, status); err != nil {
		return "", err
	}
	return status, nil
}

func (s *Service) UpdateStatusesFromOneC(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	statuses, err := s.oneC.ReturnRequestsStatuses(ctx, ids)
	if err != nil {
		return &OneCError{Err: err}
	}
	return s.storage.SyncReturnStatuses(ctx, statuses)
}

func (s *Service) SyncActiveStatuses(ctx context.Context) error {
	ids, err := s.storage.ActiveReturnIDs(ctx)
	if err != nil {
		return err
	}
	return s.UpdateStatusesFromOneC(ctx, ids)
}

func (s *Service) Create(ctx context.Context, request models.ReturnRequest) (models.ReturnRequest, error) {
	request.Status = models.ReturnNew
	request.Date = time.Now()
	return s.save(ctx, request, true)
}

func (s *Service) Update(ctx context.Context, request models.ReturnRequest) (models.ReturnRequest, error) {
	return s.save(ctx, request, false)
}

func (s *Service) save(ctx context.Context, request models.ReturnRequest, create bool) (models.ReturnRequest, error) {
	if request.ID == uuid.Nil || request.StoreID == uuid.Nil || request.Seller == uuid.Nil ||
		request.Manager <= 0 || len(request.Items) == 0 || len(request.Items) > math.MaxInt16 {
		return models.ReturnRequest{}, services.ErrInvalid
	}
	marks := make(map[string]bool)
	products := make(map[uuid.UUID]bool)
	for i := range request.Items {
		item := &request.Items[i]
		if products[item.ProductID] || item.ProductID == uuid.Nil || item.Amount <= 0 || math.IsNaN(item.Amount) || math.IsInf(item.Amount, 0) ||
			(len(item.Marks) > 0 && float64(len(item.Marks)) != item.Amount) {
			return models.ReturnRequest{}, services.ErrInvalid
		}
		products[item.ProductID] = true
		item.Line = int16(i + 1)
		for _, mark := range item.Marks {
			if strings.TrimSpace(mark) == "" || marks[mark] {
				return models.ReturnRequest{}, services.ErrInvalid
			}
			marks[mark] = true
		}
	}
	request.ItemCount = len(request.Items)
	payload, err := json.Marshal(models.ReturnCommand{ID: request.ID, StoreID: request.StoreID})
	if err != nil {
		return models.ReturnRequest{}, err
	}
	err = s.storage.SubmitReturn(ctx, models.Message{Handler: Handler, Payload: payload}, func(tx Transaction) error {
		if !create {
			current, err := tx.GetReturn(ctx, request.StoreID, request.ID)
			if err != nil {
				return err
			}
			request.Status, request.Date, request.Number = current.Status, current.Date, current.Number
		}
		if err := tx.ValidateReturn(ctx, request); err != nil {
			return err
		}
		if err := tx.SaveReturn(ctx, request, create); err != nil {
			return err
		}
		if request.Status == models.ReturnProcessed || request.Status == models.ReturnCancelled {
			request.Status = models.ReturnSent
			return s.updateStatus(ctx, tx, request.ID, request.Status)
		}
		return nil
	})
	if err != nil {
		return models.ReturnRequest{}, err
	}
	return request, nil
}

func (s *Service) ChangeStatus(ctx context.Context, store, id uuid.UUID, status models.ReturnStatus) (models.ReturnRequest, error) {
	if !status.Valid() {
		return models.ReturnRequest{}, services.ErrInvalid
	}
	request, err := s.Get(ctx, store, id)
	if err != nil || request.Status == status {
		return request, err
	}
	if status == models.ReturnCancelled && !request.Status.CanCancel() {
		return request, services.ErrConflict
	}
	return s.submitStatus(ctx, models.ReturnCommand{ID: id, StoreID: store, Status: status})
}

func (s *Service) submitStatus(ctx context.Context, command models.ReturnCommand) (models.ReturnRequest, error) {
	payload, err := json.Marshal(command)
	if err != nil {
		return models.ReturnRequest{}, err
	}
	message, err := s.queue.AddMessage(ctx, models.Message{Handler: Handler, Payload: payload})
	if err != nil {
		return models.ReturnRequest{}, err
	}
	if err := s.Process(ctx, message.ID); err != nil {
		return models.ReturnRequest{}, err
	}
	return s.Get(ctx, command.StoreID, command.ID)
}

func (s *Service) Process(ctx context.Context, id int64) error {
	return s.storage.ProcessReturnMessage(ctx, id, func(ctx context.Context, message models.Message, tx Transaction) error {
		var command models.ReturnCommand
		if err := json.Unmarshal(message.Payload, &command); err != nil {
			return fmt.Errorf("%w: %v", services.ErrInvalid, err)
		}
		if message.Handler != Handler {
			return services.ErrInvalid
		}
		statusOnly := command.Status != ""
		request, err := tx.GetReturn(ctx, command.StoreID, command.ID)
		if err != nil {
			return err
		}
		if statusOnly {
			if request.Status == command.Status {
				return nil
			}
			if !command.Status.Valid() {
				return services.ErrInvalid
			}
			if command.Status == models.ReturnCancelled && !request.Status.CanCancel() {
				return services.ErrConflict
			}
			request.Status = command.Status
		}
		result, err := s.sendToOneC(ctx, request, statusOnly)
		if err != nil {
			return err
		}
		if statusOnly {
			return s.updateStatus(ctx, tx, request.ID, result.Status)
		}
		return s.saveOneCResult(ctx, tx, request.ID, result)
	})
}

func (s *Service) sendToOneC(ctx context.Context, request models.ReturnRequest, statusOnly bool) (models.ReturnResult, error) {
	result, err := s.oneC.SendReturn(ctx, request, statusOnly)
	if err != nil {
		return models.ReturnResult{}, &OneCError{Err: err}
	}
	if !result.Status.Valid() {
		return models.ReturnResult{}, &OneCError{Err: errors.New("Некорректный ответ 1С")}
	}
	return result, nil
}

func (s *Service) saveOneCResult(ctx context.Context, tx Transaction, id uuid.UUID, result models.ReturnResult) error {
	if result.Number != nil {
		if err := tx.SetReturnNumber(ctx, id, *result.Number); err != nil {
			return err
		}
	}
	return s.updateStatus(ctx, tx, id, result.Status)
}

func (s *Service) updateStatus(ctx context.Context, tx Transaction, id uuid.UUID, status models.ReturnStatus) error {
	return tx.SetReturnStatus(ctx, id, status)
}
