package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/services/messagequeue"
	"rms/internal/services/returnrequest"
)

type returnSender func(context.Context, models.ReturnRequest, bool) (models.ReturnResult, error)

func (f returnSender) SendReturn(ctx context.Context, request models.ReturnRequest, statusOnly bool) (models.ReturnResult, error) {
	return f(ctx, request, statusOnly)
}

func (f returnSender) GetReturnRequestStatus(context.Context, uuid.UUID) (models.ReturnStatus, error) {
	return models.ReturnProcessed, nil
}

func (f returnSender) ReturnRequestsStatuses(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]models.ReturnStatus, error) {
	statuses := make(map[uuid.UUID]models.ReturnStatus, len(ids))
	for _, id := range ids {
		statuses[id] = models.ReturnCompleted
	}
	return statuses, nil
}

func TestReturnQueueRetry(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	store, product, marked := uuid.New(), uuid.New(), uuid.New()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO stores(id, code, name, address) VALUES ($1,'1','Store','Address')`, []any{store}},
		{`INSERT INTO users(id, name, user_token, password) VALUES (1,'Manager','manager','hash')`, nil},
		{`INSERT INTO products(id, parent_id, code, name, marking_type)
            VALUES ($1,$3,'1','Plain','БезОсобенностейУчета'),($2,$3,'2','Marked','Маркировка')`, []any{product, marked, uuid.Nil}},
	} {
		if _, err := db.db.ExecContext(ctx, seed.sql, seed.args...); err != nil {
			t.Fatal(err)
		}
	}

	queue := messagequeue.New(db)
	failure := errors.New("1С временно недоступна")
	calls := 0
	service := returnrequest.New(db, queue, returnSender(func(_ context.Context, request models.ReturnRequest, statusOnly bool) (models.ReturnResult, error) {
		calls++
		if calls == 1 {
			return models.ReturnResult{}, failure
		}
		if statusOnly {
			return models.ReturnResult{Status: request.Status}, nil
		}
		number := "00042"
		return models.ReturnResult{Number: &number, Status: models.ReturnSent}, nil
	}), time.UTC)

	request := models.ReturnRequest{ID: uuid.New(), StoreID: store, Manager: 1, Seller: uuid.New(),
		Items: []models.ReturnItem{{ProductID: product, Amount: 1.5}, {ProductID: marked, Amount: 2, Marks: []string{"mark-a", "mark-b"}}}}
	result, err := service.Create(ctx, request)
	if err != nil || calls != 0 || result.Status != models.ReturnNew || result.Number != nil {
		t.Fatalf("create must return a saved request without calling 1C: %+v, calls %d, error %v", result, calls, err)
	}
	var status models.ReturnStatus
	if err := db.db.QueryRowContext(ctx, `SELECT status FROM return_request WHERE id=$1`, request.ID).Scan(&status); err != nil || status != models.ReturnNew {
		t.Fatalf("expected NEW in return_request, got %q, error %v", status, err)
	}
	if _, err := db.db.ExecContext(ctx, `UPDATE return_request SET status='INVALID' WHERE id=$1`, request.ID); err == nil {
		t.Fatal("return_request accepted an invalid status")
	}
	if err := service.Process(ctx, 1); !errors.Is(err, failure) {
		t.Fatalf("expected 1C error, got %v", err)
	}
	if saved, err := service.Get(ctx, store, request.ID); err != nil || saved.Status != models.ReturnNew {
		t.Fatalf("request must remain stored after the 1C error: %+v, error %v", saved, err)
	}

	message, err := queue.Message(ctx, 1)
	if err != nil || message.Attempts != 1 || message.Status != models.MessagePending ||
		message.ErrorMessage == nil || *message.ErrorMessage != failure.Error() {
		t.Fatalf("unexpected message %+v, error %v", message, err)
	}
	message.AvailableAt = time.Now().Add(-time.Minute)
	if _, err := queue.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	pending, err := queue.PendingMessages(ctx, "")
	if err != nil || len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("unexpected pending messages %+v, error %v", pending, err)
	}
	if err := service.Process(ctx, message.ID); err != nil {
		t.Fatal(err)
	}
	result, err = service.Get(ctx, store, request.ID)
	if err != nil || result.Status != models.ReturnSent || result.Number == nil || *result.Number != "00042" ||
		result.ItemCount != 2 || result.Items[0].Amount != 1.5 || result.Items[1].Amount != 2 || len(result.Items[1].Marks) != 2 {
		t.Fatalf("unexpected stored request %+v, error %v", result, err)
	}
	if synced, err := service.GetReturnRequestStatus(ctx, store, request.ID); err != nil || synced != models.ReturnProcessed {
		t.Fatalf("expected PROCESSED from status sync, got %q, error %v", synced, err)
	}
	if saved, err := service.Get(ctx, store, request.ID); err != nil || saved.Status != models.ReturnProcessed {
		t.Fatalf("expected PROCESSED in return_request, got %q, error %v", saved.Status, err)
	}
	request.Items[0].Amount = 3
	result, err = service.Update(ctx, request)
	if err != nil || calls != 2 || result.Status != models.ReturnSent || result.Items[0].Amount != 3 {
		t.Fatalf("update must save without calling 1C: %+v, calls %d, error %v", result, calls, err)
	}
	if err := db.db.QueryRowContext(ctx, `SELECT status FROM return_request WHERE id=$1`, request.ID).Scan(&status); err != nil || status != models.ReturnSent {
		t.Fatalf("expected SENT before processing the queued update, got %q, error %v", status, err)
	}
	pending, err = queue.PendingMessages(ctx, "")
	if err != nil || len(pending) != 1 {
		t.Fatalf("expected one queued update: %+v, error %v", pending, err)
	}
	if err := service.Process(ctx, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeStatus(ctx, store, request.ID, models.ReturnCancelled); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeStatus(ctx, store, request.ID, models.ReturnCancelled); err != nil || calls != 4 {
		t.Fatalf("same status must not call 1C, got %d calls, error %v", calls, err)
	}
	if err := db.db.QueryRowContext(ctx, `SELECT status FROM return_request WHERE id=$1`, request.ID).Scan(&status); err != nil || status != models.ReturnCancelled {
		t.Fatalf("expected CANCELLED in return_request, got %q, error %v", status, err)
	}
	for days, count := range map[int]int{0: 0, -3: 1} {
		list, err := service.List(ctx, store, days)
		if err != nil || len(list) != count {
			t.Fatalf("history_days=%d returned %+v, error %v", days, list, err)
		}
	}
	request.Items[0].Amount = 4
	result, err = service.Update(ctx, request)
	if err != nil || result.Status != models.ReturnSent || calls != 4 {
		t.Fatalf("resuming must queue without calling 1C: %+v, calls %d, error %v", result, calls, err)
	}
	saved, err := service.Get(ctx, store, request.ID)
	if err != nil || saved.Status != models.ReturnSent || saved.Items[0].Amount != 4 {
		t.Fatalf("expected saved SENT request before processing: %+v, error %v", saved, err)
	}
	pending, err = queue.PendingMessages(ctx, "")
	if err != nil || len(pending) != 1 {
		t.Fatalf("expected one queued resume: %+v, error %v", pending, err)
	}
	if err := service.SyncActiveStatuses(ctx); err != nil {
		t.Fatal(err)
	}
	saved, err = service.Get(ctx, store, request.ID)
	if err != nil || saved.Status != models.ReturnCompleted {
		t.Fatalf("expected COMPLETED after batch sync: %+v, error %v", saved, err)
	}
	ids, err := db.ActiveReturnIDs(ctx)
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected no active returns: %v, error %v", ids, err)
	}
}

func TestReturnQueueStopsAtMaxAttempts(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	queue := messagequeue.New(db)
	message, err := queue.AddMessage(ctx, models.Message{
		Handler: returnrequest.Handler, Payload: []byte(`{}`), MaxAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	maxAttempts := message.MaxAttempts
	for attempt := int16(1); attempt <= maxAttempts; attempt++ {
		err := db.ProcessReturnMessage(ctx, message.ID, func(context.Context, models.Message, returnrequest.Transaction) error {
			calls++
			return services.ErrInvalid
		})
		if !errors.Is(err, services.ErrInvalid) {
			t.Fatalf("attempt %d: expected handler error, got %v", attempt, err)
		}
		message, err = queue.Message(ctx, message.ID)
		if err != nil || message.Attempts != attempt {
			t.Fatalf("attempt %d: unexpected message %+v, error %v", attempt, message, err)
		}
		if attempt < maxAttempts && message.Status != models.MessagePending {
			t.Fatalf("message should be pending before the limit: %+v", message)
		}
		message.AvailableAt = time.Now().Add(-time.Minute)
		message, err = queue.SaveMessage(ctx, message)
		if err != nil {
			t.Fatal(err)
		}
	}
	if message.Status != models.MessageFailed {
		t.Fatalf("message should fail at the limit: %+v", message)
	}

	// The listing must enforce the attempt limit even if the status is pending.
	if err := queue.SetMessageStatus(ctx, message.ID, models.MessagePending); err != nil {
		t.Fatal(err)
	}
	pending, err := queue.PendingMessages(ctx, "")
	if err != nil || len(pending) != 0 || calls != 2 {
		t.Fatalf("expected two calls and no pending messages, got %d calls, %+v, error %v", calls, pending, err)
	}
}

func TestReturnQueueCountsCancelledAttempt(t *testing.T) {
	db := authTestStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	message, err := db.AddMessage(ctx, models.Message{Handler: returnrequest.Handler, Payload: []byte(`{}`), MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}

	err = db.ProcessReturnMessage(ctx, message.ID, func(ctx context.Context, _ models.Message, _ returnrequest.Transaction) error {
		cancel()
		return ctx.Err()
	})
	if err != context.Canceled {
		t.Fatalf("expected the handler error after recording the attempt, got %v", err)
	}

	message, err = db.Message(context.Background(), message.ID)
	if err != nil || message.Attempts != 1 || message.Status != models.MessagePending ||
		message.ErrorMessage == nil || *message.ErrorMessage != context.Canceled.Error() {
		t.Fatalf("expected a recorded failed attempt, got %+v, error %v", message, err)
	}
}
