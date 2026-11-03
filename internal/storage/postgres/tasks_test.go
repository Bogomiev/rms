package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/services/task"
)

func TestTaskResourcesLifecycle(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	s := task.New(db)
	storeID := "00000000-0000-0000-0000-000000000001"
	if _, e := db.db.Exec("INSERT INTO stores(id,code,name,address) VALUES($1,'001','Store','')", storeID); e != nil {
		t.Fatal(e)
	}
	if _, e := db.db.Exec("INSERT INTO users(id,user_token,name,password) VALUES(100,'employee','User','hash')"); e != nil {
		t.Fatal(e)
	}
	write := func(k string, id int64, in any) map[string]json.RawMessage {
		t.Helper()
		data, _ := json.Marshal(in)
		out, e := s.Write(ctx, k, id, data, 100)
		if e != nil {
			t.Fatalf("%s: %v", k, e)
		}
		var record map[string]json.RawMessage
		json.Unmarshal(out, &record)
		return record
	}
	idOf := func(m map[string]json.RawMessage) int64 { var id int64; json.Unmarshal(m["id"], &id); return id }
	icon, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j6xkAAAAASUVORK5CYII=")
	typ := models.TaskTypeInput{Name: "Test", Service: "cash_collection", Icon: icon, Color: "#ffffff", ColorDark: "#000000", IsActive: true}
	created := write("task_types", 0, typ)
	typeID := idOf(created)
	var gotIcon []byte
	json.Unmarshal(created["icon"], &gotIcon)
	if string(gotIcon) != string(icon) {
		t.Fatal("icon roundtrip failed")
	}
	data, _ := json.Marshal(typ)
	if _, e := s.Write(ctx, "task_types", 0, data, 100); !errors.Is(e, services.ErrConflict) {
		t.Fatal("duplicate name", e)
	}
	typ.Name = "Renamed"
	typ.Icon = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M3 7h18" stroke="#fff"/></svg>`)
	updated := write("task_types", typeID, typ)
	if err := json.Unmarshal(updated["icon"], &gotIcon); err != nil || string(gotIcon) != string(typ.Icon) {
		t.Fatalf("SVG update roundtrip: %s, %v", updated["icon"], err)
	}
	stored, err := s.ByID(ctx, "task_types", typeID)
	if err != nil {
		t.Fatal(err)
	}
	var saved models.TaskTypeInput
	if err := json.Unmarshal(stored, &saved); err != nil || string(saved.Icon) != string(typ.Icon) {
		t.Fatalf("SVG not persisted: %s, %v", stored, err)
	}

	if string(updated["created_at"]) != string(created["created_at"]) || string(updated["update_at"]) == string(created["update_at"]) {
		t.Fatal("timestamps not maintained")
	}
	schedule := models.TaskSchedulingInput{TaskTypeID: int16(typeID), Frequency: 2, PeriodFrom: "2026-11-04", PeriodTo: "2026-11-12", Days: []int16{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}, Time: "09:00", IsActive: true, Stores: []string{storeID}}
	scheduled := write("task_scheduling", 0, schedule)
	scheduleID := idOf(scheduled)
	// Both creating without a time and clearing an existing time store SQL NULL.
	schedule.Time = ""
	withoutTime := write("task_scheduling", 0, schedule)
	if string(withoutTime["time"]) != "null" {
		t.Fatalf("missing time: %s", withoutTime["time"])
	}
	if err := s.Delete(ctx, "task_scheduling", idOf(withoutTime)); err != nil {
		t.Fatal(err)
	}
	cleared := write("task_scheduling", scheduleID, schedule)
	if string(cleared["time"]) != "null" {
		t.Fatalf("cleared time: %s", cleared["time"])
	}
	schedule.Time = "00:00"
	midnight := write("task_scheduling", scheduleID, schedule)
	if string(midnight["time"]) != `"00:00:00"` {
		t.Fatalf("midnight: %s", midnight["time"])
	}
	schedule.Time = "09:00"
	write("task_scheduling", scheduleID, schedule)
	var storedIDs []string
	if err := json.Unmarshal(scheduled["stores"], &storedIDs); err != nil || len(storedIDs) != 1 || storedIDs[0] != storeID {
		t.Fatal("stores array roundtrip", storedIDs, err)
	}
	var hasJoinTable bool
	if err := db.db.QueryRow("SELECT to_regclass('task_scheduling_stores') IS NOT NULL").Scan(&hasJoinTable); err != nil || hasJoinTable {
		t.Fatal("unexpected store join table", hasJoinTable, err)
	}
	if _, err := db.db.Exec("UPDATE stores SET id='00000000-0000-0000-0000-000000000003' WHERE id=$1", storeID); !isErrorPg(err, ERR_ForeignKeyViolation) {
		t.Fatal("array reference did not restrict UUID update", err)
	}
	// Updating the actual array must immediately change membership filtering.
	schedule.Stores = []string{}
	emptied := write("task_scheduling", scheduleID, schedule)
	if string(emptied["stores"]) != "[]" {
		t.Fatal("empty stores array", string(emptied["stores"]))
	}
	filtered, err := s.List(ctx, "task_scheduling", models.TaskFilter{Limit: 100, StoreID: storeID})
	if err != nil || len(filtered) != 0 {
		t.Fatal("removed store still matched schedule", filtered, err)
	}
	schedule.Stores = []string{storeID}
	write("task_scheduling", scheduleID, schedule)

	if string(scheduled["period_from"]) != `"2026-11-02"` || string(scheduled["period_to"]) != `"2026-11-15"` {
		t.Fatal("normalization", scheduled)
	}
	if _, e := db.db.Exec("DELETE FROM stores WHERE id=$1", storeID); !isErrorPg(e, ERR_ForeignKeyViolation) {
		t.Fatal("array FK did not restrict store deletion", e)
	}
	schedule.Stores = []string{"00000000-0000-0000-0000-000000000002"}
	data, _ = json.Marshal(schedule)
	if _, e := s.Write(ctx, "task_scheduling", scheduleID, data, 100); !errors.Is(e, services.ErrConflict) {
		t.Fatal("missing array store accepted", e)
	}
	list, e := s.List(ctx, "task_scheduling", models.TaskFilter{Limit: 100, TypeID: typeID, StoreID: storeID})
	if e != nil || len(list) != 1 {
		t.Fatal("schedule filter/atomic rollback", len(list), e)
	}
	taskID := idOf(write("tasks", 0, models.TaskInput{TypeID: int16(typeID), StoreID: storeID, Start: "2026-11-03T09:00:00", Status: "new", CustomFields: json.RawMessage(`{"amount":100}`)}))
	current, err := db.TaskCurrent(ctx, storeID, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
	if err != nil || len(current) != 1 {
		t.Fatalf("current tasks: %v, %v", current, err)
	}
	var currentIcon struct {
		Icon []byte `json:"icon"`
	}
	if err := json.Unmarshal(current[0], &currentIcon); err != nil || string(currentIcon.Icon) != string(typ.Icon) {
		t.Fatalf("current SVG icon: %s, %v", current[0], err)
	}
	historyID := idOf(write("task_status_history", 0, models.TaskHistoryInput{Period: "2026-11-03T10:00:00", TaskID: taskID, UserID: 100, Status: "in_progress"}))
	write("task_status_history", 0, models.TaskHistoryInput{Period: "2026-11-03T11:00:00", TaskID: taskID, UserID: 100, Status: "completed"})
	list, e = s.List(ctx, "task_status_history", models.TaskFilter{Limit: 1, TaskID: taskID, UserID: 100, PeriodTo: "2026-11-03T10:30:00"})
	if e != nil || len(list) != 1 {
		t.Fatal("historical status filter", list, e)
	}
	var history models.TaskHistoryInput
	json.Unmarshal(list[0], &history)
	if history.Status != "in_progress" {
		t.Fatal(history)
	}
	for k, id := range map[string]int64{"task_types": typeID, "tasks": taskID} {
		if e = s.Delete(ctx, k, id); !errors.Is(e, services.ErrConflict) {
			t.Fatal("deleted referenced", k, e)
		}
	}
	write("task_status_history", historyID, models.TaskHistoryInput{Period: "2026-11-03T10:00:00", TaskID: taskID, UserID: 100, Status: "cancelled"})
	histories, e := s.List(ctx, "task_status_history", models.TaskFilter{Limit: 100, TaskID: taskID})
	if e != nil {
		t.Fatal(e)
	}
	for _, h := range histories {
		var m map[string]json.RawMessage
		json.Unmarshal(h, &m)
		if e = s.Delete(ctx, "task_status_history", idOf(m)); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []struct {
		k  string
		id int64
	}{{"tasks", taskID}, {"task_scheduling", scheduleID}, {"task_types", typeID}} {
		if e = s.Delete(ctx, v.k, v.id); e != nil {
			t.Fatal(v, e)
		}
	}
	if _, e = db.db.Exec("DELETE FROM stores WHERE id=$1", storeID); e != nil {
		t.Fatal("store remained restricted after deleting schedule", e)
	}
	if _, e = s.ByID(ctx, "task_types", typeID); !errors.Is(e, services.ErrNotFound) {
		t.Fatal("deleted record found", e)
	}
}

func TestTaskTypeOptionalColors(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	service := task.New(db)
	in := models.TaskTypeInput{Name: "Optional background", Service: models.TaskServiceCashCollection, Color: "#123456", ColorDark: "#abcdef"}
	write := func(id int64) models.TaskTypeInput {
		t.Helper()
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		out, err := service.Write(ctx, "task_types", id, data, 100)
		if err != nil {
			t.Fatal(err)
		}
		var got models.TaskTypeInput
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	write(0)
	var id int64
	if err := db.db.QueryRow("SELECT id FROM task_types WHERE name=$1", in.Name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, colors := range [][2]string{{"", "#abcdef"}, {"#123456", ""}, {"", ""}, {"#654321", "#fedcba"}} {
		in.Color, in.ColorDark = colors[0], colors[1]
		got := write(id)
		if got.Color != in.Color || got.ColorDark != in.ColorDark {
			t.Fatalf("write lost colors: %+v", got)
		}
		stored, err := service.ByID(ctx, "task_types", id)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(stored, &got); err != nil {
			t.Fatal(err)
		}
		if got.Color != in.Color || got.ColorDark != in.ColorDark {
			t.Fatalf("read lost colors: %+v", got)
		}
	}
	in.Name = "Created without background"
	in.Color, in.ColorDark = "", ""
	got := write(0)
	if got.Color != "" || got.ColorDark != "" {
		t.Fatalf("empty colors not saved: %+v", got)
	}
	for _, column := range []string{"color", "color_dark"} {
		if _, err := db.db.Exec("UPDATE task_types SET "+column+"='invalid' WHERE id=$1", id); !isErrorPg(err, "23514") {
			t.Fatalf("invalid %s accepted: %v", column, err)
		}
	}
}
