package onec

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"rms/internal/domain/models"
)

type fakeProducts struct {
	batches [][]models.Product
	failAt  int
}

func (f *fakeProducts) UpsertProducts(_ context.Context, v []models.Product) error {
	f.batches = append(f.batches, append([]models.Product(nil), v...))
	if len(f.batches) == f.failAt {
		return errors.New("database failure")
	}
	return nil
}

func TestSyncGoodsBatches(t *testing.T) {
	for _, count := range []int{0, 1, 250, 251, 501} {
		goods := make([]models.Product, count)
		for i := range goods {
			goods[i] = models.Product{ID: uuid.New(), Code: "ЦБ-00003573", Name: "Кольца кальмара", MarkingType: "БезОсобенностейУчета", IsWeight: true, IsThermalMode: true, Barcodes: []models.BarcodeInfo{{Barcode: "4602009936098", Unit: "шт", Ratio: 6}}}
		}
		body, _ := json.Marshal(goods)
		for _, failAt := range []int{0, 2} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/GetGoods" || r.URL.RawQuery != "" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.Write(body)
			}))
			f := &fakeProducts{failAt: failAt}
			svc, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, f)
			if err != nil {
				t.Fatal(err)
			}
			err = svc.SyncGoods(context.Background())
			expected := (count + 249) / 250
			wantErr := failAt > 0 && expected >= failAt
			if wantErr {
				expected = failAt
			}
			if (err != nil) != wantErr || len(f.batches) != expected {
				t.Fatalf("count=%d err=%v batches=%d", count, err, len(f.batches))
			}
			for i, batch := range f.batches {
				if !reflect.DeepEqual(batch, goods[i*250:min((i+1)*250, count)]) {
					t.Fatal("batch contents differ")
				}
			}
			server.Close()
		}
	}
}

func TestSyncGoodsInvalidResponse(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[`, `[] {}`, `[{}]`, `[{"uid":"invalid"}]`, `[{"uid":"cc3dc5bd-6120-11ef-8da0-00155d1a6906"},{"uid":"cc3dc5bd-6120-11ef-8da0-00155d1a6906"}]`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			f := &fakeProducts{}
			svc, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, f)
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.SyncGoods(context.Background()); err == nil || len(f.batches) != 0 {
				t.Fatalf("err=%v batches=%d", err, len(f.batches))
			}
		})
	}
}
