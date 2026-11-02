package postgres

import (
	"context"
	"encoding/json"
	"rms/internal/domain/models"
	"rms/internal/services/task"
	"testing"
)

func TestTaskStatusHistoryAutomatic(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	if _, err := db.db.Exec(`INSERT INTO stores(id,code,name,address) VALUES('00000000-0000-0000-0000-000000000001','001','Store','');
 INSERT INTO users(id,user_token,name,password) VALUES(100,'first','First','hash'),(101,'second','Second','hash');
 INSERT INTO task_types(id,name,service) VALUES(1,'Task','without_service')`); err != nil {
		t.Fatal(err)
	}
	s := task.New(db)
	input := models.TaskInput{TypeID: 1, StoreID: "00000000-0000-0000-0000-000000000001", Start: "2026-11-02T09:00:00", Status: "new", CustomFields: json.RawMessage(`{}`)}
	write := func(id, userID int64) (json.RawMessage, error) {
		data, _ := json.Marshal(input)
		return s.Write(ctx, "tasks", id, data, userID)
	}
	var id int64
	out, err := write(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatal(err)
	}
	id = created.ID
	check := func(count int, status string, userID int64) {
		t.Helper()
		var gotCount int
		var gotStatus string
		var gotUser int64
		var currentDate bool
		if err := db.db.QueryRow(`SELECT count(*) FROM task_status_history WHERE task_id=$1`, id).Scan(&gotCount); err != nil {
			t.Fatal(err)
		}
		if err := db.db.QueryRow(`SELECT status,user_id,period::date=CURRENT_DATE FROM task_status_history WHERE task_id=$1 ORDER BY id DESC LIMIT 1`, id).Scan(&gotStatus, &gotUser, &currentDate); err != nil {
			t.Fatal(err)
		}
		if gotCount != count || gotStatus != status || gotUser != userID || !currentDate {
			t.Fatalf("history: count=%d status=%s user=%d currentDate=%v", gotCount, gotStatus, gotUser, currentDate)
		}
	}
	check(1, "new", 100)
	input.Start = "2026-11-03T10:00:00"
	if _, err = write(id, 101); err != nil {
		t.Fatal(err)
	}
	check(1, "new", 100)
	input.Status = "in_progress"
	if _, err = write(id, 101); err != nil {
		t.Fatal(err)
	}
	check(2, "in_progress", 101)
	// Concurrent identical changes must generate only one history entry.
	input.Status = "completed"
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, e := write(id, 100); results <- e }()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	check(3, "completed", 100)
	input.Status = "cancelled"
	if _, err = write(id, 999); err == nil {
		t.Fatal("missing history author accepted")
	}
	var status string
	if err = db.db.QueryRow("SELECT status FROM tasks WHERE id=$1", id).Scan(&status); err != nil || status != "completed" {
		t.Fatalf("task update was not rolled back: %s %v", status, err)
	}
	check(3, "completed", 100)
	if _, err = write(0, 999); err == nil {
		t.Fatal("task created with missing history author")
	}
	var count int
	if err = db.db.QueryRow("SELECT count(*) FROM tasks").Scan(&count); err != nil || count != 1 {
		t.Fatalf("creation was not rolled back: %d %v", count, err)
	}
	if _, err = write(id, 0); err == nil {
		t.Fatal("task written without an actor")
	}
}
