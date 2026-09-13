package product

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"rms/internal/domain/models"
	"unicode/utf8"
)

type ProductStorage interface {
	Products(context.Context) ([]models.Product, error)
	GetProduct(context.Context, uuid.UUID) (*models.Product, error)
	AddProduct(context.Context, *models.Product) (*models.Product, error)
	UpdateProduct(context.Context, *models.Product) (*models.Product, error)
	DeleteProduct(context.Context, uuid.UUID) error
}
type ProductService struct{ storage ProductStorage }

func New(_ *slog.Logger, s ProductStorage) *ProductService { return &ProductService{storage: s} }
func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return s.storage.GetProduct(ctx, id)
}
func (s *ProductService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	return s.storage.DeleteProduct(ctx, id)
}
func (s *ProductService) AddProduct(ctx context.Context, v *models.Product) (*models.Product, error) {
	if v == nil || v.ID == uuid.Nil {
		return nil, fmt.Errorf("id is required")
	}
	if utf8.RuneCountInString(v.Code) > 11 || utf8.RuneCountInString(v.Name) > 100 {
		return nil, fmt.Errorf("code must be at most 11 characters and name at most 100")
	}
	return s.storage.AddProduct(ctx, v)
}
func (s *ProductService) UpdateProduct(ctx context.Context, v *models.Product) (*models.Product, error) {
	if v == nil || v.ID == uuid.Nil {
		return nil, fmt.Errorf("id is required")
	}
	if utf8.RuneCountInString(v.Code) > 11 || utf8.RuneCountInString(v.Name) > 100 {
		return nil, fmt.Errorf("code must be at most 11 characters and name at most 100")
	}
	return s.storage.UpdateProduct(ctx, v)
}
func (s *ProductService) Products(ctx context.Context) ([]models.Product, error) {
	return s.storage.Products(ctx)
}
