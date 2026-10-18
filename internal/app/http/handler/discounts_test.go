package handler

import (
	"context"
	"encoding/xml"
	"net/http/httptest"
	"rms/internal/domain/models"
	"strings"
	"testing"
)

type discountStub struct{ items []models.Discount }

func (s discountStub) Discounts(context.Context) ([]models.Discount, error) { return s.items, nil }
func TestDiscountXMLDownload(t *testing.T) {
	for _, items := range [][]models.Discount{nil, {{StoreID: "store", ProductID: "product", Price: "149.00", Start: "2026-10-01 00:00:00", DocID: "doc", WriteOffBonus: true}}} {
		h := NewHandler(nil, nil)
		h.ConfigureDiscounts(discountStub{items: items})
		w := httptest.NewRecorder()
		h.Discounts(w, httptest.NewRequest("GET", "/discounts", nil))
		if w.Code != 200 || !strings.HasPrefix(w.Body.String(), xml.Header) || w.Header().Get("Content-Disposition") != `attachment; filename="discounts.xml"` {
			t.Fatalf("%+v %s", w.Result(), w.Body.String())
		}
		var got models.Discounts
		if err := xml.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Items) != len(items) {
			t.Fatalf("%+v", got)
		}
		if len(items) > 0 && (got.Items[0] != items[0] || !strings.Contains(w.Body.String(), `apply_other_discounts="false"`)) {
			t.Fatal(w.Body.String())
		}
	}
}
