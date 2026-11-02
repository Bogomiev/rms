package postgres

import (
	"context"
	"encoding/json"
	"rms/internal/services/task"
	"sync"
	"testing"
	"time"
)

func TestTaskGenerationAndActions(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	svc := task.New(db)
	_, e := db.db.Exec(`INSERT INTO users(id,user_token,name,password,is_admin) VALUES(100,'admin','admin','hash',true);
 INSERT INTO stores(id,code,name,address) VALUES('00000000-0000-0000-0000-000000000001','001','Store','');
 INSERT INTO task_types(id,name,service,custom_fields) VALUES(1,'Collection','cash_collection','{"qrCodeSrc":"image"}');
 INSERT INTO task_scheduling(task_type_id,frequency,period_from,period_to,days,time,stores) VALUES(1,2,'2026-11-02','2026-11-29',ARRAY[1,0,0,0,0,0,0,0,1,0,0,0,0,0]::smallint[],'09:00',ARRAY['00000000-0000-0000-0000-000000000001']::uuid[]);`)
	if e != nil {
		t.Fatal(e)
	}
	zone := time.FixedZone("store", 10*3600)
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, zone)
	count := func(want int) {
		t.Helper()
		var n int
		if e := db.db.QueryRow("SELECT count(*) FROM tasks").Scan(&n); e != nil || n != want {
			t.Fatalf("tasks %d, want %d: %v", n, want, e)
		}
	}
	generate := func(at time.Time) {
		t.Helper()
		if e := svc.Generate(ctx, at); e != nil {
			t.Fatal(e)
		}
	}
	generate(now)
	generate(now)
	count(1)
	current, e := svc.Current(ctx, "00000000-0000-0000-0000-000000000001", now)
	if e != nil || len(current) != 0 {
		t.Fatal("before start", current, e)
	}
	now = now.Add(time.Hour)
	current, e = svc.Current(ctx, "00000000-0000-0000-0000-000000000001", now)
	if e != nil || len(current) != 1 {
		t.Fatal(current, e)
	}
	var saved struct {
		ID           int64
		Service      string
		CustomFields map[string]string `json:"custom_fields"`
	}
	json.Unmarshal(current[0], &saved)
	if saved.Service != "cash_collection" || saved.CustomFields["qrCodeSrc"] != "image" {
		t.Fatal(saved)
	}
	// A failing history insert must roll the status update back.
	if _, e = svc.Transition(ctx, saved.ID, "start", 999999); e == nil {
		t.Fatal("missing actor accepted")
	}
	var state string
	db.db.QueryRow("SELECT status FROM tasks WHERE id=$1", saved.ID).Scan(&state)
	if state != "new" {
		t.Fatal("status escaped rollback", state)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := svc.Transition(ctx, saved.ID, "start", 100); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	var histories int
	db.db.QueryRow("SELECT count(*) FROM task_status_history WHERE task_id=$1", saved.ID).Scan(&histories)
	if histories != 2 {
		t.Fatal("duplicate history", histories)
	}
	if _, e = svc.Transition(ctx, saved.ID, "complete", 100); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Transition(ctx, saved.ID, "complete", 100); e != nil {
		t.Fatal(e)
	}
	current, e = svc.Current(ctx, "00000000-0000-0000-0000-000000000001", now)
	if e != nil || len(current) != 0 {
		t.Fatal("completed visible", e)
	}
	// Changed parameters preserve the completed task and create its replacement.
	if _, e = db.db.Exec("UPDATE task_types SET custom_fields='{}'"); e != nil {
		t.Fatal(e)
	}
	generate(now)
	count(2)
	// Inactive scheduling deletes only the replacement that is still new, with its history.
	if _, e = db.db.Exec("UPDATE task_scheduling SET is_active=false"); e != nil {
		t.Fatal(e)
	}
	generate(now)
	count(1)
	if _, e = db.db.Exec("UPDATE task_scheduling SET is_active=true,time=NULL"); e != nil {
		t.Fatal(e)
	}
	generate(now.AddDate(0, 0, 7))
	count(1) // second Monday disabled
	generate(now.AddDate(0, 0, 8))
	count(2) // second Tuesday enabled
	generate(now.AddDate(0, 0, 14))
	count(3) // cycle repeats
	generate(now.AddDate(0, 0, 28))
	count(3) // outside period
	// Terminal cancellation is idempotent and its history remains atomic.
	var id int64
	db.db.QueryRow("SELECT id FROM tasks WHERE status='new' ORDER BY id LIMIT 1").Scan(&id)
	for i := 0; i < 2; i++ {
		if _, e = svc.Transition(ctx, id, "cancel", 100); e != nil {
			t.Fatal(e)
		}
	}
	db.db.QueryRow("SELECT count(*) FROM task_status_history WHERE task_id=$1", id).Scan(&histories)
	if histories != 2 {
		t.Fatal(histories)
	}
}

func TestTaskGenerationByType(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	svc := task.New(db)
	_, err := db.db.Exec(`INSERT INTO users(id,user_token,name,password,is_admin) VALUES(100,'admin','admin','hash',true);
 INSERT INTO stores(id,code,name,address) VALUES('00000000-0000-0000-0000-000000000001','001','Store','');
 INSERT INTO task_types(id,name,service) VALUES(1,'First','without_service'),(2,'Second','without_service');
 INSERT INTO task_scheduling(task_type_id,frequency,period_from,period_to,days,stores)
 SELECT id,1,'2026-11-02','2026-11-08',ARRAY[1,0,0,0,0,0,0]::smallint[],ARRAY['00000000-0000-0000-0000-000000000001']::uuid[] FROM task_types;`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	if err = svc.GenerateType(ctx, 1, now); err != nil {
		t.Fatal(err)
	}
	var first, second int
	check := func(wantFirst, wantSecond int) {
		t.Helper()
		if err := db.db.QueryRow("SELECT count(*) FILTER (WHERE type_id=1),count(*) FILTER (WHERE type_id=2) FROM tasks").Scan(&first, &second); err != nil || first != wantFirst || second != wantSecond {
			t.Fatalf("counts %d/%d, want %d/%d: %v", first, second, wantFirst, wantSecond, err)
		}
	}
	check(1, 0)
	if err = svc.Generate(ctx, now); err != nil {
		t.Fatal(err)
	}
	check(1, 1)
	if _, err = db.db.Exec("UPDATE task_scheduling SET is_active=false"); err != nil {
		t.Fatal(err)
	}
	if err = svc.GenerateType(ctx, 1, now); err != nil {
		t.Fatal(err)
	}
	check(0, 1)
	if err = svc.GenerateType(ctx, 999, now); err == nil {
		t.Fatal("missing type accepted")
	}
	check(0, 1)
}
