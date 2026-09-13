package store

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"rms/internal/domain/models"
	"unicode/utf8"
)

type StoreStorage interface {
	Stores(context.Context) ([]models.Store, error)
	GetStore(context.Context, uuid.UUID) (*models.Store, error)
	AddStore(context.Context, *models.Store) (*models.Store, error)
	UpdateStore(context.Context, *models.Store) (*models.Store, error)
	DeleteStore(context.Context, uuid.UUID) error
}
type StoreService struct{ storage StoreStorage }

func New(_ *slog.Logger, s StoreStorage) *StoreService { return &StoreService{storage: s} }
func (s *StoreService) GetStore(ctx context.Context, id uuid.UUID) (*models.Store, error) {
	return s.storage.GetStore(ctx, id)
}
func (s *StoreService) DeleteStore(ctx context.Context, id uuid.UUID) error {
	return s.storage.DeleteStore(ctx, id)
}
func (s *StoreService) AddStore(ctx context.Context, v *models.Store) (*models.Store, error) {
	if v == nil || v.ID == uuid.Nil {
		return nil, fmt.Errorf("id is required")
	}
	if utf8.RuneCountInString(v.Address) > 250 {
		return nil, fmt.Errorf("address must be at most 250 characters")
	}
	return s.storage.AddStore(ctx, v)
}
func (s *StoreService) UpdateStore(ctx context.Context, v *models.Store) (*models.Store, error) {
	if v == nil || v.ID == uuid.Nil {
		return nil, fmt.Errorf("id is required")
	}
	if utf8.RuneCountInString(v.Address) > 250 {
		return nil, fmt.Errorf("address must be at most 250 characters")
	}
	return s.storage.UpdateStore(ctx, v)
}
func (s *StoreService) Stores(ctx context.Context) (models.StoresResponse, error) {
	items, err := s.storage.Stores(ctx)
	if err != nil {
		return models.StoresResponse{}, err
	}
	if items == nil {
		items = []models.Store{}
	}
	pages := 0
	if len(items) > 0 {
		pages = 1
	}
	return models.StoresResponse{Page: 1, PerPage: len(items), TotalPages: pages, TotalItems: len(items), Items: items}, nil
}
