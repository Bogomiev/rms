package task

import (
	"context"
	"encoding/json"
	"errors"
	"rms/internal/services"
	"rms/internal/storage"
	"testing"
	"time"
)

type manualStub struct {
	Storage
	store string
	calls int
	err   error
}

func (s *manualStub) TaskTypesAvailable(context.Context) ([]json.RawMessage, error) {
	return []json.RawMessage{}, s.err
}
func (s *manualStub) TaskCreateManual(_ context.Context, _ int64, store string, _ time.Time, _ int64) (json.RawMessage, error) {
	s.store, s.calls = store, s.calls+1
	return json.RawMessage(`{}`), s.err
}

func TestCreateManualValidation(t *testing.T) {
	stub := &manualStub{}
	s := New(stub)
	if _, err := s.CreateManual(context.Background(), 1, "018E166F-4220-11F1-95F7-00155D1A2B22", 3, time.Now()); err != nil || stub.store != "018e166f-4220-11f1-95f7-00155d1a2b22" {
		t.Fatal(err, stub.store)
	}
	stub.calls = 0
	for _, tc := range []struct {
		typeID, user int64
		store        string
	}{{0, 3, "018e166f-4220-11f1-95f7-00155d1a2b22"}, {1, 0, "018e166f-4220-11f1-95f7-00155d1a2b22"}, {1, 3, "bad"}, {1, 3, "00000000-0000-0000-0000-000000000000"}} {
		if _, err := s.CreateManual(context.Background(), tc.typeID, tc.store, tc.user, time.Now()); !errors.Is(err, services.ErrInvalid) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	if stub.calls != 0 {
		t.Fatal("invalid input reached storage")
	}
	stub.err = storage.ErrNotFound
	if _, err := s.CreateManual(context.Background(), 9, "018e166f-4220-11f1-95f7-00155d1a2b22", 3, time.Now()); !errors.Is(err, services.ErrNotFound) {
		t.Fatal(err)
	}
}
func TestStoppersTaskTypeIsValid(t *testing.T) {
	if _, err := normalize("task_types", []byte(`{"name":"Стопперы","service":"stoppers","color":"#FBEADB","color_dark":"#3A2817"}`)); err != nil {
		t.Fatal(err)
	}
}
