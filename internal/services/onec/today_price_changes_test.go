package onec

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetTodayPriceChanges(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		count   int
		wantErr bool
	}{
		{
			name:  "complete",
			body:  `{"page":1,"perPage":1,"totalPages":1,"totalItems":1,"items":[{"store_id":"639fa273-c326-11e9-80c2-0cc47ae051d5","product_id":"bece3d84-69c4-11ee-bbf5-00505601d66f","price":690.123456789,"is_promo":true}]}`,
			count: 1,
		},
		{name: "empty", body: `{"page":1,"perPage":0,"totalPages":0,"totalItems":0,"items":[]}`},
		{name: "incomplete", body: `{"page":1,"perPage":1,"totalPages":2,"totalItems":2,"items":[]}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.RequestURI() != "/v1/GetTodayPriceChanges" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			svc, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			items, err := svc.GetTodayPriceChanges(context.Background())
			if (err != nil) != tc.wantErr || len(items) != tc.count {
				t.Fatalf("items = %+v, error = %v", items, err)
			}
			if tc.count > 0 && (items[0].Price != "690.123456789" || !items[0].IsPromo) {
				t.Fatalf("unexpected price change %+v", items[0])
			}
		})
	}
}
