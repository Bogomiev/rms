package postgres

import (
	"context"
	"testing"
)

// A database that applied the name search migration as 012_product_name_search.sql
// reruns it as 015 without errors and keeps a single record of it.
func TestRenumberedNameSearchMigration(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version='015_product_name_search.sql';
 INSERT INTO schema_migrations(version) VALUES('012_product_name_search.sql')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, s.db); err != nil {
		t.Fatal(err)
	}
	var old, renamed int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE version='012_product_name_search.sql'), count(*) FILTER (WHERE version='015_product_name_search.sql') FROM schema_migrations`).Scan(&old, &renamed); err != nil {
		t.Fatal(err)
	}
	if old != 0 || renamed != 1 {
		t.Fatal(old, renamed)
	}
	var index string
	if err := s.db.QueryRowContext(ctx, "SELECT indexname FROM pg_indexes WHERE indexname='idx_product_name_trgm' AND schemaname=current_schema()").Scan(&index); err != nil {
		t.Fatal("name search index:", err)
	}
}
