package onec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"rms/internal/domain/models"
	"rms/internal/storage"
)

type fakeStores struct {
	added, updated int
	failure        error
}

func (f *fakeStores) AddStore(_ context.Context, v *models.Store) (*models.Store, error) {
	f.added++
	return v, nil
}
func (f *fakeStores) UpdateStore(_ context.Context, v *models.Store) (*models.Store, error) {
	f.updated++
	return v, f.failure
}

func TestSyncStores(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		status        int
		failure       error
		wantErr       bool
		updates, adds int
	}{
		{"new", `{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[{"id":"11111111-1111-1111-1111-111111111111","code":"01","name":"Store","address":"Street"}]}`, 200, storage.ErrNotFound, false, 1, 1},
		{"existing", `{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[{"id":"11111111-1111-1111-1111-111111111111"}]}`, 200, nil, false, 1, 0},
		{"db failure", `{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[{"id":"11111111-1111-1111-1111-111111111111"}]}`, 200, errors.New("db failed"), true, 1, 0},
		{"empty", `{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[]}`, 200, nil, false, 0, 0},
		{"missing items", `{}`, 200, nil, true, 0, 0},
		{"null items", `{"items":null}`, 200, nil, true, 0, 0},
		{"bare array", `[]`, 200, nil, true, 0, 0},
		{"null", `null`, 200, nil, true, 0, 0},
		{"invalid batch", `{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[{"id":"11111111-1111-1111-1111-111111111111"},{}]}`, 200, nil, true, 0, 0},
		{"malformed", `[`, 200, nil, true, 0, 0},
		{"trailing", `{"items":[]} {}`, 200, nil, true, 0, 0},
		{"http error", `{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[]}`, 500, nil, true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/eshop/hs/PAPI/v1/GetStores" || r.URL.RawQuery != "" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			stores := &fakeStores{failure: tc.failure}
			svc, err := New(Config{BaseURL: server.URL + "/eshop/hs/PAPI/", Timeout: time.Second}, stores, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = svc.SyncStores(context.Background())
			if (err != nil) != tc.wantErr || stores.updated != tc.updates || stores.added != tc.adds {
				t.Fatalf("err=%v updates=%d adds=%d", err, stores.updated, stores.added)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	svc, err := New(Config{BaseURL: "http://localhost:1/", Timeout: time.Second}, &fakeStores{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.SyncStores(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestGetStoresResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"page":1,"perPage":2,"totalPages":1,"totalItems":2,"items":[
   {"id":"5a6bb748-bfd1-11e8-80c8-0cc47ab32259","uid_1c":"5a6bb748-bfd1-11e8-80c8-0cc47ab32259","code":"8031","name":"8031_Королев Горького (ИП Ефремян РБ)","address":"141080, Московская область, г.о. Королёв, г Королёв, д. 6А","pin":"8031"},
   {"id":"02b3665b-f732-11e9-80ce-0cc47ae051d5","uid_1c":"02b3665b-f732-11e9-80ce-0cc47ae051d5","code":"8071","name":"8071_Руднева (ИП Ситникова)","address":"117041, Город Москва, ул Адмирала Руднева, д. 2","pin":"8071"}
  ]}`))
	}))
	defer server.Close()
	svc, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, &fakeStores{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := svc.GetStores(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 2 {
		t.Fatalf("got %d stores", len(stores))
	}
	if stores[0].ID.String() != "5a6bb748-bfd1-11e8-80c8-0cc47ab32259" || stores[0].Code != "8031" || stores[0].Name != "8031_Королев Горького (ИП Ефремян РБ)" || stores[0].Address != "141080, Московская область, г.о. Королёв, г Королёв, д. 6А" || stores[1].Code != "8071" {
		t.Fatalf("unexpected stores: %+v", stores)
	}
}
