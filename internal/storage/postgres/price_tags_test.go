package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
	"rms/internal/storage"
)

// currentTasks lists the open tasks of the store at the moment by id with their service.
func currentTasks(t *testing.T, s *Storage, store uuid.UUID, now time.Time) map[int64]string {
	t.Helper()
	list, err := s.TaskCurrent(context.Background(), store.String(), now)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]string{}
	for _, raw := range list {
		var v struct {
			ID      int64  `json:"id"`
			Service string `json:"service"`
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		out[v.ID] = v.Service
	}
	return out
}

func taskOf(t *testing.T, tasks map[int64]string, service string) int64 {
	t.Helper()
	for id, v := range tasks {
		if v == service {
			return id
		}
	}
	return 0
}

// createdTask reads the id of a task created by TaskCreateManual.
func createdTask(t *testing.T) func(json.RawMessage, error) int64 {
	return func(raw json.RawMessage, err error) int64 {
		t.Helper()
		var v struct {
			ID int64 `json:"id"`
		}
		if err != nil || json.Unmarshal(raw, &v) != nil {
			t.Fatal(string(raw), err)
		}
		return v.ID
	}
}

// The standing «Печать ценников» task appears on 2026-11-10 for a new regular price, a stopper promotion
// that starts, one that ended the day before and a «Не соответствует» mark; it is never closed and lists
// what is to print and what was printed on the day. Printed products are printed again on a new reason or
// from the product card, and the print made before is kept.
func TestPriceTagTaskForStore(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	empty := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	for _, v := range []*models.Store{&store, &empty} {
		if _, err := s.AddStore(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	reval, start, ended, bad, manual := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	revalDoc, startDoc, endedDoc, badDoc := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := s.UpsertProducts(ctx, []models.Product{
		{ID: reval, Code: "001", Name: "Хлеб", MarkingType: "none"},
		{ID: start, Code: "002", Name: "Икра", MarkingType: "none", IsWeight: true},
		{ID: ended, Code: "003", Name: "Кальмар", MarkingType: "none"},
		{ID: bad, Code: "004", Name: "Краб", MarkingType: "none"},
		{ID: manual, Code: "005", Name: "Чай", MarkingType: "none", Barcodes: []models.BarcodeInfo{{Barcode: "4601234567893", Unit: "шт", Ratio: 1, IsBase: true}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePrices(ctx, []models.Price{
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: reval, DocID: uuid.New(), Price: "100"},
		{Period: "2026-11-10T00:00:00", PriceType: store.PriceType, ProductID: reval, DocID: revalDoc, Price: "120"},
		{Period: "2026-11-11T00:00:00", PriceType: store.PriceType, ProductID: reval, DocID: uuid.New(), Price: "130"},
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: start, DocID: uuid.New(), Price: "200"},
		{Period: "2026-11-10T00:00:00", PriceType: store.PriceTypePromo, ProductID: start, DocID: startDoc, Price: "150"},
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: ended, DocID: uuid.New(), Price: "300"},
		{Period: "2026-11-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: ended, DocID: endedDoc, Price: "250"},
		{Period: "2026-11-10T00:00:00", PriceType: store.PriceTypePromo, ProductID: ended, DocID: uuid.New(), Price: "0"},
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: bad, DocID: uuid.New(), Price: "900"},
		{Period: "2026-11-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: bad, DocID: badDoc, Price: "700"},
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: manual, DocID: uuid.New(), Price: "50"},
	}); err != nil {
		t.Fatal(err)
	}
	// 1C sends today's price changes of the store: a new regular price and a new promotional one; the tags are
	// printed with these prices, not with the price records.
	if err := s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{{StoreID: store.ID, ProductID: reval, Price: "125"}, {StoreID: store.ID, ProductID: bad, Price: "650", IsPromo: true}}); err != nil {
		t.Fatal(err)
	}
	// The changes arrived the evening before (the sync clock is the database clock; the test days are fixed):
	// a change is dated not before the day.
	arrived := func(product uuid.UUID, at string) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, "UPDATE today_price_changes SET arrived_at=$2::timestamp AT TIME ZONE 'UTC' WHERE product_id=$1", product, at); err != nil {
			t.Fatal(err)
		}
	}
	arrived(reval, "2026-11-09 20:00")
	arrived(bad, "2026-11-09 20:00")
	stopper, err := s.AddStopper(ctx, models.StopperInput{Name: "Тест", Color: "#1F5FBF"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []models.StopperPositionKey{{DocID: &startDoc, ProductID: start}, {DocID: &endedDoc, ProductID: ended}, {DocID: &badDoc, ProductID: bad}} {
		if err = s.SetPositionsStopper(ctx, []models.StopperPositionKey{key}, &stopper); err != nil {
			t.Fatal(err)
		}
	}
	endedTo := "2026-11-09"
	if err = s.SetPositionsPeriod(ctx, []models.StopperPositionKey{{DocID: &endedDoc, ProductID: ended}}, "2026-11-01", &endedTo); err != nil {
		t.Fatal(err)
	}
	var user, stopperType, priceType int64
	if err = s.db.QueryRowContext(ctx, "INSERT INTO users(name,user_token,password) VALUES('admin','admin','x') RETURNING id").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT (SELECT id FROM task_types WHERE service='stoppers'),(SELECT id FROM task_types WHERE service='price_print')").Scan(&stopperType, &priceType); err != nil {
		t.Fatal(err)
	}
	// A «Не соответствует» mark in a «Стопперы» task asks for a new tag.
	morning := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	stoppers := createdTask(t)(s.TaskCreateManual(ctx, stopperType, store.ID.String(), morning, user))
	if _, err = s.SetStopperTaskCheck(ctx, stoppers, models.StopperTaskCheckInput{DocID: &badDoc, ProductID: bad, Result: "bad"}, user); err != nil {
		t.Fatal(err)
	}
	// The mark time is set by the database clock; the test days are fixed.
	if _, err = s.db.ExecContext(ctx, "UPDATE stopper_task_checks SET checked_at=$1::timestamp AT TIME ZONE 'UTC' AT TIME ZONE current_setting('TimeZone')", "2026-11-10 09:30"); err != nil {
		t.Fatal(err)
	}

	// The task is not created by hand; it appears by itself in a store with price tags to print.
	noon := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if _, err = s.TaskCreateManual(ctx, priceType, store.ID.String(), noon, user); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("price tag task by hand:", err)
	}
	types, err := s.TaskTypesAvailable(ctx, store.ID.String(), noon)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range types {
		var v struct {
			Service string `json:"service"`
		}
		if json.Unmarshal(raw, &v) != nil || v.Service == models.TaskServicePricePrint {
			t.Fatal("price tag task offered to create:", string(raw))
		}
	}
	if got := currentTasks(t, s, empty.ID, noon); len(got) != 0 {
		t.Fatal("store without price tags:", got)
	}
	tags := taskOf(t, currentTasks(t, s, store.ID, noon), models.TaskServicePricePrint)
	if tags == 0 || taskOf(t, currentTasks(t, s, store.ID, noon), models.TaskServicePricePrint) != tags {
		t.Fatal("one standing task:", tags)
	}

	task, err := s.PriceTagTask(ctx, tags, noon)
	if err != nil || task.StoreID != store.ID || task.Status != "new" || len(task.Items) != 4 {
		t.Fatal(task, err)
	}
	items := map[uuid.UUID]models.PriceTagItem{}
	for _, v := range task.Items {
		items[v.ProductID] = v
		if !v.Pending || v.PrintedAt != nil || v.LastPrintedAt != nil {
			t.Fatalf("to print %+v", v)
		}
	}
	if v := items[reval]; v.Source != models.PriceTagRevaluation || v.TagType != "regular" || v.DocID != nil || *v.Price != "125.00" || *v.PriceFrom != "2026-11-10" || v.PromoPrice != nil || v.ActualAt != "2026-11-10T00:00:00" {
		t.Fatalf("revaluation %+v", v)
	}
	if v := items[start]; v.Source != models.PriceTagPromoStart || v.TagType != "promo" || !v.IsWeight || *v.Price != "200.00" || *v.PromoPrice != "150.00" || *v.PromoFrom != "2026-11-10" || v.PromoTo != nil {
		t.Fatalf("promotion start %+v", v)
	}
	if v := items[ended]; v.Source != models.PriceTagPromoEnd || v.TagType != "regular" || *v.Price != "300.00" || v.PromoPrice != nil {
		t.Fatalf("promotion end %+v", v)
	}
	if v := items[bad]; v.Source != models.PriceTagStopperBad || v.TagType != "promo" || *v.Price != "900.00" || *v.PromoPrice != "650.00" || *v.PromoFrom != "2026-11-10" {
		t.Fatalf("stopper mark %+v", v)
	}

	// Products are found by name, code or barcode; adding one twice adds a tag.
	found, err := s.PriceTagProducts(ctx, tags, "4601234567893", 20)
	if err != nil || len(found) != 1 || found[0].ID != manual || found[0].Added {
		t.Fatal(found, err)
	}
	for range 2 {
		if err = s.AddPriceTag(ctx, tags, manual, noon); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.AddPriceTag(ctx, tags, uuid.New(), noon); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("unknown product:", err)
	}
	card, err := s.PriceTagProduct(ctx, store.ID.String(), manual, noon)
	if err != nil || card.TaskID == nil || *card.TaskID != tags || card.ItemID == nil || !card.Pending || card.Quantity != 2 || *card.Source != models.PriceTagManual || card.LastPrintedAt != nil || card.Stopper != nil {
		t.Fatalf("%+v %v", card, err)
	}
	// The product card shows the stopper of the product on the shelf.
	if card, err = s.PriceTagProduct(ctx, store.ID.String(), bad, noon); err != nil || card.Stopper == nil || card.Stopper.Name != "Тест" || card.Stopper.Color != "#1F5FBF" {
		t.Fatalf("stopper %+v %v", card, err)
	}
	if card, _ = s.PriceTagProduct(ctx, store.ID.String(), manual, noon); card.ItemID == nil {
		t.Fatal("manual position")
	}
	manualItem := *card.ItemID
	// Weighed goods are priced per 1 кг (default) or 100 г, others per piece.
	if items[start].Unit != models.PriceTagUnitKg || items[reval].Unit != models.PriceTagUnitPiece {
		t.Fatalf("units %+v %+v", items[start], items[reval])
	}
	if err = s.SetPriceTag(ctx, tags, items[start].ID, models.PriceTagItemInput{TagType: "promo", Quantity: 1, Unit: models.PriceTagUnitPiece}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatal("weighed goods per piece:", err)
	}
	if err = s.SetPriceTag(ctx, tags, manualItem, models.PriceTagItemInput{TagType: "regular", Quantity: 5, Unit: models.PriceTagUnitKg}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatal("piece goods per kg:", err)
	}
	if err = s.SetPriceTag(ctx, tags, items[start].ID, models.PriceTagItemInput{TagType: "promo", Quantity: 1, Unit: models.PriceTagUnit100g}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPriceTag(ctx, tags, manualItem, models.PriceTagItemInput{TagType: "regular", Quantity: 5, Unit: models.PriceTagUnitPiece}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeletePriceTag(ctx, tags, items[reval].ID, user); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("price reason removed:", err)
	}

	// The first print starts the task; positions of another task are refused.
	ids := []int64{items[reval].ID, items[start].ID, items[ended].ID, items[bad].ID}
	if _, err = s.SetPriceTagsPrinted(ctx, tags, []int64{manualItem, 999999}, user, noon); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("foreign position printed:", err)
	}
	status, err := s.SetPriceTagsPrinted(ctx, tags, ids, user, noon)
	if err != nil || status != "in_progress" {
		t.Fatal(status, err)
	}
	if card, _ = s.PriceTagProduct(ctx, store.ID.String(), reval, noon); card.Pending || card.LastPrintedAt == nil || *card.LastPrintedAt != "2026-11-10T12:00:00" {
		t.Fatalf("printed %+v", card)
	}
	// The task is never closed: with nothing left to print it stays, showing today's prints.
	if status, err = s.DeletePriceTag(ctx, tags, manualItem, user); err != nil || status != "in_progress" {
		t.Fatal(status, err)
	}
	if got := taskOf(t, currentTasks(t, s, store.ID, noon), models.TaskServicePricePrint); got != tags {
		t.Fatal("printed today, still shown:", got)
	}
	if task, err = s.PriceTagTask(ctx, tags, noon); err != nil || task.Status != "in_progress" || len(task.Items) != 4 {
		t.Fatal(task, err)
	}

	// A price change that comes after the print the same day puts the product to print again with the new
	// price; an unchanged row keeps the time it arrived.
	if err = s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{{StoreID: store.ID, ProductID: reval, Price: "127"}, {StoreID: store.ID, ProductID: bad, Price: "650", IsPromo: true}}); err != nil {
		t.Fatal(err)
	}
	var kept bool
	if err = s.db.QueryRowContext(ctx, "SELECT arrived_at=$2::timestamp AT TIME ZONE 'UTC' FROM today_price_changes WHERE product_id=$1", bad, "2026-11-09 20:00").Scan(&kept); err != nil || !kept {
		t.Fatal("unchanged row restamped:", err)
	}
	arrived(reval, "2026-11-10 14:00")
	afternoon := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	if task, err = s.PriceTagTask(ctx, tags, afternoon); err != nil {
		t.Fatal(err)
	}
	for _, v := range task.Items {
		if v.ProductID == reval && (!v.Pending || *v.Price != "127.00" || v.ActualAt != "2026-11-10T14:00:00" || *v.LastPrintedAt != "2026-11-10T12:00:00") {
			t.Fatalf("late change %+v", v)
		}
		if v.ProductID != reval && v.Pending {
			t.Fatalf("unchanged %+v", v)
		}
	}
	if _, err = s.SetPriceTagsPrinted(ctx, tags, []int64{items[reval].ID}, user, afternoon); err != nil {
		t.Fatal(err)
	}

	// The next day the same task lists only the new price; yesterday's prints are gone from it.
	next := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	// 1C sends the price changes of the new day.
	if err = s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{{StoreID: store.ID, ProductID: reval, Price: "130"}}); err != nil {
		t.Fatal(err)
	}
	arrived(reval, "2026-11-11 00:30")
	lastPrint := map[uuid.UUID]string{reval: "2026-11-10T15:00:00", start: "2026-11-10T12:00:00"}
	if got := taskOf(t, currentTasks(t, s, store.ID, next), models.TaskServicePricePrint); got != tags {
		t.Fatal("one task for a new price:", got)
	}
	// A printed product added from the product card is printed again.
	if card, err = s.AddPriceTagProduct(ctx, store.ID.String(), start, next); err != nil || card.TaskID == nil || *card.TaskID != tags || !card.Pending || card.Quantity != 1 || *card.Source != models.PriceTagManual || *card.LastPrintedAt != "2026-11-10T12:00:00" {
		t.Fatalf("%+v %v", card, err)
	}
	task, err = s.PriceTagTask(ctx, tags, next)
	if err != nil || len(task.Items) != 2 {
		t.Fatal(task, err)
	}
	ids = ids[:0]
	for _, v := range task.Items {
		ids = append(ids, v.ID)
		if !v.Pending || v.LastPrintedAt == nil || *v.LastPrintedAt != lastPrint[v.ProductID] {
			t.Fatalf("%+v", v)
		}
		if v.ProductID == reval && (v.Source != models.PriceTagRevaluation || *v.Price != "130.00" || v.ActualAt != "2026-11-11T00:30:00") {
			t.Fatalf("new price %+v", v)
		}
	}
	if status, err = s.SetPriceTagsPrinted(ctx, tags, ids, user, next); err != nil || status != "in_progress" {
		t.Fatal("everything printed:", status, err)
	}
	// A printed position keeps showing the print made before it was put to print.
	task, _ = s.PriceTagTask(ctx, tags, next)
	for _, v := range task.Items {
		if v.Pending || *v.PrintedAt != "2026-11-11T10:00:00" || v.LastPrintedAt == nil || *v.LastPrintedAt != lastPrint[v.ProductID] {
			t.Fatalf("printed %+v", v)
		}
	}
	// With nothing to print and nothing printed on the day, the task is not shown.
	if err = s.ReplaceTodayPriceChanges(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := taskOf(t, currentTasks(t, s, store.ID, next.AddDate(0, 0, 1)), models.TaskServicePricePrint); got != 0 {
		t.Fatal("idle task shown:", got)
	}
	// The product card puts a product into the same task again.
	if card, err = s.AddPriceTagProduct(ctx, store.ID.String(), manual, next.AddDate(0, 0, 1)); err != nil || card.TaskID == nil || *card.TaskID != tags || !card.Pending {
		t.Fatalf("%+v %v", card, err)
	}
}

