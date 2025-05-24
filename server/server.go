package server

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"errors"
)
 
// Middleware defines a function to wrap http.Handlers.
type Middleware func(http.Handler) http.Handler

// DefaultAddr returns the default network address the server listens on.
func DefaultAddr() string {
	return defaultAddr
}

const (
	// defaultReadTimeout is the maximum duration for reading the entire request, including the body.
	defaultReadTimeout = 5 * time.Second
	// defaultWriteTimeout is the maximum duration before timing out writes of the response.
	defaultWriteTimeout = 5 * time.Second
	// defaultAddr is the default network address to listen on.
	defaultAddr = ":8080"
	// defaultShutdownTimeout is the maximum time allowed for the server to complete outstanding requests on shutdown.
	defaultShutdownTimeout = 3 * time.Second
)

// Server wraps http.Server to provide Start, Shutdown, and Addr accessors.
// It listens on the configured address and supports graceful shutdown via context.
type Server struct {
	server          *http.Server
	errors          chan error
	shutdownTimeout time.Duration
	middlewares     []Middleware
}

// NewServer creates a Server with the given HTTP handler and applies any options.
// It does not begin listening until Start is called.
func WithMiddleware(m Middleware) Option {
	return func(s *Server) {
		s.middlewares = append(s.middlewares, m)
	}
}

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

	// Apply functional options to customize server behavior
	for _, opt := range opts {
		opt(s)
	}

	if len(s.middlewares) > 0 {
		h := s.server.Handler
		for _, m := range s.middlewares {
			h = m(h)
		}
		s.server.Handler = h
	}

	return s
}

// Start begins serving HTTP requests and listens for context cancellation to trigger shutdown.
// It blocks until the server stops, returning any error encountered.
func (s *Server) Start(ctx context.Context) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		errShutdown := s.Shutdown()
		errServe := <-errChan
		if errShutdown != nil {
			return fmt.Errorf("failed to shutdown server: %w", errShutdown)
		}
		if errors.Is(errServe, http.ErrServerClosed) {
			return nil
		}
		return errServe
	case errServe := <-errChan:
		if errors.Is(ctx.Err(), context.Canceled) && errors.Is(errServe, http.ErrServerClosed) {
		    return nil
		}
		return errServe
	}
}

// Shutdown gracefully shuts down the server without interrupting active connections.
// It uses a timeout to ensure the shutdown completes in a bounded time.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}
	return nil
}

// Addr returns the network address the server is listening on.
func (s *Server) Addr() string {
	return s.server.Addr
}
