package task

import (
	"encoding/json"
	"errors"
	"rms/internal/domain/models"
	"rms/internal/services"
	"testing"
)

func TestScheduleNormalization(t *testing.T) {
	in := models.TaskSchedulingInput{TaskTypeID: 1, Frequency: 2, PeriodFrom: "2026-11-04", PeriodTo: "2026-11-12", Days: []int16{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}, Time: "09:30", Stores: []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000001"}}
	data, _ := json.Marshal(in)
	out, e := normalize("task_scheduling", data)
	if e != nil {
		t.Fatal(e)
	}
	var got models.TaskSchedulingInput
	json.Unmarshal(out, &got)
	if got.PeriodFrom != "2026-11-02" || got.PeriodTo != "2026-11-15" || len(got.Stores) != 1 || got.Days[1] != 1 || got.Days[11] != 1 {
		t.Fatalf("%+v", got)
	}
	for _, change := range []func(*models.TaskSchedulingInput){func(v *models.TaskSchedulingInput) { v.Days = v.Days[:7] }, func(v *models.TaskSchedulingInput) { v.Days[0] = 2 }, func(v *models.TaskSchedulingInput) { v.Frequency = 11 }, func(v *models.TaskSchedulingInput) { v.Stores = []string{"invalid"} }, func(v *models.TaskSchedulingInput) { v.Time = "25:00" }, func(v *models.TaskSchedulingInput) { v.PeriodTo = "2026-10-01" }} {
		copy := in
		copy.Days = append([]int16{}, in.Days...)
		change(&copy)
		data, _ = json.Marshal(copy)
		if _, e = normalize("task_scheduling", data); !errors.Is(e, services.ErrInvalid) {
			t.Fatalf("accepted invalid schedule %s: %v", data, e)
		}
	}
}
func TestRejectInvalidTaskInput(t *testing.T) {
	for _, tc := range []struct{ k, data string }{
		{"tasks", `{"type_id":1,"store_id":"00000000-0000-0000-0000-000000000001","start":"2026-11-02T09:00:00","status":"unknown"}`},
		{"task_status_history", `{"period":"2026-11-02T09:00:00","task_id":1,"user_id":0,"status":"new"}`},
		{"task_types", `{"name":"Test","service":"invalid","color":"#ffffff","color_dark":"#000000"}`},
		{"task_types", `{"name":"Test","service":"without_service","color":"red","color_dark":"#000000"}`},
		{"task_types", `{"name":"Test","service":"without_service","color":"#ffffff","color_dark":"#000000","id":1}`},
	} {
		if _, e := normalize(tc.k, json.RawMessage(tc.data)); !errors.Is(e, services.ErrInvalid) {
			t.Fatalf("accepted %s: %v", tc.data, e)
		}
	}
}
