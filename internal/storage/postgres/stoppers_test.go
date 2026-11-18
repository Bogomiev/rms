package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/storage"
	"testing"
	"time"
)

func TestStopperPositionsLifecycle(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	if _, err := s.AddStore(ctx, &store); err != nil {
		t.Fatal(err)
	}
	promo, manual, doc := uuid.New(), uuid.New(), uuid.New()
	if err := s.UpsertProducts(ctx, []models.Product{{ID: promo, Code: "001", Name: "Икра", MarkingType: "none", IsThermalMode: true}, {ID: manual, Code: "002", Name: "Чай", MarkingType: "none"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePrices(ctx, []models.Price{
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: promo, DocID: uuid.New(), Price: "1490"},
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: manual, DocID: uuid.New(), Price: "490"},
		{Period: "2026-11-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: promo, DocID: doc, Price: "1190"},
		{Period: "2026-12-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: promo, DocID: uuid.New(), Price: "0"},
	}); err != nil {
		t.Fatal(err)
	}
	window := models.StopperPositionFilter{From: "2026-11-02", To: "2026-11-16", Stores: []uuid.UUID{store.ID}}
	got, err := s.StopperPositions(ctx, window, "UTC")
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	v := got[0]
	if *v.DocID != doc || v.Manual || v.Stores != 1 || v.StopperID != nil || v.From != "2026-11-01" || v.To == nil || *v.To != "2026-11-30" || v.Received != "2026-11-01" || len(v.Prices) != 1 || *v.Prices[0].Price != "1490.00" || *v.Prices[0].PromoPrice != "1190.00" {
		t.Fatalf("%+v", v)
	}
	id, err := s.AddStopper(ctx, models.StopperInput{Name: "Сезон", Color: "#1F5FBF"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddStopper(ctx, models.StopperInput{Name: "сезон", Color: "#1F5FBF"}); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("duplicate name:", err)
	}
	key := models.StopperPositionKey{DocID: &doc, ProductID: promo}
	if err = s.SetPositionsStopper(ctx, []models.StopperPositionKey{key}, &id); err != nil {
		t.Fatal(err)
	}
	to := "2026-11-20"
	if err = s.SetPositionsPeriod(ctx, []models.StopperPositionKey{key}, "2026-11-10", &to); err != nil {
		t.Fatal(err)
	}
	if err = s.AddManualPosition(ctx, models.StopperManualInput{ProductID: manual, StopperID: id, From: "2026-11-09"}); err != nil {
		t.Fatal(err)
	}
	if err = s.AddManualPosition(ctx, models.StopperManualInput{ProductID: manual, StopperID: id, From: "2026-11-09"}); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("second manual position:", err)
	}
	if err = s.SetPositionsStopper(ctx, []models.StopperPositionKey{{ProductID: promo}}, &id); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("unknown manual position:", err)
	}
	got, err = s.StopperPositions(ctx, window, "UTC")
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	byProduct := map[uuid.UUID]models.StopperPosition{}
	for _, v := range got {
		byProduct[v.ProductID] = v
	}
	if v := byProduct[promo]; *v.StopperID != id || v.From != "2026-11-10" || *v.To != "2026-11-20" || !v.ThermalMode {
		t.Fatalf("%+v", v)
	}
	if v := byProduct[manual]; !v.Manual || v.DocID != nil || *v.StopperID != id || v.To != nil || *v.Prices[0].Price != "490.00" || v.Prices[0].PromoPrice != nil {
		t.Fatalf("%+v", v)
	}
	stoppers, err := s.Stoppers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range stoppers {
		if v.ID == id && v.Positions != 2 {
			t.Fatalf("%+v", v)
		}
	}
	if err = s.DeleteStopper(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, err = s.StopperPositions(ctx, window, "UTC")
	if err != nil || len(got) != 2 || got[0].StopperID != nil || got[1].StopperID != nil {
		t.Fatal("positions stay without a stopper:", got, err)
	}
	products, err := s.StopperProducts(ctx, "Ча", 10)
	if err != nil || len(products) != 1 || products[0].ID != manual || !products[0].Added {
		t.Fatal(products, err)
	}
}

func TestStopperTaskForStore(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	other := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	for _, v := range []*models.Store{&store, &other} {
		if _, err := s.AddStore(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	active, ended, foreign, doc, otherDoc := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := s.UpsertProducts(ctx, []models.Product{{ID: active, Code: "001", Name: "Икра", MarkingType: "none", IsWeight: true}, {ID: ended, Code: "002", Name: "Скумбрия", MarkingType: "none"}, {ID: foreign, Code: "003", Name: "Краб", MarkingType: "none"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePrices(ctx, []models.Price{
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: active, DocID: uuid.New(), Price: "1490"},
		{Period: "2026-11-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: active, DocID: doc, Price: "1190"},
		{Period: "2026-10-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: ended, DocID: doc, Price: "799"},
		{Period: "2026-11-01T00:00:00", PriceType: other.PriceTypePromo, ProductID: foreign, DocID: otherDoc, Price: "4990"},
	}); err != nil {
		t.Fatal(err)
	}
	stopper, err := s.AddStopper(ctx, models.StopperInput{Name: "Тест", Color: "#1F5FBF"})
	if err != nil {
		t.Fatal(err)
	}
	endedTo := "2026-11-05"
	for _, key := range []models.StopperPositionKey{{DocID: &doc, ProductID: active}, {DocID: &doc, ProductID: ended}, {DocID: &otherDoc, ProductID: foreign}} {
		if err = s.SetPositionsStopper(ctx, []models.StopperPositionKey{key}, &stopper); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SetPositionsPeriod(ctx, []models.StopperPositionKey{{DocID: &doc, ProductID: ended}}, "2026-10-01", &endedTo); err != nil {
		t.Fatal(err)
	}
	var user, typeID int64
	if err = s.db.QueryRowContext(ctx, "INSERT INTO users(name,user_token,password) VALUES('u','u','x') RETURNING id").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT id FROM task_types WHERE service='stoppers'").Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	raw, err := s.TaskCreateManual(ctx, typeID, store.ID.String(), time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC), user)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID     int64 `json:"id"`
		Manual bool  `json:"manual"`
	}
	if err = json.Unmarshal(raw, &created); err != nil || !created.Manual {
		t.Fatal(string(raw), err)
	}
	task, err := s.StopperTask(ctx, created.ID, "2026-11-10", "UTC")
	if err != nil || task.StoreID != store.ID || len(task.Items) != 2 {
		t.Fatal(task, err)
	}
	if v := task.Items[0]; !v.Remove || v.ProductID != ended || *v.To != endedTo {
		t.Fatalf("%+v", v)
	}
	if v := task.Items[1]; v.Remove || v.ProductID != active || v.Unit != "кг" || v.Stopper != "Тест" || *v.Price != "1490.00" || *v.PromoPrice != "1190.00" || v.Result != "" {
		t.Fatalf("%+v", v)
	}
	status, err := s.SetStopperTaskCheck(ctx, created.ID, models.StopperTaskCheckInput{DocID: &doc, ProductID: active, Result: "bad"}, user)
	if err != nil || status != "in_progress" {
		t.Fatal(status, err)
	}
	if _, err = s.SetStopperTaskCheck(ctx, created.ID, models.StopperTaskCheckInput{DocID: &doc, ProductID: active, Result: "ok"}, user); err != nil {
		t.Fatal(err)
	}
	task, _ = s.StopperTask(ctx, created.ID, "2026-11-10", "UTC")
	if task.Status != "in_progress" || task.Items[1].Result != "ok" {
		t.Fatalf("%+v", task)
	}
	// A store has one active «Стопперы» task a day.
	if _, err = s.TaskCreateManual(ctx, typeID, store.ID.String(), time.Date(2026, 11, 10, 11, 0, 0, 0, time.UTC), user); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("second active task:", err)
	}
	// Schedule reconciliation keeps tasks created by hand; generated tasks are authored by admin.
	if _, err = s.db.ExecContext(ctx, "INSERT INTO users(name,user_token,password) VALUES('admin','admin','x')"); err != nil {
		t.Fatal(err)
	}
	if err = s.TaskGenerate(ctx, time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StopperTask(ctx, created.ID, "2026-11-10", "UTC"); err != nil {
		t.Fatal("manual task removed:", err)
	}
	// A store with nothing to check or remove gets no «Стопперы» task, by hand or by schedule.
	empty := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	if _, err = s.AddStore(ctx, &empty); err != nil {
		t.Fatal(err)
	}
	noon := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if _, err = s.TaskCreateManual(ctx, typeID, empty.ID.String(), noon, user); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("empty stoppers task:", err)
	}
	for _, tc := range []struct {
		store     string
		available bool
	}{{empty.ID.String(), false}, {other.ID.String(), true}, {"", true}} {
		types, err := s.TaskTypesAvailable(ctx, tc.store, noon)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, raw := range types {
			var v struct {
				ID        int64 `json:"id"`
				Available bool  `json:"available"`
			}
			if err = json.Unmarshal(raw, &v); err != nil {
				t.Fatal(err)
			}
			if v.ID == typeID {
				found = true
				if v.Available != tc.available {
					t.Fatalf("store %q: available %v", tc.store, v.Available)
				}
			}
		}
		if !found {
			t.Fatal("stoppers type not listed")
		}
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO task_scheduling(task_type_id,frequency,period_from,period_to,days,time,stores)
 VALUES($1,1,'2026-11-09','2026-11-15','{1,1,1,1,1,1,1}','08:00',ARRAY[$2::uuid,$3::uuid])`, typeID, other.ID, empty.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.TaskGenerate(ctx, noon); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		store uuid.UUID
		tasks int
	}{{other.ID, 1}, {empty.ID, 0}} {
		var n int
		if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE store_id=$1 AND type_id=$2", tc.store, typeID).Scan(&n); err != nil || n != tc.tasks {
			t.Fatal(tc.store, n, err)
		}
	}
}

func TestStopperManagerRole(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	var manager, seller int64
	if err := s.db.QueryRowContext(ctx, "INSERT INTO users(name,user_token,password,position) VALUES('Иванов Иван Иванович','m','x',(SELECT id FROM positions WHERE name='manager')) RETURNING id").Scan(&manager); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "INSERT INTO users(name,user_token,password,position) VALUES('s','s','x',(SELECT id FROM positions WHERE name='seller')) RETURNING id").Scan(&seller); err != nil {
		t.Fatal(err)
	}
	for user, want := range map[int64]bool{manager: true, seller: false, 999999: false} {
		if got, err := s.StopperManager(ctx, user); err != nil || got != want {
			t.Fatal(user, got, err)
		}
	}
}
