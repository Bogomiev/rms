// Package files coordinates PostgreSQL metadata with immutable S3 objects.
package files

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"rms/internal/domain/models"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid file request")
	ErrNotFound    = errors.New("file not found")
	ErrConflict    = errors.New("file conflict or nonempty directory")
	ErrUnavailable = errors.New("S3 is not configured")
)

type Store interface {
	List(context.Context) ([]models.File, error)
	Save(context.Context, models.File) error
	Delete(context.Context, string) error
}
type Repository interface {
	Transaction(context.Context, func(Store) error) error
}
type Objects interface {
	Ensure(context.Context, string, []byte) error
	Delete(context.Context, string) error
	Exists(context.Context, string) error
}
type Service struct {
	db      Repository
	objects Objects
}

func New(db Repository, objects Objects) *Service { return &Service{db, objects} }

// Content and Icon are base64 encoded by JSON. A nil Content preserves existing bytes.
type Input struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Priority int     `json:"priority"`
	ParentID *string `json:"parent_id"`
	IsFile   bool    `json:"is_file"`
	Icon     []byte  `json:"icon"`
	Content  []byte  `json:"content"`
}

func (s *Service) List(ctx context.Context, parent *string, deleted bool) (out []models.File, err error) {
	if parent != nil {
		parsed, e := uuid.Parse(*parent)
		if e != nil {
			return nil, ErrInvalid
		}
		v := parsed.String()
		parent = &v
	}
	out = []models.File{}
	err = s.db.Transaction(ctx, func(db Store) error {
		all, e := db.List(ctx)
		if e != nil {
			return e
		}
		for _, f := range all {
			if sameParent(f.ParentID, parent) && (deleted || f.DeletedAt == nil) {
				out = append(out, f)
			}
		}
		return nil
	})
	return
}
func sameParent(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func find(all []models.File, id string) (models.File, error) {
	for _, f := range all {
		if f.ID == id {
			return f, nil
		}
	}
	return models.File{}, ErrNotFound
}
func (s *Service) Get(ctx context.Context, id, path string) (out models.File, err error) {
	err = s.db.Transaction(ctx, func(db Store) error {
		all, e := db.List(ctx)
		if e != nil {
			return e
		}
		if id != "" {
			parsed, parseErr := uuid.Parse(id)
			if parseErr != nil {
				return ErrInvalid
			}
			id = parsed.String()
			out, e = find(all, id)
			return e
		}
		var parent *string
		for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
			found := false
			for _, f := range all {
				if f.Name == part && sameParent(f.ParentID, parent) && f.DeletedAt == nil {
					out = f
					v := f.ID
					parent = &v
					found = true
					break
				}
			}
			if !found {
				return ErrNotFound
			}
		}
		return nil
	})
	return
}
func (s *Service) Write(ctx context.Context, in Input, update bool) (out models.File, err error) {
	if parsed, e := uuid.Parse(in.ID); e != nil || parsed == uuid.Nil {
		return out, ErrInvalid
	}
	parsedID, _ := uuid.Parse(in.ID)
	in.ID = parsedID.String()
	if in.ParentID != nil {
		if parsed, e := uuid.Parse(*in.ParentID); e != nil {
			return out, ErrInvalid
		} else {
			value := parsed.String()
			in.ParentID = &value
		}
	}
	if strings.TrimSpace(in.Name) == "" || strings.ContainsAny(in.Name, "/\\\x00") || in.Name == "." || in.Name == ".." {
		return out, ErrInvalid
	}
	err = s.db.Transaction(ctx, func(db Store) error {
		all, e := db.List(ctx)
		if e != nil {
			return e
		}
		old, exists := find(all, in.ID)
		hasOld := exists == nil
		if update && !hasOld {
			return ErrNotFound
		}
		if hasOld && (old.IsFile != in.IsFile || old.DeletedAt != nil) {
			return ErrConflict
		}
		if !update && hasOld {
			if old.Name != in.Name || !sameParent(old.ParentID, in.ParentID) || old.Priority != in.Priority || string(old.Icon) != string(in.Icon) {
				return ErrConflict
			}
		}
		if in.ParentID != nil {
			p, e := find(all, *in.ParentID)
			if e != nil || p.IsFile || p.DeletedAt != nil {
				return ErrInvalid
			}
			for {
				if p.ID == in.ID {
					return ErrInvalid
				}
				if p.ParentID == nil {
					break
				}
				p, e = find(all, *p.ParentID)
				if e != nil {
					return ErrInvalid
				}
			}
		}
		for _, f := range all {
			if f.ID != in.ID && f.Name == in.Name && sameParent(f.ParentID, in.ParentID) {
				return ErrConflict
			}
		}
		if !in.IsFile && in.Content != nil {
			return ErrInvalid
		}
		key := old.S3Key
		if !hasOld || in.Content != nil {
			if in.IsFile && in.Content == nil {
				return ErrInvalid
			}
			key = fmt.Sprintf("files/%s/%x", in.ID, sha256.Sum256(in.Content))
			if !in.IsFile {
				key = "files/" + in.ID + "/"
			}
		}
		if !update && hasOld && key != old.S3Key {
			return ErrConflict
		}
		if !hasOld || in.Content != nil || !in.IsFile {
			if s.objects == nil {
				return ErrUnavailable
			}
			if e = s.objects.Ensure(ctx, key, in.Content); e != nil {
				return e
			}
		}
		if hasOld && in.Content == nil && in.IsFile {
			if s.objects == nil {
				return ErrUnavailable
			}
			if e = s.objects.Exists(ctx, key); e != nil {
				return e
			}
		}
		now := time.Now().UTC()
		out = models.File{ID: in.ID, Name: in.Name, Priority: in.Priority, ParentID: in.ParentID, S3Key: key, IsFile: in.IsFile, Icon: in.Icon, CreatedAt: now, UpdatedAt: now}
		if hasOld {
			out.CreatedAt = old.CreatedAt
		}
		return db.Save(ctx, out)
	})
	return
}

// Delete defaults to a soft delete when marked is omitted.
func (s *Service) Delete(ctx context.Context, id string, marked ...bool) error {
	parsed, e := uuid.Parse(id)
	if e != nil {
		return ErrInvalid
	}
	id = parsed.String()
	soft := len(marked) == 0 || marked[0]
	return s.db.Transaction(ctx, func(db Store) error {
		all, e := db.List(ctx)
		if e != nil {
			return e
		}
		f, e := find(all, id)
		if e != nil {
			return e
		}
		for _, child := range all {
			if child.ParentID != nil && *child.ParentID == id {
				return ErrConflict
			}
		}
		if soft {
			now := time.Now().UTC()
			f.DeletedAt = &now
			f.UpdatedAt = now
			return db.Save(ctx, f)
		}
		if s.objects == nil {
			return ErrUnavailable
		}
		if e = s.objects.Delete(ctx, f.S3Key); e != nil {
			return e
		}
		return db.Delete(ctx, id)
	})
}

// Content reads an active file through authenticated RMS, without exposing S3 credentials.
func (s *Service) Content(ctx context.Context, id string) (models.File, io.ReadCloser, error) {
	f, err := s.Get(ctx, id, "")
	if err != nil {
		return f, nil, err
	}
	if !f.IsFile || f.DeletedAt != nil {
		return f, nil, ErrNotFound
	}
	reader, ok := s.objects.(interface {
		Open(context.Context, string) (io.ReadCloser, error)
	})
	if !ok {
		return f, nil, ErrUnavailable
	}
	body, err := reader.Open(ctx, f.S3Key)
	return f, body, err
}
