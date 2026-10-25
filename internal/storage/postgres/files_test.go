package postgres

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"rms/internal/services/files"
	"testing"
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
