package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"rms/internal/services/onec"
	"testing"
	"time"
)

func TestMarketplaceData(t *testing.T) {
	for _, tc := range []struct {
		body           string
		code, messages int
		fail           bool
	}{
		{`{"code":0,"mess":"","data":{"Eshop":{"extra":true}}}`, 0, 0, false},
		{`{"code":4,"mess":"Ошибка","data":{}}`, 4, 1, false},
		{`{"code":2,"mess":["a","b"],"data":{}}`, 2, 2, false},
		{`{"data":{}}`, 0, 0, true},
		{`{"code":0} {}`, 0, 0, true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/GetMarketplaces" {
					t.Errorf("path %s", r.URL.Path)
				}
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := onec.New(onec.Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := New(client).MarketplaceData(context.Background())
			if (err != nil) != tc.fail {
				t.Fatalf("error %v", err)
			}
			if !tc.fail && (got.ResultCode != tc.code || got.Messages == nil || len(got.Messages) != tc.messages) {
				t.Fatalf("response %+v", got)
			}
			if !tc.fail && tc.code == 0 && string(got.Data["Eshop"]) != `{"extra":true}` {
				t.Fatal("lost marketplace fields")
			}
		})
	}
}
