package role

import (
	"context"
	"errors"
	"rms/internal/services"
	"rms/internal/storage"
	"testing"
)

type writeStorage struct {
	Storage
	err   error
	calls int
}

func (s *writeStorage) AddRole(context.Context, string) (int64, error) { s.calls++; return 1, s.err }
func (s *writeStorage) SaveRole(context.Context, int64, string) error  { s.calls++; return s.err }

func TestRoleNames(t *testing.T) {
	for _, name := range []string{"admin", "orders", "products", "documents", "goods-receipts", "goods-distribution", "cash-collection", "", "order", "Admin", " orders", "orders ", "custom"} {
		t.Run(name, func(t *testing.T) {
			valid := name == "admin" || name == "orders" || name == "products" || name == "documents" || name == "goods-receipts" || name == "goods-distribution" || name == "cash-collection"
			db := &writeStorage{}
			svc := New(db)
			_, addErr := svc.AddRole(context.Background(), name)
			saveErr := svc.SaveRole(context.Background(), 1, name)
			if valid {
				if addErr != nil || saveErr != nil || db.calls != 2 {
					t.Fatalf("valid name rejected: %v, %v", addErr, saveErr)
				}
			} else if !errors.Is(addErr, services.ErrInvalid) || !errors.Is(saveErr, services.ErrInvalid) || db.calls != 0 {
				t.Fatalf("invalid name reached storage: %v, %v", addErr, saveErr)
			}
		})
	}
}
func TestDuplicateRoleName(t *testing.T) {
	svc := New(&writeStorage{err: storage.ErrConflict})
	if _, err := svc.AddRole(context.Background(), "orders"); !errors.Is(err, services.ErrConflict) {
		t.Fatal(err)
	}
	if err := svc.SaveRole(context.Background(), 1, "orders"); !errors.Is(err, services.ErrConflict) {
		t.Fatal(err)
	}
}
