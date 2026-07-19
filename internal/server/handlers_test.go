package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pianista215/my-assistant/internal/display"
)

func TestHandleDisplayRequiresToken(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDisplayReturnsEncodedImage(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display", nil)
	req.Header.Set("Authorization", "Bearer correct-token")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", ct)
	}

	img, err := display.Decode(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if img.Width != display.Width || img.Height != display.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", img.Width, img.Height, display.Width, display.Height)
	}
}
