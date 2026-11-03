package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"rms/internal/domain/models"
	"rms/internal/services"
	"strings"
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

func TestScheduleOptionalTime(t *testing.T) {
	for _, field := range []string{``, `,"time":null`, `,"time":""`, `,"time":"00:00"`, `,"time":"09:30:15"`} {
		data := json.RawMessage(`{"task_type_id":1,"frequency":1,"period_from":"2026-11-02","period_to":"2026-11-08","days":[1,0,0,0,0,0,0]` + field + `}`)
		out, err := normalize("task_scheduling", data)
		if err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		if field == `` || field == `,"time":null` || field == `,"time":""` {
			if _, exists := got["time"]; exists {
				t.Fatalf("empty time must be omitted for SQL NULL: %s", out)
			}
		} else if got["time"] == nil {
			t.Fatalf("time lost: %s", out)
		}
	}
}

func TestTaskSVGIcons(t *testing.T) {
	for _, service := range []string{models.TaskServiceCashCollection, models.TaskServicePricePrint, models.TaskServiceWithoutService} {
		t.Run(service, func(t *testing.T) {
			for _, icon := range []string{
				`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M3 7h18" stroke="#fff"/></svg>`,
				"\xef\xbb\xbf" + `<?xml version="1.0"?><!-- icon --><svg xmlns="http://www.w3.org/2000/svg"/>`,
			} {
				in := models.TaskTypeInput{Name: "SVG", Service: service, Icon: []byte(icon), Color: "#ffffff", ColorDark: "#000000"}
				data, err := json.Marshal(in)
				if err != nil {
					t.Fatal(err)
				}
				out, err := normalize("task_types", data)
				if err != nil {
					t.Fatal(err)
				}
				var got models.TaskTypeInput
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got.Icon, in.Icon) {
					t.Fatal("SVG bytes changed")
				}
			}
		})
	}
}

func TestRejectInvalidSVGIcons(t *testing.T) {
	for _, icon := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg">`,
		`<html><svg xmlns="http://www.w3.org/2000/svg"/></html>`,
		`<svg xmlns="urn:other"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg"/><svg xmlns="http://www.w3.org/2000/svg"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg"/>garbage`,
		`<!DOCTYPE svg [<!ENTITY test "value">]><svg xmlns="http://www.w3.org/2000/svg"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat(" ", 1024*1024) + `</svg>`,
		`not an image`,
	} {
		in := models.TaskTypeInput{Name: "SVG", Service: models.TaskServiceCashCollection, Icon: []byte(icon), Color: "#ffffff", ColorDark: "#000000"}
		data, _ := json.Marshal(in)
		if _, err := normalize("task_types", data); !errors.Is(err, services.ErrInvalid) {
			t.Fatalf("invalid SVG accepted: %v", err)
		}
	}
}

func TestOptionalTaskColors(t *testing.T) {
	for _, fields := range []string{
		``,
		`,"color":"","color_dark":""`,
		`,"color":null,"color_dark":null`,
		`,"color":"#ffffff","color_dark":""`,
		`,"color":"","color_dark":"#000000"`,
	} {
		data := json.RawMessage(`{"name":"No background","service":"cash_collection"` + fields + `}`)
		out, err := normalize("task_types", data)
		if err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		var got models.TaskTypeInput
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		var want models.TaskTypeInput
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatal(err)
		}
		if got.Color != want.Color || got.ColorDark != want.ColorDark {
			t.Fatalf("colors changed: %s", out)
		}
	}
	for _, field := range []string{"color", "color_dark"} {
		for _, invalid := range []string{"red", "transparent", "#fff", " ", "#zzzzzz"} {
			data, _ := json.Marshal(map[string]any{"name": "Invalid", "service": "cash_collection", field: invalid})
			if _, err := normalize("task_types", data); !errors.Is(err, services.ErrInvalid) {
				t.Fatalf("invalid %s accepted: %s", field, invalid)
			}
		}
	}
}
