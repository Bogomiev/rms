package pricetag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/storage"
)

type stub struct {
	Storage
	calls int
	err   error
}

func (s *stub) SetPriceTag(context.Context, int64, int64, models.PriceTagItemInput) error {
	s.calls++
	return s.err
}
func (s *stub) SetPriceTagsPrinted(context.Context, int64, []int64, int64, time.Time) (string, error) {
	s.calls++
	return "in_progress", s.err
}
func (s *stub) PriceTagProducts(context.Context, int64, string, int) ([]models.PriceTagProduct, error) {
	s.calls++
	return nil, s.err
}
func (s *stub) AddPriceTag(context.Context, int64, uuid.UUID, time.Time) error {
	s.calls++
	return s.err
}
func (s *stub) DeletePriceTag(context.Context, int64, int64, int64) (string, error) {
	s.calls++
	return "in_progress", s.err
}
func (s *stub) PriceTagProduct(context.Context, string, uuid.UUID, time.Time) (models.PriceTagProductState, error) {
	s.calls++
	return models.PriceTagProductState{}, s.err
}
func (s *stub) AddPriceTagProduct(context.Context, string, uuid.UUID, time.Time) (models.PriceTagProductState, error) {
	s.calls++
	return models.PriceTagProductState{}, s.err
}

func TestInvalidInputDoesNotReachStorage(t *testing.T) {
	st := &stub{}
	s := New(st)
	ctx := context.Background()
	now := time.Now()
	for _, err := range []error{
		s.Set(ctx, 1, 1, models.PriceTagItemInput{TagType: "big", Quantity: 1}),
		s.Set(ctx, 1, 1, models.PriceTagItemInput{TagType: "promo", Quantity: 0}),
		s.Set(ctx, 1, 1, models.PriceTagItemInput{TagType: "regular", Quantity: 100}),
		s.Set(ctx, 1, 0, models.PriceTagItemInput{TagType: "regular", Quantity: 1, Unit: "pc"}),
		s.Set(ctx, 1, 1, models.PriceTagItemInput{TagType: "regular", Quantity: 1, Unit: "g"}),
		s.Add(ctx, 1, models.PriceTagAddInput{}, now),
		func() error { _, e := s.Delete(ctx, 1, 2, 0); return e }(),
		func() error { _, e := s.Printed(ctx, 1, models.PriceTagPrintedInput{}, 3, now); return e }(),
		func() error {
			_, e := s.Printed(ctx, 1, models.PriceTagPrintedInput{IDs: []int64{2, 2}}, 3, now)
			return e
		}(),
		func() error {
			_, e := s.Printed(ctx, 1, models.PriceTagPrintedInput{IDs: []int64{2}}, 0, now)
			return e
		}(),
		func() error { _, e := s.Products(ctx, 1, " ч "); return e }(),
		func() error { _, e := s.Product(ctx, "store", uuid.NewString(), now); return e }(),
		func() error { _, e := s.Product(ctx, uuid.NewString(), uuid.Nil.String(), now); return e }(),
		func() error {
			_, e := s.AddProduct(ctx, models.PriceTagProductInput{StoreID: uuid.New()}, now)
			return e
		}(),
	} {
		if !errors.Is(err, services.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if st.calls != 0 {
		t.Fatal("invalid input reached storage")
	}
	if err := s.Set(ctx, 1, 1, models.PriceTagItemInput{TagType: "promo", Quantity: 99, Unit: "kg"}); err != nil || st.calls != 1 {
		t.Fatal(err)
	}
	if err := s.Set(ctx, 1, 1, models.PriceTagItemInput{TagType: "card", Quantity: 1, Unit: "pc"}); err != nil || st.calls != 2 {
		t.Fatal(err)
	}
	if _, err := s.AddProduct(ctx, models.PriceTagProductInput{StoreID: uuid.New(), ProductID: uuid.New()}, now); err != nil || st.calls != 3 {
		t.Fatal(err)
	}
	st.err = storage.ErrInvalid
	if _, err := s.Printed(ctx, 1, models.PriceTagPrintedInput{IDs: []int64{2, 3}}, 3, now); !errors.Is(err, services.ErrInvalid) {
		t.Fatal("closed task:", err)
	}
}
