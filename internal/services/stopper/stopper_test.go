package stopper

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/storage"
	"testing"
	"time"
)

type storageStub struct {
	Storage
	timezone string
	input    models.StopperInput
	calls    int
	err      error
}

func (s *storageStub) AddStopper(_ context.Context, in models.StopperInput) (int64, error) {
	s.input, s.calls = in, s.calls+1
	return 7, s.err
}
func (s *storageStub) StopperPositions(_ context.Context, _ models.StopperPositionFilter, timezone string) ([]models.StopperPosition, error) {
	s.timezone, s.calls = timezone, s.calls+1
	return []models.StopperPosition{}, s.err
}
func (s *storageStub) SetPositionsStopper(context.Context, []models.StopperPositionKey, *int64) error {
	s.calls++
	return s.err
}
func (s *storageStub) SetPositionsPeriod(context.Context, []models.StopperPositionKey, string, *string) error {
	s.calls++
	return s.err
}

func TestAddStopperTrimsAndValidates(t *testing.T) {
	stub := &storageStub{}
	got, err := New(stub, "UTC").AddStopper(context.Background(), models.StopperInput{Name: "  Сезон ", Color: "#1F5FBF"})
	if err != nil || got.ID != 7 || got.Name != "Сезон" || stub.input.Name != "Сезон" {
		t.Fatal(got, err, stub.input)
	}
	for _, in := range []models.StopperInput{{Name: " ", Color: "#1F5FBF"}, {Name: "Сезон", Color: "blue"}, {Name: "Очень длинное имя стоппера!", Color: "#1F5FBF"}} {
		if _, err := New(stub, "UTC").AddStopper(context.Background(), in); !errors.Is(err, services.ErrInvalid) {
			t.Fatalf("%+v: %v", in, err)
		}
	}
	stub.err = storage.ErrConflict
	if _, err := New(stub, "UTC").AddStopper(context.Background(), models.StopperInput{Name: "ХИТ", Color: "#D7261E"}); !errors.Is(err, services.ErrConflict) {
		t.Fatal(err)
	}
}
func TestPositionsFilter(t *testing.T) {
	stub := &storageStub{}
	store := uuid.New()
	if _, err := New(stub, "Asia/Vladivostok").Positions(context.Background(), models.StopperPositionFilter{From: "2026-11-02", To: "2026-11-16", Stores: []uuid.UUID{store}}); err != nil || stub.timezone != "Asia/Vladivostok" {
		t.Fatal(err, stub.timezone)
	}
	stub.calls = 0
	for _, f := range []models.StopperPositionFilter{
		{From: "2026-11-16", To: "2026-11-02"},
		{From: "02.11.2026", To: "2026-11-16"},
		{From: "2026-11-02", To: "2026-11-16", Stores: []uuid.UUID{store, store}},
		{From: "2026-11-02", To: "2026-11-16", Stores: []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}},
	} {
		if _, err := New(stub, "UTC").Positions(context.Background(), f); !errors.Is(err, services.ErrInvalid) {
			t.Fatalf("%+v: %v", f, err)
		}
	}
	if stub.calls != 0 {
		t.Fatal("invalid filter reached storage")
	}
}
func TestPositionChangesValidation(t *testing.T) {
	stub := &storageStub{}
	s := New(stub, "UTC")
	doc, zero, stopper, bad := uuid.New(), uuid.Nil, int64(2), int64(0)
	key := models.StopperPositionKey{DocID: &doc, ProductID: uuid.New()}
	early, late := "2026-11-01", "2026-12-01"
	if err := s.SetPositionsStopper(context.Background(), models.StopperAssignInput{Positions: []models.StopperPositionKey{key}, StopperID: &stopper}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPositionsStopper(context.Background(), models.StopperAssignInput{Positions: []models.StopperPositionKey{{ProductID: uuid.New()}}}); err != nil {
		t.Fatal("manual position without stopper:", err)
	}
	if err := s.SetPositionsPeriod(context.Background(), models.StopperPeriodInput{Positions: []models.StopperPositionKey{key}, From: early, To: &late}); err != nil {
		t.Fatal(err)
	}
	stub.calls = 0
	for _, err := range []error{
		s.SetPositionsStopper(context.Background(), models.StopperAssignInput{}),
		s.SetPositionsStopper(context.Background(), models.StopperAssignInput{Positions: []models.StopperPositionKey{key}, StopperID: &bad}),
		s.SetPositionsStopper(context.Background(), models.StopperAssignInput{Positions: []models.StopperPositionKey{{DocID: &zero, ProductID: uuid.New()}}}),
		s.SetPositionsPeriod(context.Background(), models.StopperPeriodInput{Positions: []models.StopperPositionKey{key}, From: late, To: &early}),
		s.SetPositionsPeriod(context.Background(), models.StopperPeriodInput{Positions: []models.StopperPositionKey{key}}),
		s.AddManualPosition(context.Background(), models.StopperManualInput{ProductID: uuid.New(), From: early}),
	} {
		if !errors.Is(err, services.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if stub.calls != 0 {
		t.Fatal("invalid change reached storage")
	}
}

type taskStub struct {
	Storage
	today string
	calls int
}

func (s *taskStub) StopperTask(_ context.Context, id int64, today string, _ string) (models.StopperTask, error) {
	s.today, s.calls = today, s.calls+1
	return models.StopperTask{TaskID: id}, nil
}
func (s *taskStub) SetStopperTaskCheck(context.Context, int64, models.StopperTaskCheckInput, int64) (string, error) {
	s.calls++
	return "in_progress", nil
}
func TestTaskUsesSchedulerDay(t *testing.T) {
	stub := &taskStub{}
	// 20:00 UTC is already the next day in Vladivostok.
	if _, err := New(stub, "Asia/Vladivostok").Task(context.Background(), 5, time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)); err != nil || stub.today != "2026-11-14" {
		t.Fatal(err, stub.today)
	}
	if _, err := New(stub, "UTC").Task(context.Background(), 0, time.Now()); !errors.Is(err, services.ErrInvalid) {
		t.Fatal(err)
	}
}
func TestSetTaskCheckValidation(t *testing.T) {
	stub := &taskStub{}
	s := New(stub, "UTC")
	doc, zero := uuid.New(), uuid.Nil
	for _, result := range []string{"ok", "bad", "removed", ""} {
		if _, err := s.SetTaskCheck(context.Background(), 5, models.StopperTaskCheckInput{DocID: &doc, ProductID: uuid.New(), Result: result}, 2); err != nil {
			t.Fatal(result, err)
		}
	}
	stub.calls = 0
	for _, in := range []models.StopperTaskCheckInput{{ProductID: uuid.New(), Result: "done"}, {Result: "ok"}, {DocID: &zero, ProductID: uuid.New(), Result: "ok"}} {
		if _, err := s.SetTaskCheck(context.Background(), 5, in, 2); !errors.Is(err, services.ErrInvalid) {
			t.Fatalf("%+v: %v", in, err)
		}
	}
	if _, err := s.SetTaskCheck(context.Background(), 5, models.StopperTaskCheckInput{ProductID: uuid.New(), Result: "ok"}, 0); !errors.Is(err, services.ErrInvalid) {
		t.Fatal(err)
	}
	if stub.calls != 0 {
		t.Fatal("invalid check reached storage")
	}
}
