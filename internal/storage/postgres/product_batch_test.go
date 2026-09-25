package postgres

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"rms/internal/domain/models"
)

func TestUpsertProducts(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	existing := models.Product{ID: uuid.New(), ParentID: uuid.New(), Marked: true, Code: "old", Name: "Old"}
	if _, err := s.AddProduct(ctx, &existing); err != nil {
		t.Fatal(err)
	}
	products := make([]models.Product, 250)
	for i := range products {
		products[i] = models.Product{ID: uuid.New(), Code: "ЦБ-00003573", Name: "Товар", MarkingType: "БезОсобенностейУчета", IsWeight: true, IsThermalMode: true, Barcodes: []models.BarcodeInfo{{Barcode: "001234", Unit: "шт", Ratio: 6}}}
	}
	products[0].ID = existing.ID
	for repeat := 0; repeat < 2; repeat++ {
		if err := s.UpsertProducts(ctx, products); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Products(ctx)
	if err != nil || len(got) != 250 {
		t.Fatalf("count=%d err=%v", len(got), err)
	}
	updated, err := s.GetProduct(ctx, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ParentID != existing.ParentID || !updated.Marked || updated.Name != products[0].Name || !reflect.DeepEqual(updated.Barcodes, products[0].Barcodes) {
		t.Fatalf("unexpected product: %+v", updated)
	}
	// A failed statement must leave every product in this batch unchanged.
	products[0].Name = "Should not persist"
	products[1].Code = "too-long-code"
	if err := s.UpsertProducts(ctx, products); err == nil {
		t.Fatal("expected constraint failure")
	}
	updated, err = s.GetProduct(ctx, existing.ID)
	if err != nil || updated.Name != "Товар" {
		t.Fatalf("batch was not atomic: %+v %v", updated, err)
	}
}

func TestBarcodeReplacementAndCascade(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	p := models.Product{ID: uuid.New(), Code: "1", Name: "Product", Barcodes: []models.BarcodeInfo{{Barcode: "001", Unit: "pc", Ratio: 1, IsBase: true}, {Barcode: "001", Unit: "box", Ratio: 6}}}
	if _, err := s.AddProduct(ctx, &p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(got.Barcodes, p.Barcodes) {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	p.Barcodes = []models.BarcodeInfo{{Barcode: "002", Unit: "kg", Ratio: 0.5}}
	if err := s.UpsertProducts(ctx, []models.Product{p}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProduct(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(got.Barcodes, p.Barcodes) {
		t.Fatalf("replacement: %+v %v", got, err)
	}
	p.Barcodes = nil
	got, err = s.UpdateProduct(ctx, &p)
	if err != nil || got.Barcodes == nil || len(got.Barcodes) != 0 {
		t.Fatalf("clear: %+v %v", got, err)
	}
	p.Barcodes = []models.BarcodeInfo{{Barcode: "003"}}
	if err := s.UpsertProducts(ctx, []models.Product{p}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProduct(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM product_barcodes WHERE product_id=$1", p.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade: %d %v", count, err)
	}
}

func TestBarcodeMigration(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	// Recreate the old column in the isolated test schema to exercise the upgrade.
	_, err := s.db.Exec(`DROP TABLE product_barcodes;
 ALTER TABLE products ADD COLUMN barcodes JSONB NOT NULL DEFAULT '[]';
 DELETE FROM schema_migrations WHERE version='002_product_barcodes.sql';
 INSERT INTO products(id,code,name,parent_id,marking_type,barcodes) VALUES
 ('11111111-1111-1111-1111-111111111111','1','Old','00000000-0000-0000-0000-000000000000','none',
 '[{"barcode":"001","unit":"pc","ratio":1.5,"isBase":true},{"barcode":"001","unit":"box","ratio":6,"isBase":false}]');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrate(ctx, s.db); err != nil {
		t.Fatal(err)
	}
	if err = migrate(ctx, s.db); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(ctx, uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	want := []models.BarcodeInfo{{Barcode: "001", Unit: "pc", Ratio: 1.5, IsBase: true}, {Barcode: "001", Unit: "box", Ratio: 6}}
	if err != nil || !reflect.DeepEqual(got.Barcodes, want) {
		t.Fatalf("migration: %+v %v", got, err)
	}
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='products' AND column_name='barcodes'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old column: %d %v", count, err)
	}
}

func TestBarcodeFailureRollsBackProductBatch(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	p := models.Product{ID: uuid.New(), Code: "1", Name: "Original", Barcodes: []models.BarcodeInfo{{Barcode: "001"}}}
	if _, err := s.AddProduct(ctx, &p); err != nil {
		t.Fatal(err)
	}
	// Force a child-table failure after parent upserts and barcode deletion.
	if _, err := s.db.Exec(`ALTER TABLE product_barcodes ADD CONSTRAINT test_barcode CHECK (barcode <> 'reject')`); err != nil {
		t.Fatal(err)
	}
	changed := p
	changed.Name = "Changed"
	changed.Barcodes = []models.BarcodeInfo{{Barcode: "reject"}}
	added := models.Product{ID: uuid.New(), Code: "2", Name: "New"}
	if err := s.UpsertProducts(ctx, []models.Product{changed, added}); err == nil {
		t.Fatal("expected barcode failure")
	}
	got, err := s.GetProduct(ctx, p.ID)
	if err != nil || got.Name != p.Name || !reflect.DeepEqual(got.Barcodes, p.Barcodes) {
		t.Fatalf("rollback: %+v %v", got, err)
	}
	all, err := s.Products(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("partial batch persisted: %+v %v", all, err)
	}
}
