package task

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/services"
)

type Storage interface {
	TaskList(context.Context, string, models.TaskFilter) ([]json.RawMessage, error)
	TaskByID(context.Context, string, int64) (json.RawMessage, error)
	TaskWrite(context.Context, string, int64, json.RawMessage, int64) (json.RawMessage, error)
	TaskDelete(context.Context, string, int64) error
}
type Service struct{ storage Storage }

func New(s Storage) *Service { return &Service{s} }
func validKind(k string) bool {
	return k == "task_types" || k == "task_scheduling" || k == "tasks" || k == "task_status_history"
}
func status(s string) bool {
	return s == "new" || s == "in_progress" || s == "completed" || s == "cancelled"
}
func timestamp(s string) bool {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05"} {
		if _, e := time.Parse(layout, s); e == nil {
			return true
		}
	}
	return false
}
func strict(data json.RawMessage, v any) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return false
	}
	return d.Decode(new(any)) == io.EOF
}

var color = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func normalize(k string, data json.RawMessage) (json.RawMessage, error) {
	var v any
	switch k {
	case "task_types":
		in := models.TaskTypeInput{}
		if !strict(data, &in) || strings.TrimSpace(in.Name) == "" || len(in.Icon) > 1024*1024 || !color.MatchString(in.Color) || !color.MatchString(in.ColorDark) {
			return nil, services.ErrInvalid
		}
		if in.Service != models.TaskServiceCashCollection && in.Service != models.TaskServicePricePrint && in.Service != models.TaskServiceWithoutService {
			return nil, services.ErrInvalid
		}
		if len(in.Icon) > 0 {
			switch http.DetectContentType(in.Icon) {
			case "image/png", "image/jpeg", "image/gif", "image/webp":
			default:
				return nil, services.ErrInvalid
			}
		}
		if len(in.CustomFields) == 0 || string(in.CustomFields) == "null" {
			in.CustomFields = json.RawMessage(`{}`)
		}
		var fields map[string]any
		if json.Unmarshal(in.CustomFields, &fields) != nil || fields == nil {
			return nil, services.ErrInvalid
		}
		in.Name = strings.TrimSpace(in.Name)
		v = in
	case "task_scheduling":
		in := models.TaskSchedulingInput{}
		if !strict(data, &in) || in.TaskTypeID <= 0 || in.Frequency < 1 || in.Frequency > 10 || len(in.Days) != int(in.Frequency)*7 {
			return nil, services.ErrInvalid
		}
		from, e := time.Parse("2006-01-02", in.PeriodFrom)
		if e != nil {
			return nil, services.ErrInvalid
		}
		to, e := time.Parse("2006-01-02", in.PeriodTo)
		if e != nil {
			return nil, services.ErrInvalid
		}
		from = from.AddDate(0, 0, -(int(from.Weekday())+6)%7)
		to = to.AddDate(0, 0, (7-int(to.Weekday()))%7)
		if to.Before(from) {
			return nil, services.ErrInvalid
		}
		in.PeriodFrom = from.Format("2006-01-02")
		in.PeriodTo = to.Format("2006-01-02")
		if _, e = time.Parse("15:04:05", in.Time); in.Time != "" && e != nil {
			if _, e = time.Parse("15:04", in.Time); e != nil {
				return nil, services.ErrInvalid
			}
		}
		for _, d := range in.Days {
			if d != 0 && d != 1 {
				return nil, services.ErrInvalid
			}
		}
		seen := map[string]bool{}
		stores := []string{}
		for _, id := range in.Stores {
			u, e := uuid.Parse(id)
			if e != nil || u == uuid.Nil {
				return nil, services.ErrInvalid
			}
			id = u.String()
			if !seen[id] {
				stores = append(stores, id)
				seen[id] = true
			}
		}
		in.Stores = stores
		v = in
	case "tasks":
		in := models.TaskInput{}
		if !strict(data, &in) || in.TypeID <= 0 || !status(in.Status) || !timestamp(in.Start) {
			return nil, services.ErrInvalid
		}
		if u, e := uuid.Parse(in.StoreID); e != nil || u == uuid.Nil {
			return nil, services.ErrInvalid
		}
		if string(in.CustomFields) == "null" {
			return nil, services.ErrInvalid
		}
		if len(in.CustomFields) == 0 {
			in.CustomFields = json.RawMessage(`{}`)
		}
		v = in
	case "task_status_history":
		in := models.TaskHistoryInput{}
		if !strict(data, &in) || in.TaskID <= 0 || in.UserID <= 0 || !status(in.Status) || !timestamp(in.Period) {
			return nil, services.ErrInvalid
		}
		v = in
	default:
		return nil, services.ErrInvalid
	}
	return json.Marshal(v)
}
func (s *Service) List(ctx context.Context, k string, f models.TaskFilter) ([]json.RawMessage, error) {
	if !validKind(k) || f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.ID < 0 || f.TypeID < 0 || f.TaskID < 0 || f.UserID < 0 {
		return nil, services.ErrInvalid
	}
	if f.StoreID != "" {
		if _, e := uuid.Parse(f.StoreID); e != nil {
			return nil, services.ErrInvalid
		}
	}
	for _, p := range []string{f.PeriodFrom, f.PeriodTo} {
		if p != "" && !timestamp(p) {
			return nil, services.ErrInvalid
		}
	}
	v, e := s.storage.TaskList(ctx, k, f)
	return v, services.PersonnelError(e)
}
func (s *Service) ByID(ctx context.Context, k string, id int64) (json.RawMessage, error) {
	if !validKind(k) || id <= 0 {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.TaskByID(ctx, k, id)
	return v, services.PersonnelError(e)
}
func (s *Service) Write(ctx context.Context, k string, id int64, data json.RawMessage, userID int64) (json.RawMessage, error) {
	if id < 0 || (k == "tasks" && userID <= 0) {
		return nil, services.ErrInvalid
	}
	in, e := normalize(k, data)
	if e != nil {
		return nil, e
	}
	v, e := s.storage.TaskWrite(ctx, k, id, in, userID)
	return v, services.PersonnelError(e)
}
func (s *Service) Delete(ctx context.Context, k string, id int64) error {
	if !validKind(k) || id <= 0 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.TaskDelete(ctx, k, id))
}
