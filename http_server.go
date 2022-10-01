package httplib

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultReadTimeout     = 5 * time.Second
	defaultWriteTimeout    = 5 * time.Second
	defaultAddr            = ":8080"
	defaultShutdownTimeout = 3 * time.Second
)

// Server implements http server.
type Server struct {
	server          *http.Server
	errors          chan error
	shutdownTimeout time.Duration
}

// NewServer creates and starts a new server.
func NewServer(handler http.Handler, opts ...Option) *Server {
	httpServer := &http.Server{
		Handler:      handler,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		Addr:         defaultAddr,
	}

	s := &Server{
		server:          httpServer,
		errors:          make(chan error, 1),
		shutdownTimeout: defaultShutdownTimeout,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		if err := s.Shutdown(); err != nil {
			s.errors <- err
		}
	}()

	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			s.errors <- err
		}
	}()

	return <-s.errors
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}
