package httpapp

import (
	"context"
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"rms/internal/domain/models"
	"testing"
)

type publicDiscountService struct{}

func (publicDiscountService) Discounts(context.Context) ([]models.Discount, error) {
	return []models.Discount{}, nil
}

func TestDiscountsIsPublic(t *testing.T) {
	a := New(Config{}, Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Discounts: publicDiscountService{},
	})
	w := httptest.NewRecorder()
	a.hTTPServer.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/discounts", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="discounts.xml"` {
		t.Fatalf("unexpected Content-Disposition: %q", got)
	}
	var body models.Discounts
	if err := xml.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid discounts XML: %v", err)
	}
}
