package files

import (
	"context"
	"io"
	"rms/internal/domain/models"
	"strings"
	"testing"
	"time"
)

type readableObjects struct {
	*objects
	reads int
}

func (o *readableObjects) Open(context.Context, string) (io.ReadCloser, error) {
	o.reads++
	return io.NopCloser(strings.NewReader("document")), nil
}
func TestContentActiveOnly(t *testing.T) {
	id := "a7954397-4aee-48e1-8b90-d5bbca8bf5f3"
	for _, mode := range []string{"active", "deleted", "directory", "missing", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			f := models.File{ID: id, Name: "doc.pdf", IsFile: true, S3Key: "files/" + id + "/hash"}
			if mode == "directory" {
				f.IsFile = false
			}
			if mode == "deleted" {
				now := time.Now()
				f.DeletedAt = &now
			}
			db := &memory{rows: []models.File{f}}
			if mode == "missing" {
				db.rows = nil
			}
			o := &readableObjects{objects: &objects{}}
			s := New(db, o)
			if mode == "unavailable" {
				s = New(db, nil)
			}
			_, body, err := s.Content(context.Background(), id)
			if mode == "active" {
				if err != nil {
					t.Fatal(err)
				}
				defer body.Close()
				data, _ := io.ReadAll(body)
				if string(data) != "document" {
					t.Fatal(string(data))
				}
			} else {
				if err == nil || o.reads != 0 {
					t.Fatal("inactive content must not be read", err)
				}
			}
		})
	}
}
