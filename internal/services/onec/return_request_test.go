package onec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
)

type returnTransport func(*http.Request) (*http.Response, error)

func (f returnTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendReturn(t *testing.T) {
	id, seller, store, product := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	date := time.Date(2026, time.November, 26, 14, 30, 0, 0, time.FixedZone("UTC+10", 10*60*60))
	for _, statuses := range []struct {
		current models.ReturnStatus
		sent    models.ReturnStatus
	}{
		{models.ReturnNew, models.ReturnNew},
		{models.ReturnProcessed, models.ReturnProcessed},
	} {
		t.Run(string(statuses.current), func(t *testing.T) {
			request := models.ReturnRequest{ID: id, Date: date, Seller: seller, StoreID: store, Status: statuses.current,
				Items: []models.ReturnItem{{ProductID: product, Amount: 2, Marks: []string{"mark1", "mark2"}}}}
			oneC := &Service{baseURL: "http://onec.test/", client: &http.Client{Transport: returnTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/ReturnRequest" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var body struct {
					ID     uuid.UUID           `json:"returnId"`
					Date   string              `json:"date"`
					Seller uuid.UUID           `json:"seller"`
					Store  uuid.UUID           `json:"storeId"`
					Status models.ReturnStatus `json:"status"`
					Items  []returnLine        `json:"items"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.ID != id || body.Date != "2026-11-26T14:30:00+10:00" || body.Seller != seller || body.Store != store || body.Status != statuses.sent ||
					len(body.Items) != 2 || body.Items[0] != (returnLine{product, 1, "mark1"}) || body.Items[1] != (returnLine{product, 1, "mark2"}) {
					t.Fatalf("unexpected payload: %+v", body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"returnNumber":"006","status":"SENT"}}`))}, nil
			})}}
			result, err := oneC.SendReturn(context.Background(), request, false)
			if err != nil || result.Status != models.ReturnSent || result.Number == nil || *result.Number != "006" {
				t.Fatalf("result %+v, error %v", result, err)
			}
		})
	}
}

func TestSendReturnStatusPreservesOneCError(t *testing.T) {
	id := uuid.New()
	for _, statusCode := range []int{200, 400} {
		oneC := &Service{baseURL: "http://onec.test/", client: &http.Client{Transport: returnTransport(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/ReturnRequestStatus" {
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 2 || body["returnId"] != id.String() || body["status"] != "CANCELLED" {
				t.Fatalf("unexpected payload: %+v", body)
			}
			return &http.Response{StatusCode: statusCode, Body: io.NopCloser(strings.NewReader(`{"code":0,"mess":"Заявку нельзя отменить"}`))}, nil
		})}}
		_, err := oneC.SendReturn(context.Background(), models.ReturnRequest{ID: id, Status: models.ReturnCancelled}, true)
		if err == nil || err.Error() != "Заявку нельзя отменить" {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestGetReturnRequestStatus(t *testing.T) {
	id := uuid.New()
	for _, tt := range []struct {
		name       string
		httpStatus int
		body       string
		want       models.ReturnStatus
		message    string
	}{
		{"new", 200, `{"code":0,"data":{"returnNumber":"006","status":"NEW"}}`, models.ReturnProcessed, ""},
		{"processed", 200, `{"code":0,"data":{"status":"PROCESSED"}}`, models.ReturnProcessed, ""},
		{"completed", 200, `{"code":0,"data":{"status":"COMPLETED"}}`, models.ReturnCompleted, ""},
		{"cancelled", 200, `{"code":0,"data":{"status":"CANCELLED"}}`, models.ReturnCancelled, ""},
		{"canceled from 1C", 200, `{"code":0,"data":{"status":"CANCELED"}}`, models.ReturnCancelled, ""},
		{"http error", 501, `{"mess":"Заявка не найдена"}`, "", "Заявка не найдена"},
		{"business error", 200, `{"code":1,"mess":"Заявка не найдена"}`, "", "Заявка не найдена"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oneC := &Service{baseURL: "http://onec.test/", client: &http.Client{Transport: returnTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/GetReturnRequestStatus/" || r.URL.Query().Get("returnId") != id.String() {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				return &http.Response{StatusCode: tt.httpStatus, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}}
			got, err := oneC.GetReturnRequestStatus(context.Background(), id)
			if tt.message != "" {
				if err == nil || err.Error() != tt.message {
					t.Fatalf("error = %v, want %q", err, tt.message)
				}
			} else if err != nil || got != tt.want {
				t.Fatalf("status = %q, error = %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestReturnRequestsStatuses(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	oneC := &Service{baseURL: "http://onec.test/", client: &http.Client{Transport: returnTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/ReturnRequestsStatuses" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		}
		var payload struct {
			IDs []uuid.UUID `json:"returnIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.IDs) != 2 || payload.IDs[0] != first || payload.IDs[1] != second {
			t.Fatalf("unexpected payload %+v, error %v", payload, err)
		}
		body := `{"code":0,"data":[{"returnId":"` + first.String() + `","status":"COMPLETED"},{"returnId":"` + second.String() + `","status":"CANCELED"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	got, err := oneC.ReturnRequestsStatuses(context.Background(), []uuid.UUID{first, second})
	if err != nil || len(got) != 2 || got[first] != models.ReturnCompleted || got[second] != models.ReturnCancelled {
		t.Fatalf("unexpected statuses %v, error %v", got, err)
	}
}