// An open «Стопперы» task stays current on later days and blocks a new one until it is closed; its
// «Не соответствует» mark brings the standing «Печать ценников» task, shown until the day it is printed.
// Other services stay daily.
func TestOpenTasksCarryOver(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	if _, err := s.AddStore(ctx, &store); err != nil {
		t.Fatal(err)
	}
	product := uuid.New()
	if err := s.UpsertProducts(ctx, []models.Product{{ID: product, Code: "1", Name: "Чай", MarkingType: "none"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePrices(ctx, []models.Price{{Period: "2020-01-01T00:00:00", PriceType: store.PriceType, ProductID: product, DocID: uuid.New(), Price: "50"}}); err != nil {
		t.Fatal(err)
	}
	stopper, err := s.AddStopper(ctx, models.StopperInput{Name: "Тест", Color: "#1F5FBF"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddManualPosition(ctx, models.StopperManualInput{ProductID: product, StopperID: stopper, From: "2026-11-01"}); err != nil {
		t.Fatal(err)
	}
	var user, stopperType, cashType int64
	if err = s.db.QueryRowContext(ctx, "INSERT INTO users(name,user_token,password) VALUES('admin','admin','x') RETURNING id").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, "INSERT INTO task_types(name,service) VALUES('Инкассация','cash_collection') RETURNING id").Scan(&cashType); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT id FROM task_types WHERE service='stoppers'").Scan(&stopperType); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "INSERT INTO tasks(type_id,store_id,start,status) VALUES($1,$2,'2026-11-10 08:00','new')", cashType, store.ID); err != nil {
		t.Fatal(err)
	}
	yesterday, today := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC), time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	old := createdTask(t)(s.TaskCreateManual(ctx, stopperType, store.ID.String(), yesterday, user))
	if _, err = s.SetStopperTaskCheck(ctx, old, models.StopperTaskCheckInput{ProductID: product, Result: "bad"}, user); err != nil {
		t.Fatal(err)
	}
	// The mark time is set by the database clock; the test days are fixed.
	if _, err = s.db.ExecContext(ctx, "UPDATE stopper_task_checks SET checked_at=$1::timestamp AT TIME ZONE 'UTC' AT TIME ZONE current_setting('TimeZone')", "2026-11-10 09:30"); err != nil {
		t.Fatal(err)
	}
	tags := taskOf(t, currentTasks(t, s, store.ID, yesterday), models.TaskServicePricePrint)
	if tags == 0 {
		t.Fatal("a mark brings the price tag task")
	}
	if got := currentTasks(t, s, store.ID, today); len(got) != 2 || got[old] != "stoppers" || got[tags] != "price_print" {
		t.Fatal("yesterday's open tasks only:", got)
	}
	if _, err = s.TaskCreateManual(ctx, stopperType, store.ID.String(), today, user); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("new stoppers task while the old one is open:", err)
	}
	task, err := s.PriceTagTask(ctx, tags, today)
	if err != nil || len(task.Items) != 1 || task.Items[0].ProductID != product || task.Items[0].Source != models.PriceTagStopperBad {
		t.Fatal(task, err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE tasks SET status='completed' WHERE id=$1", old); err != nil {
		t.Fatal(err)
	}
	if got := currentTasks(t, s, store.ID, today); len(got) != 1 || got[tags] != "price_print" {
		t.Fatal("a closed task leaves the list:", got)
	}
	if _, err = s.SetPriceTagsPrinted(ctx, tags, []int64{task.Items[0].ID}, user, today); err != nil {
		t.Fatal(err)
	}
	if got := currentTasks(t, s, store.ID, today); len(got) != 1 || got[tags] != "price_print" {
		t.Fatal("printed today, still shown:", got)
	}
	if got := currentTasks(t, s, store.ID, today.AddDate(0, 0, 1)); len(got) != 0 {
		t.Fatal("nothing to print the next day:", got)
	}
	createdTask(t)(s.TaskCreateManual(ctx, stopperType, store.ID.String(), today, user))
}
