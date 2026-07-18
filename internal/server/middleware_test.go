package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pianista215/my-assistant/internal/config"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(&config.Config{AuthToken: "correct-token", Port: "0"})
}

func TestRequireAuth(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic correct-token", http.StatusUnauthorized},
		{"wrong token", "Bearer wrong-token", http.StatusUnauthorized},
		{"correct token", "Bearer correct-token", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)

			called := false
			protected := srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/display", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()

			protected.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			wantCalled := tc.wantStatus == http.StatusOK
			if called != wantCalled {
				t.Fatalf("next handler called = %v, want %v", called, wantCalled)
			}
		})
	}
}
