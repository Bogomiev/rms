package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/token"
	"strings"
	"testing"
)

type stopperStub struct {
	StopperService
	filter models.StopperPositionFilter
	assign models.StopperAssignInput
	saved  int64
	err    error
}

func (s *stopperStub) Positions(_ context.Context, f models.StopperPositionFilter) ([]models.StopperPosition, error) {
	s.filter = f
	return []models.StopperPosition{}, s.err
}
func (s *stopperStub) SetPositionsStopper(_ context.Context, in models.StopperAssignInput) error {
	s.assign = in
	return s.err
}
func (s *stopperStub) SaveStopper(_ context.Context, id int64, _ models.StopperInput) error {
	s.saved = id
	return s.err
}

func TestStopperPositionsQuery(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	stub := &stopperStub{}
	h := NewHandler(nil, nil)
	h.ConfigureStoppers(stub)
	w := httptest.NewRecorder()
	h.StopperPositions(w, httptest.NewRequest("GET", "/stoppers/positions?from=2026-11-02&to=2026-11-16&store_id="+first.String()+"&store_id="+second.String(), nil))
	if w.Code != 200 || stub.filter.From != "2026-11-02" || stub.filter.To != "2026-11-16" || len(stub.filter.Stores) != 2 || stub.filter.Stores[0] != first || stub.filter.Stores[1] != second {
		t.Fatalf("%d %s %+v", w.Code, w.Body.String(), stub.filter)
	}
	w = httptest.NewRecorder()
	h.StopperPositions(w, httptest.NewRequest("GET", "/stoppers/positions?from=2026-11-02&to=2026-11-16&store_id=bad", nil))
	if w.Code != 400 {
		t.Fatalf("bad store: %d", w.Code)
	}
}
func TestSetPositionsStopperBody(t *testing.T) {
	doc, product := uuid.New(), uuid.New()
	for _, tc := range []struct {
		body   string
		err    error
		status int
	}{
		{`{"positions":[{"doc_id":"` + doc.String() + `","product_id":"` + product.String() + `"}],"stopper_id":3}`, nil, 200},
		{`{"positions":[{"doc_id":null,"product_id":"` + product.String() + `"}],"stopper_id":null}`, nil, 200},
		{`{"positions":[],"stopper":3}`, nil, 400},
		{`{"positions":[{"doc_id":null,"product_id":"` + product.String() + `"}],"stopper_id":3}`, services.ErrNotFound, 404},
	} {
		stub := &stopperStub{err: tc.err}
		h := NewHandler(nil, nil)
		h.ConfigureStoppers(stub)
		w := httptest.NewRecorder()
		h.SetPositionsStopper(w, httptest.NewRequest("PUT", "/stoppers/positions/stopper", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	stub := &stopperStub{}
	h := NewHandler(nil, nil)
	h.ConfigureStoppers(stub)
	h.SetPositionsStopper(httptest.NewRecorder(), httptest.NewRequest("PUT", "/stoppers/positions/stopper", strings.NewReader(`{"positions":[{"doc_id":"`+doc.String()+`","product_id":"`+product.String()+`"}],"stopper_id":3}`)))
	if len(stub.assign.Positions) != 1 || *stub.assign.Positions[0].DocID != doc || stub.assign.Positions[0].ProductID != product || *stub.assign.StopperID != 3 {
		t.Fatalf("%+v", stub.assign)
	}
}
func TestSaveStopperRoute(t *testing.T) {
	stub := &stopperStub{err: services.ErrConflict}
	h := NewHandler(nil, nil)
	h.ConfigureStoppers(stub)
	router := chi.NewRouter()
	router.Put("/stoppers/{id}", h.SaveStopper)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("PUT", "/stoppers/5", strings.NewReader(`{"name":"ХИТ","color":"#D7261E"}`)))
	if w.Code != 409 || stub.saved != 5 {
		t.Fatalf("%d %s %d", w.Code, w.Body.String(), stub.saved)
	}
}

type managerStub struct {
	StopperService
	allowed bool
}

func (s *managerStub) CanManage(context.Context, int64) (bool, error) { return s.allowed, nil }
func TestStopperManagerOnly(t *testing.T) {
	for _, tc := range []struct {
		allowed bool
		claims  *token.UserClaims
		status  int
	}{{true, &token.UserClaims{UserID: 5}, 200}, {false, &token.UserClaims{UserID: 5}, 403}, {true, nil, 401}} {
		h := NewHandler(nil, nil)
		h.ConfigureStoppers(&managerStub{allowed: tc.allowed})
		r := httptest.NewRequest("GET", "/stoppers/", nil)
		if tc.claims != nil {
			r = r.WithContext(context.WithValue(r.Context(), authKey{}, tc.claims))
		}
		w := httptest.NewRecorder()
		h.StopperManagerOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
}
