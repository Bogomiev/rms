package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"rms/internal/domain/models"
	"rms/internal/storage"
	"strings"
)

// All SQL identifiers come from this closed map, never from request input.
var taskColumns = map[string]string{
	"task_types":          "name,short_name,service,icon,color,color_dark,is_active,sort_order",
	"task_scheduling":     "task_type_id,frequency,period_from,period_to,days,time,is_active,stores",
	"tasks":               "type_id,store_id,start,status,custom_fields",
	"task_status_history": "period,task_id,user_id,status",
}

func taskJSON(k, alias string) string {
	if k == "task_types" {
		return "to_jsonb(" + alias + ") || jsonb_build_object('icon', CASE WHEN " + alias + ".icon IS NULL THEN NULL ELSE replace(encode(" + alias + ".icon,'base64'),chr(10),'') END)"
	}
	return "to_jsonb(" + alias + ")"
}
func (s *Storage) TaskList(ctx context.Context, k string, f models.TaskFilter) ([]json.RawMessage, error) {
	if _, ok := taskColumns[k]; !ok {
		return nil, storage.ErrInvalid
	}
	args := []any{}
	where := []string{"TRUE"}
	add := func(expr string, v any) { args = append(args, v); where = append(where, fmt.Sprintf(expr, len(args))) }
	if f.ID > 0 {
		add("t.id=$%d", f.ID)
	}
	if f.Name != "" {
		if k != "task_types" {
			return nil, storage.ErrInvalid
		}
		add("t.name=$%d", f.Name)
	}
	if f.TypeID > 0 {
		switch k {
		case "task_scheduling":
			add("t.task_type_id=$%d", f.TypeID)
		case "tasks":
			add("t.type_id=$%d", f.TypeID)
		default:
			return nil, storage.ErrInvalid
		}
	}
	if f.StoreID != "" {
		switch k {
		case "task_scheduling":
			add("t.stores @> ARRAY[$%d::uuid]", f.StoreID)
		case "tasks":
			add("t.store_id=$%d::uuid", f.StoreID)
		default:
			return nil, storage.ErrInvalid
		}
	}
	if f.TaskID > 0 || f.UserID > 0 || f.PeriodFrom != "" || f.PeriodTo != "" {
		if k != "task_status_history" {
			return nil, storage.ErrInvalid
		}
	}
	if f.TaskID > 0 {
		add("t.task_id=$%d", f.TaskID)
	}
	if f.UserID > 0 {
		add("t.user_id=$%d", f.UserID)
	}
	if f.PeriodFrom != "" {
		add("t.period >= $%d::timestamp", f.PeriodFrom)
	}
	if f.PeriodTo != "" {
		add("t.period <= $%d::timestamp", f.PeriodTo)
	}
	order := "t.id"
	if k == "task_types" {
		order = "t.sort_order,t.id"
	}
	if k == "task_status_history" {
		order = "t.period DESC,t.id DESC"
	}
	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf("SELECT %s FROM %s t WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d", taskJSON(k, "t"), k, strings.Join(where, " AND "), order, len(args)-1, len(args))
	rows, e := s.db.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, personnelError(e)
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var data json.RawMessage
		if e = rows.Scan(&data); e != nil {
			return nil, e
		}
		out = append(out, data)
	}
	return out, rows.Err()
}
func (s *Storage) TaskByID(ctx context.Context, k string, id int64) (json.RawMessage, error) {
	if _, ok := taskColumns[k]; !ok {
		return nil, storage.ErrInvalid
	}
	var data json.RawMessage
	e := s.db.QueryRowContext(ctx, "SELECT "+taskJSON(k, "t")+" FROM "+k+" t WHERE id=$1", id).Scan(&data)
	return data, personnelError(e)
}
func (s *Storage) TaskWrite(ctx context.Context, k string, id int64, data json.RawMessage) (json.RawMessage, error) {
	cols, ok := taskColumns[k]
	if !ok {
		return nil, storage.ErrInvalid
	}
	if k == "task_types" {
		var in map[string]json.RawMessage
		if e := json.Unmarshal(data, &in); e != nil {
			return nil, e
		}
		var icon []byte
		if e := json.Unmarshal(in["icon"], &icon); e != nil {
			return nil, e
		}
		if icon != nil {
			in["icon"], _ = json.Marshal(`\x` + hex.EncodeToString(icon))
		}
		data, _ = json.Marshal(in)
	}
	names := strings.Split(cols, ",")
	values := make([]string, len(names))
	sets := make([]string, len(names))
	for i, n := range names {
		values[i] = "v." + n
		sets[i] = n + "=v." + n
	}
	var q string
	args := []any{string(data)}
	if id == 0 {
		q = "INSERT INTO " + k + " AS t (" + cols + ") SELECT " + strings.Join(values, ",") + " FROM jsonb_populate_record(NULL::" + k + ",$1::jsonb) v"
	} else {
		q = "UPDATE " + k + " AS t SET " + strings.Join(sets, ",") + " FROM jsonb_populate_record(NULL::" + k + ",$1::jsonb) v WHERE t.id=$2"
		args = append(args, id)
	}
	q += " RETURNING " + taskJSON(k, "t")
	var out json.RawMessage
	e := s.db.QueryRowContext(ctx, q, args...).Scan(&out)
	return out, personnelError(e)
}
func (s *Storage) TaskDelete(ctx context.Context, k string, id int64) error {
	if _, ok := taskColumns[k]; !ok {
		return storage.ErrInvalid
	}
	return affected(s.db.ExecContext(ctx, "DELETE FROM "+k+" WHERE id=$1", id))
}
