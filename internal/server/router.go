package server

import "net/http"

func (s *Server) routes() {
	s.mux.Handle("/api/v1/display", s.requireAuth(http.HandlerFunc(s.handleDisplay)))
}
