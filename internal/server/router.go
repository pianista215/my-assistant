package server

import "net/http"

func (s *Server) routes() {
	s.mux.Handle("/api/v1/display", s.requireAuth(http.HandlerFunc(s.handleDisplay)))
	if s.tls.Fingerprint != "" {
		s.mux.HandleFunc("/api/v1/tls-cert", s.handleTLSCert)
	}
}
