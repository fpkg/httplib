package httplib

import (
	"time"
)

// Option is a server option.
type Option func(*Server)

// BindAddr set address for server.
func BindAddr(addr string) Option {
	return func(s *Server) {
		s.server.Addr = addr
	}
}

// ReadTimeout set read timeout for server.
func ReadTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.server.ReadTimeout = timeout
	}
}

// WriteTimeout set write timeout for server.
func WriteTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.server.WriteTimeout = timeout
	}
}

// ShutdownTimeout set shutdown timeout for server.
func ShutdownTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.shutdownTimeout = timeout
	}
}
