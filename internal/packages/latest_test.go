package packages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubLatestReleaseURL points Latest at a test server and returns the undo.
func stubLatestReleaseURL(url string) func() {
	previous := latestReleaseAPIURL
	latestReleaseAPIURL = url
	return func() { latestReleaseAPIURL = previous }
}

func TestLatestReturnsTheNewestTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("no User-Agent header was sent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v8"}`))
	}))
	defer server.Close()
	defer stubLatestReleaseURL(server.URL)()

	version, err := Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != "v8" {
		t.Fatalf("got %q", version)
	}
}

func TestLatestReportsAFailedLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	defer stubLatestReleaseURL(server.URL)()

	if _, err := Latest(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}
