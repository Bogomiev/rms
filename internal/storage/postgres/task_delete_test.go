package postgres

import (
	"context"
	"testing"
)

func TestTaskDeleteWithStatusHistory(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	if _, err := db.db.Exec(`
 INSERT INTO stores(id,code,name,address) VALUES('00000000-0000-0000-0000-000000000001','001','Store','');
 INSERT INTO users(id,user_token,name,password) VALUES(100,'delete-test','Test','hash');
 INSERT INTO task_types(id,name,service) VALUES(1,'Task','without_service');
 INSERT INTO tasks(id,type_id,store_id,start,status) VALUES
 (1,1,'00000000-0000-0000-0000-000000000001','2026-11-02 09:00:00','in_progress'),
 (2,1,'00000000-0000-0000-0000-000000000001','2026-11-02 10:00:00','new');
 INSERT INTO task_status_history(task_id,user_id,status,period) VALUES
 (1,100,'new',CURRENT_TIMESTAMP),(1,100,'in_progress',CURRENT_TIMESTAMP),(2,100,'new',CURRENT_TIMESTAMP);
 `); err != nil {
		t.Fatal(err)
	}
	if err := db.TaskDelete(ctx, "tasks", 1); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query string
		want  int
	}{
		{"SELECT count(*) FROM tasks WHERE id=1", 0},
		{"SELECT count(*) FROM task_status_history WHERE task_id=1", 0},
		{"SELECT count(*) FROM tasks WHERE id=2", 1},
		{"SELECT count(*) FROM task_status_history WHERE task_id=2", 1},
	} {
		var count int
		if err := db.db.QueryRowContext(ctx, check.query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%s: got %d, want %d", check.query, count, check.want)
		}
	}
}
