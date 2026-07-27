package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleTLSCertServesFingerprintAndPEM(t *testing.T) {
	tlsInfo := TLSInfo{
		Fingerprint: "AA:BB:CC",
		CertPEM:     "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n",
	}
	srv := newTestServerWithTLS(t, fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{}, tlsInfo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls-cert", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html prefix", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, tlsInfo.Fingerprint) {
		t.Errorf("body does not contain fingerprint %q", tlsInfo.Fingerprint)
	}
	if !strings.Contains(body, "BEGIN CERTIFICATE") {
		t.Errorf("body does not contain the PEM certificate block")
	}
}

func TestTLSCertRouteNotRegisteredWhenHTTPSDisabled(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls-cert", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
