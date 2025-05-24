package server

import (
	"time"
)

// Option is a server option.
type Option func(*Server)

// WithAddr set address for server.
func WithAddr(addr string) Option {
	return func(s *Server) {
		s.server.Addr = addr
	}
}

// WithReadTimeout set read timeout for server.
func WithReadTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.server.ReadTimeout = timeout
	}
}

// WithWriteTimeout set write timeout for server.
func WithWriteTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.server.WriteTimeout = timeout
	}
}

// WithShutdownTimeout set shutdown timeout for server.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.shutdownTimeout = timeout
	}
}
