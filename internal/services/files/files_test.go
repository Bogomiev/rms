package files

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"testing"
)

type memory struct {
	rows []models.File
	fail bool
}

func (m *memory) Transaction(ctx context.Context, fn func(Store) error) error {
	before := append([]models.File(nil), m.rows...)
	e := fn(m)
	if e != nil {
		m.rows = before
	}
	return e
}
func (m *memory) List(context.Context) ([]models.File, error) { return m.rows, nil }
func (m *memory) Save(_ context.Context, f models.File) error {
	if m.fail {
		return errors.New("database failed")
	}
	for i, old := range m.rows {
		if old.ID == f.ID {
			m.rows[i] = f
			return nil
		}
	}
	m.rows = append(m.rows, f)
	return nil
}
func (m *memory) Delete(_ context.Context, id string) error {
	if m.fail {
		return errors.New("database failed")
	}
	for i, f := range m.rows {
		if f.ID == id {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			break
		}
	}
	return nil
}

type objects struct {
	data    map[string][]byte
	fail    bool
	puts    int
	deletes int
}

func (o *objects) Ensure(_ context.Context, k string, b []byte) error {
	if o.fail {
		return errors.New("S3 failed")
	}
	if _, ok := o.data[k]; !ok {
		o.data[k] = b
		o.puts++
	}
	return nil
}
func (o *objects) Exists(_ context.Context, k string) error {
	if _, ok := o.data[k]; !ok {
		return ErrConflict
	}
	return nil
}
func (o *objects) Delete(_ context.Context, k string) error {
	if o.fail {
		return errors.New("S3 failed")
	}
	delete(o.data, k)
	o.deletes++
	return nil
}
func fixture() (*Service, *memory, *objects) {
	db := &memory{}
	o := &objects{data: map[string][]byte{}}
	return New(db, o), db, o
}
func TestRetryAfterDatabaseFailure(t *testing.T) {
	s, db, o := fixture()
	ctx := context.Background()
	in := Input{ID: uuid.NewString(), Name: "report.txt", IsFile: true, Content: []byte("first")}
	db.fail = true
	if _, e := s.Write(ctx, in, false); e == nil {
		t.Fatal("expected database failure")
	}
	if len(db.rows) != 0 || o.puts != 1 {
		t.Fatal("unexpected side effects")
	}
	db.fail = false
	first, e := s.Write(ctx, in, false)
	if e != nil {
		t.Fatal(e)
	}
	if o.puts != 1 {
		t.Fatal("retry reuploaded object")
	}
	in.Content = []byte("replacement")
	db.fail = true
	if _, e = s.Write(ctx, in, true); e == nil {
		t.Fatal("expected failure")
	}
	if db.rows[0].S3Key != first.S3Key || string(o.data[first.S3Key]) != "first" {
		t.Fatal("failed update changed visible content")
	}
	db.fail = false
	updated, e := s.Write(ctx, in, true)
	if e != nil {
		t.Fatal(e)
	}
	if updated.S3Key == first.S3Key || o.puts != 2 {
		t.Fatal("update retry failed")
	}
}
func TestStorageFailureKeepsMetadata(t *testing.T) {
	s, db, o := fixture()
	o.fail = true
	_, e := s.Write(context.Background(), Input{ID: uuid.NewString(), Name: "f", IsFile: true, Content: []byte("x")}, false)
	if e == nil || len(db.rows) != 0 {
		t.Fatal("metadata persisted without S3")
	}
}
func TestTreeValidationAndDeletion(t *testing.T) {
	s, db, o := fixture()
	ctx := context.Background()
	parent := Input{ID: uuid.NewString(), Name: "docs"}
	if _, e := s.Write(ctx, parent, false); e != nil {
		t.Fatal(e)
	}
	child := Input{ID: uuid.NewString(), Name: "a.txt", ParentID: &parent.ID, IsFile: true, Content: []byte("a")}
	f, e := s.Write(ctx, child, false)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := s.Get(ctx, "", "/docs/a.txt"); e != nil || got.ID != f.ID {
		t.Fatal(got, e)
	}
	bad := child
	bad.ID = uuid.NewString()
	bad.ParentID = &child.ID
	if _, e = s.Write(ctx, bad, false); !errors.Is(e, ErrInvalid) {
		t.Fatal("file accepted as parent", e)
	}
	changed := child
	changed.IsFile = false
	if _, e = s.Write(ctx, changed, true); !errors.Is(e, ErrConflict) {
		t.Fatal("changed is_file", e)
	}
	nested := Input{ID: uuid.NewString(), Name: "nested", ParentID: &parent.ID}
	if _, e = s.Write(ctx, nested, false); e != nil {
		t.Fatal(e)
	}
	parent.ParentID = &nested.ID
	if _, e = s.Write(ctx, parent, true); !errors.Is(e, ErrInvalid) {
		t.Fatal("cycle accepted", e)
	}
	if e = s.Delete(ctx, parent.ID, false); !errors.Is(e, ErrConflict) {
		t.Fatal("deleted nonempty directory", e)
	}
	if e = s.Delete(ctx, child.ID); e != nil {
		t.Fatal(e)
	}
	if db.rows[1].DeletedAt == nil || o.deletes != 0 {
		t.Fatal("soft deletion touched S3")
	}
	visible, e := s.List(ctx, &parent.ID, false)
	if e != nil || len(visible) != 1 {
		t.Fatal(visible, e)
	}
	delete(o.data, f.S3Key)
	if e = s.Delete(ctx, child.ID, false); e != nil {
		t.Fatal("missing object blocked deletion", e)
	}
	if _, e = s.Get(ctx, child.ID, ""); !errors.Is(e, ErrNotFound) {
		t.Fatal("metadata remains", e)
	}
}

func TestHardDeleteRetriesAfterDatabaseFailure(t *testing.T) {
	s, db, o := fixture()
	ctx := context.Background()
	in := Input{ID: uuid.NewString(), Name: "f", IsFile: true, Content: []byte("data")}
	f, e := s.Write(ctx, in, false)
	if e != nil {
		t.Fatal(e)
	}
	o.fail = true
	if e = s.Delete(ctx, in.ID, false); e == nil {
		t.Fatal("expected S3 failure")
	}
	if len(db.rows) != 1 {
		t.Fatal("metadata removed before S3")
	}
	o.fail = false
	db.fail = true
	if e = s.Delete(ctx, in.ID, false); e == nil {
		t.Fatal("expected database failure")
	}
	if len(db.rows) != 1 {
		t.Fatal("metadata must remain after failed delete")
	}
	if _, ok := o.data[f.S3Key]; ok {
		t.Fatal("object must be deleted first")
	}
	db.fail = false
	if e = s.Delete(ctx, in.ID, false); e != nil {
		t.Fatal(e)
	}
	if len(db.rows) != 0 {
		t.Fatal("retry did not remove metadata")
	}
}
