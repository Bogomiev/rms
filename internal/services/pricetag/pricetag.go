// Package pricetag serves the standing «Печать ценников» store task: products whose price tag changed
// (a new regular price, a stopper promotion that starts or ended, a «Не соответствует» mark in the
// «Стопперы» task) after the last print, and products added by hand or from the product card.
// Tags are printed in the browser; RMS keeps what was printed and when.
package pricetag

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"rms/internal/domain/models"
	"rms/internal/services"
)

type Storage interface {
	PriceTagTask(context.Context, int64, time.Time) (models.PriceTagTask, error)
	AddPriceTag(context.Context, int64, uuid.UUID, time.Time) error
	SetPriceTag(context.Context, int64, int64, models.PriceTagItemInput) error
	DeletePriceTag(context.Context, int64, int64, int64) (string, error)
	SetPriceTagsPrinted(context.Context, int64, []int64, int64, time.Time) (string, error)
	PriceTagProducts(context.Context, int64, string, int) ([]models.PriceTagProduct, error)
	PriceTagProduct(context.Context, string, uuid.UUID, time.Time) (models.PriceTagProductState, error)
	AddPriceTagProduct(context.Context, string, uuid.UUID, time.Time) (models.PriceTagProductState, error)
}

type Service struct{ storage Storage }

func New(s Storage) *Service { return &Service{storage: s} }

// tagTypes: a regular tag, a discount with the regular price crossed out, a loyalty card price.
var tagTypes = map[string]bool{"regular": true, "promo": true, "card": true}

// units: шт, 100 г, 1 кг; storage checks that the unit fits the product (piece or weighed).
var units = map[string]bool{models.PriceTagUnitPiece: true, models.PriceTagUnit100g: true, models.PriceTagUnitKg: true}

// maxPrinted limits one print request; a task has far fewer positions.
const maxPrinted = 1000

// Task returns the task screen; now is the moment in the scheduler time zone.
func (s *Service) Task(ctx context.Context, id int64, now time.Time) (models.PriceTagTask, error) {
	if id <= 0 {
		return models.PriceTagTask{}, services.ErrInvalid
	}
	v, e := s.storage.PriceTagTask(ctx, id, now)
	return v, services.PersonnelError(e)
}

func (s *Service) Add(ctx context.Context, id int64, in models.PriceTagAddInput, now time.Time) error {
	if id <= 0 || in.ProductID == uuid.Nil {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.AddPriceTag(ctx, id, in.ProductID, now))
}

func (s *Service) Set(ctx context.Context, id, item int64, in models.PriceTagItemInput) error {
	if id <= 0 || item <= 0 || !tagTypes[in.TagType] || !units[in.Unit] || in.Quantity < 1 || in.Quantity > 99 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.SetPriceTag(ctx, id, item, in))
}

// Delete removes a position added by hand that was never printed and returns the task status.
func (s *Service) Delete(ctx context.Context, id, item, user int64) (string, error) {
	if id <= 0 || item <= 0 || user <= 0 {
		return "", services.ErrInvalid
	}
	status, e := s.storage.DeletePriceTag(ctx, id, item, user)
	return status, services.PersonnelError(e)
}

// Printed marks positions as printed and returns the task status.
func (s *Service) Printed(ctx context.Context, id int64, in models.PriceTagPrintedInput, user int64, now time.Time) (string, error) {
	if id <= 0 || user <= 0 || len(in.IDs) == 0 || len(in.IDs) > maxPrinted {
		return "", services.ErrInvalid
	}
	seen := map[int64]bool{}
	for _, v := range in.IDs {
		if v <= 0 || seen[v] {
			return "", services.ErrInvalid
		}
		seen[v] = true
	}
	status, e := s.storage.SetPriceTagsPrinted(ctx, id, in.IDs, user, now)
	return status, services.PersonnelError(e)
}

// Products finds products to add by name, code or barcode; the query needs at least 2 characters.
func (s *Service) Products(ctx context.Context, id int64, query string) ([]models.PriceTagProduct, error) {
	query = strings.TrimSpace(query)
	if id <= 0 || utf8.RuneCountInString(query) < 2 || len(query) > 255 {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.PriceTagProducts(ctx, id, query, 20)
	return v, services.PersonnelError(e)
}

func productInput(store, product string) (uuid.UUID, uuid.UUID, bool) {
	s, e1 := uuid.Parse(store)
	p, e2 := uuid.Parse(product)
	return s, p, e1 == nil && e2 == nil && s != uuid.Nil && p != uuid.Nil
}

// Product returns the price tag state of a product in a store for the product card.
func (s *Service) Product(ctx context.Context, store, product string, now time.Time) (models.PriceTagProductState, error) {
	st, p, ok := productInput(store, product)
	if !ok {
		return models.PriceTagProductState{}, services.ErrInvalid
	}
	v, e := s.storage.PriceTagProduct(ctx, st.String(), p, now)
	return v, services.PersonnelError(e)
}

// AddProduct puts a product into the standing task of the store, creating the task when there is none.
func (s *Service) AddProduct(ctx context.Context, in models.PriceTagProductInput, now time.Time) (models.PriceTagProductState, error) {
	if in.StoreID == uuid.Nil || in.ProductID == uuid.Nil {
		return models.PriceTagProductState{}, services.ErrInvalid
	}
	v, e := s.storage.AddPriceTagProduct(ctx, in.StoreID.String(), in.ProductID, now)
	return v, services.PersonnelError(e)
}
