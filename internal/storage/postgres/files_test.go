package postgres

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"rms/internal/services/files"
	"testing"
	"time"
)

type testFileObjects struct{}

func (testFileObjects) Ensure(context.Context, string, []byte) error { return nil }
func (testFileObjects) Exists(context.Context, string) error         { return nil }
func (testFileObjects) Delete(context.Context, string) error         { return nil }
func TestFilesPostgres(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	s := files.New(db, testFileObjects{})
	dir := files.Input{ID: uuid.NewString(), Name: "docs"}
	if _, e := s.Write(ctx, dir, false); e != nil {
		t.Fatal(e)
	}
	child := files.Input{ID: uuid.NewString(), ParentID: &dir.ID, Name: "a.txt", IsFile: true, Content: []byte("hello"), Icon: []byte{1, 2, 3}}
	if _, e := s.Write(ctx, child, false); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Write(ctx, child, false); e != nil {
		t.Fatal("retry", e)
	}
	if _, e := db.db.ExecContext(ctx, "UPDATE files SET is_file=false WHERE id=$1", child.ID); e == nil {
		t.Fatal("database allowed changing is_file")
	}
	if _, e := db.db.ExecContext(ctx, "UPDATE files SET parent_id=$1 WHERE id=$2", child.ID, dir.ID); e == nil {
		t.Fatal("database accepted file parent")
	}
	nested := files.Input{ID: uuid.NewString(), Name: "nested", ParentID: &dir.ID}
	if _, e := s.Write(ctx, nested, false); e != nil {
		t.Fatal(e)
	}
	if _, e := db.db.ExecContext(ctx, "UPDATE files SET parent_id=$1 WHERE id=$2", nested.ID, dir.ID); e == nil {
		t.Fatal("database accepted cycle")
	}
	if e := s.Delete(ctx, dir.ID, false); !errors.Is(e, files.ErrConflict) {
		t.Fatal("nonempty delete", e)
	}
	found, e := s.Get(ctx, "", "/docs/a.txt")
	if e != nil || string(found.Icon) != string(child.Icon) {
		t.Fatal(found, e)
	}
	if e = s.Delete(ctx, child.ID); e != nil {
		t.Fatal(e)
	}
	found, e = s.Get(ctx, child.ID, "")
	if e != nil || found.DeletedAt == nil {
		t.Fatal("soft delete", found, e)
	}
	if e = s.Delete(ctx, child.ID, false); e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, nested.ID, false); e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, dir.ID, false); e != nil {
		t.Fatal(e)
	}
}

func TestFileReadsDoNotWaitForWriter(t *testing.T) {
	db := authTestStorage(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := files.New(db, testFileObjects{})
	dir := files.Input{ID: uuid.NewString(), Name: "directory"}
	if _, err := service.Write(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(714206914)"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(ctx, dir.ID, ""); err != nil {
		t.Fatal("get blocked by writer", err)
	}
	if _, err = service.Get(ctx, "", "/directory"); err != nil {
		t.Fatal("path lookup blocked by writer", err)
	}
	if rows, err := service.List(ctx, nil, false); err != nil || len(rows) != 1 {
		t.Fatal("list blocked by writer", rows, err)
	}
}

func TestFileQueriesPreserveFilteringAndOrder(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	service := files.New(db, testFileObjects{})
	dir := files.Input{ID: uuid.NewString(), Name: "docs"}
	if _, err := service.Write(ctx, dir, false); err != nil {
		t.Fatal(err)
	}
	inputs := []files.Input{
		{ID: uuid.NewString(), Name: "z", Priority: 1, ParentID: &dir.ID},
		{ID: uuid.NewString(), Name: "b", ParentID: &dir.ID},
		{ID: uuid.NewString(), Name: "a", ParentID: &dir.ID},
	}
	for _, in := range inputs {
		if _, err := service.Write(ctx, in, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.Delete(ctx, inputs[1].ID); err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []bool{false, true} {
		rows, err := service.List(ctx, &dir.ID, deleted)
		want := []string{"a", "z"}
		if deleted {
			want = []string{"a", "b", "z"}
		}
		if err != nil || len(rows) != len(want) {
			t.Fatalf("rows=%v err=%v", rows, err)
		}
		for i, name := range want {
			if rows[i].Name != name {
				t.Fatalf("order: %v", rows)
			}
		}
	}
	if _, err := service.Get(ctx, "", "/docs/b"); !errors.Is(err, files.ErrNotFound) {
		t.Fatal("deleted path visible", err)
	}
	if f, err := service.Get(ctx, inputs[1].ID, ""); err != nil || f.DeletedAt == nil {
		t.Fatal("deleted ID lookup", f, err)
	}
	duplicate := inputs[1]
	duplicate.ID = uuid.NewString()
	if _, err := service.Write(ctx, duplicate, false); !errors.Is(err, files.ErrConflict) {
		t.Fatal("deleted name reused", err)
	}
	if err := service.Delete(ctx, inputs[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, inputs[2].ID, false); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, dir.ID, false); !errors.Is(err, files.ErrConflict) {
		t.Fatal("deleted child did not block removal", err)
	}
}
