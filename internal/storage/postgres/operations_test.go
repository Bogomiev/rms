package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"rms/internal/domain/models"
	"rms/internal/storage"
	"testing"
	"time"
)

func TestOperationsAndBootstrap(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("operations_test_%d", time.Now().UnixNano())
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	if _, err = db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	s := &Storage{db: db}
	if err = s.Bootstrap(ctx, "admin@example.com", "Admin", "hash1"); err != nil {
		t.Fatal(err)
	}
	if err = s.Bootstrap(ctx, "admin@example.com", "Admin", "hash2"); err != nil {
		t.Fatal(err)
	}
	admin, err := s.User(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if admin.Password != "hash1" || !admin.IsAdmin {
		t.Fatal("bootstrap overwrote existing credentials")
	}
	for i := 0; i < 3; i++ {
		if _, err = s.AddUser(ctx, fmt.Sprintf("user%d@example.com", i), "User", "hash", false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.AddUser(ctx, "user0@example.com", "User", "hash", false); !errors.Is(err, storage.ErrUserExists) {
		t.Fatal("duplicate not classified", err)
	}
	page, err := s.Users(ctx, models.UserPage{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].UserToken != "user0@example.com" || page[1].UserToken != "user1@example.com" {
		t.Fatalf("invalid page: %+v", page)
	}
	if err = s.Bootstrap(ctx, "user0@example.com", "User", "hash"); err == nil {
		t.Fatal("non-admin promoted")
	}

	_, err = s.CreateSession(ctx, &models.Session{ID: "session", UserID: admin.ID, RefreshToken: "token", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeSession(ctx, "session"); err != nil {
		t.Fatal(err)
	}
	session, err := s.GetSession(ctx, "session")
	if err != nil || !session.IsRevoked {
		t.Fatal("session not revoked", err)
	}
	if err = s.RevokeSession(ctx, "missing"); !errors.Is(err, storage.ErrSessionNotFound) {
		t.Fatal("missing session not reported", err)
	}

	product := &models.Product{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Code: "P1", Name: "Product", MarkingType: "none", Barcodes: []models.BarcodeInfo{{Barcode: "123", Unit: "kg", Ratio: 1.5, IsBase: true}}, IsWeight: true}
	if product, err = s.AddProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if product.CreatedAt.IsZero() || product.UpdatedAt.IsZero() {
		t.Fatal("product timestamps not set")
	}
	createdProduct := product.CreatedAt
	oldUpdated := product.UpdatedAt
	product.CreatedAt = time.Time{}
	product.UpdatedAt = time.Time{}
	product.Marked = true
	product.ParentID = uuid.New()
	product.Name = "Updated"
	product.IsThermalMode = true
	if product, err = s.UpdateProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if !product.CreatedAt.Equal(createdProduct) || !product.UpdatedAt.After(oldUpdated) {
		t.Fatal("product timestamps not preserved/updated")
	}
	got, err := s.GetProduct(ctx, product.ID)
	if err != nil || got.Name != "Updated" || !got.Marked || got.ParentID != product.ParentID || !got.IsThermalMode || len(got.Barcodes) != 1 || got.Barcodes[0].Ratio != 1.5 {
		t.Fatalf("product=%+v, err=%v", got, err)
	}
	products, err := s.Products(ctx)
	if err != nil || len(products) != 1 {
		t.Fatal(products, err)
	}
	if err = s.DeleteProduct(ctx, product.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteProduct(ctx, product.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}
	shop := &models.Store{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Code: "001", Name: "Shop", Address: "Address"}
	if shop, err = s.AddStore(ctx, shop); err != nil {
		t.Fatal(err)
	}
	if shop.CreatedAt.IsZero() || shop.UpdatedAt.IsZero() {
		t.Fatal("store timestamps not set")
	}
	createdShop := shop.CreatedAt
	shop.CreatedAt = time.Time{}
	shop.UpdatedAt = time.Time{}
	shop.Address = "New address"
	if shop, err = s.UpdateStore(ctx, shop); err != nil {
		t.Fatal(err)
	}
	if !shop.CreatedAt.Equal(createdShop) || shop.UpdatedAt.IsZero() {
		t.Fatal("store timestamps not preserved/updated")
	}
	gotShop, err := s.GetStore(ctx, shop.ID)
	if err != nil || *gotShop != *shop {
		t.Fatal(gotShop, err)
	}
	shops, err := s.Stores(ctx)
	if err != nil || len(shops) != 1 {
		t.Fatal(shops, err)
	}
	if err = s.DeleteStore(ctx, shop.ID); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.AddProduct(canceled, product); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
