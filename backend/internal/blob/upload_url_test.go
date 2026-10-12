package blob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The server signs through the address it can reach. The URL it hands the
// browser must use the address the browser can reach.
func TestSignedUploadURLUsesTheBrowsersAddress(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/object/upload/sign/logs/a/b.log" {
			t.Errorf("signed through %s", r.URL.Path)
		}
		w.Write([]byte(`{"url": "/object/upload/sign/logs/a/b.log?token=t"}`))
	}))
	defer storage.Close()
	const signed = "/storage/v1/object/upload/sign/logs/a/b.log?token=t"

	same := NewSupabase(storage.URL, "key", "logs")
	if got, _, err := same.SignedUploadURL(context.Background(), "a/b.log"); err != nil || got != storage.URL+signed {
		t.Errorf("with one address: %q, %v", got, err)
	}

	split := NewSupabase(storage.URL, "key", "logs").ReachedFromBrowserAt("http://localhost:54321/")
	got, _, err := split.SignedUploadURL(context.Background(), "a/b.log")
	if err != nil || got != "http://localhost:54321"+signed {
		t.Errorf("with a browser address: %q, %v", got, err)
	}
	if strings.Contains(got, storage.URL) {
		t.Errorf("the browser was handed the server's address: %s", got)
	}
}
