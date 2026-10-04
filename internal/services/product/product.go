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
	UpsertProducts(context.Context, []models.Product) error
	Products(context.Context) ([]models.Product, error)
	GetProduct(context.Context, uuid.UUID) (*models.Product, error)
	AddProduct(context.Context, *models.Product) (*models.Product, error)
	UpdateProduct(context.Context, *models.Product) (*models.Product, error)
	DeleteProduct(context.Context, uuid.UUID) error
}
type MarketplaceService interface {
	MarketplaceData(context.Context) (models.MarketplaceResponse, error)
}
type ProductInfoSource interface {
	GetProductInfo(context.Context, uuid.UUID, []uuid.UUID) (*models.OneCProductInfo, error)
}

type InfoStorage interface {
	SearchProducts(context.Context, models.ProductFilter, int) ([]models.Product, error)
	ProductValues(context.Context, uuid.UUID, []uuid.UUID, []uuid.UUID, string) (map[uuid.UUID]models.ProductValues, error)
}
type ProductService struct {
	logger       *slog.Logger
	storage      ProductStorage
	info         InfoStorage
	marketplaces MarketplaceService
	onecInfo     ProductInfoSource
	timezone     string
}

func (s *ProductService) ConfigureProductInfo(source ProductInfoSource) { s.onecInfo = source }

func (s *ProductService) ConfigureInfo(db InfoStorage, m MarketplaceService, timezone string) {
	s.info = db
	s.marketplaces = m
	s.timezone = timezone
}
func (s *ProductService) SearchProducts(ctx context.Context, f models.ProductFilter) ([]models.Product, error) {
	return s.info.SearchProducts(ctx, f, 0)
}

func New(logger *slog.Logger, s ProductStorage) *ProductService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProductService{logger: logger, storage: s}
}
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

// UpsertProducts saves a batch received from 1C.
func (s *ProductService) UpsertProducts(ctx context.Context, products []models.Product) error {
	for i, v := range products {
		if v.ID == uuid.Nil || utf8.RuneCountInString(v.Code) > 11 || utf8.RuneCountInString(v.Name) > 100 {
			return fmt.Errorf("invalid product at index %d", i)
		}
	}
	return s.storage.UpsertProducts(ctx, products)
}
