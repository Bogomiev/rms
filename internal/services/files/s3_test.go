package files

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestS3RetryAndMissingDelete(t *testing.T) {
	var puts int
	exists := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("request is not signed")
		}
		switch r.Method {
		case "HEAD":
			if !exists {
				w.WriteHeader(404)
			}
		case "PUT":
			puts++
			b, _ := io.ReadAll(r.Body)
			if string(b) != "hello" {
				t.Errorf("body %q", b)
			}
			exists = true
		case "GET":
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()
	o, e := NewS3(server.URL, "access", "secret", "bucket", "us-east-1")
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	for range 2 {
		if e = o.Ensure(ctx, "files/id/hash", []byte("hello")); e != nil {
			t.Fatal(e)
		}
	}
	if puts != 1 {
		t.Fatal("reuploaded existing object")
	}
	if e = o.Delete(ctx, "files/id/hash"); e != nil {
		t.Fatal(e)
	}
}
