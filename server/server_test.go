package server_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"


	"github.com/fpkg/httplib/server" // Using the import from your example tests
)

// Helper function to get an available port
func getAvailablePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "Failed to find an available port")
	defer listener.Close()
	return fmt.Sprintf(":%d", listener.Addr().(*net.TCPAddr).Port)
}

// Your existing tests (slightly formatted for clarity and consistency)

func TestAddrDefault(t *testing.T) {
	s := server.NewServer(http.NewServeMux())
	assert.Equal(t, ":8080", s.Addr(), "should return the default address")
}

func TestWithAddrOption(t *testing.T) {

	ephemeralAddr := getAvailablePort(t)
	s := server.NewServer(http.NewServeMux(), server.WithAddr(ephemeralAddr))
	assert.Equal(t, ephemeralAddr, s.Addr(), "should return the custom address")
}

func TestShutdownBeforeStart(t *testing.T) {
	s := server.NewServer(http.NewServeMux())
	err := s.Shutdown()

	assert.NoError(t, err, "Shutdown should succeed even if Start was not called")
}

func TestStartImmediateCancel(t *testing.T) {
	// Use a simple handler that would block if called
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when context is canceled immediately")
		w.WriteHeader(http.StatusInternalServerError) // Should not reach here
	})
	ephemeralAddr := getAvailablePort(t)
	s := server.NewServer(h, server.WithAddr(ephemeralAddr))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately


	err := s.Start(ctx)
	// The error from Start when context is immediately canceled can be nil
	// because s.Shutdown() is called, and s.server.Shutdown() on a non-running
	// server might return nil or http.ErrServerClosed. Your wrapper returns nil.
	assert.NoError(t, err, "Start should return nil when context is canceled before serving any request")
}

func TestStartHandlesRequestsAndShutdown(t *testing.T) {
	called := false
	var mu sync.Mutex // Protect `called` if requests were concurrent, though not strictly needed here.

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		called = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	ephemeralAddr := getAvailablePort(t)
	s := server.NewServer(h, server.WithAddr(ephemeralAddr))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cancellation happens eventually

	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- s.Start(ctx)
	}()


	time.Sleep(100 * time.Millisecond)


	resp, err := http.Get("http://" + s.Addr() + "/test") // Use actual address
	require.NoError(t, err, "HTTP GET request failed")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status OK")


	cancel()


	select {
	case err := <-serverErrChan:
		// http.ErrServerClosed is expected on graceful ListenAndServe shutdown,
		// but our Start method should return nil if s.Shutdown() was successful.
		assert.NoError(t, err, "Start should return nil on graceful shutdown")
	case <-time.After(2 * time.Second): // Increased timeout
		t.Fatal("Server did not shutdown in time")
	}

	mu.Lock()
	assert.True(t, called, "handler should have been called at least once")
	mu.Unlock()
}

// New Tests

func TestNewServer_WithOptions(t *testing.T) {
	mux := http.NewServeMux()
	customAddr := getAvailablePort(t)
	customReadTimeout := 10 * time.Second
	customWriteTimeout := 11 * time.Second
	customShutdownTimeout := 7 * time.Second


	sWithOptions := server.NewServer(mux,
		server.WithAddr(customAddr),
		server.WithReadTimeout(customReadTimeout),
		server.WithWriteTimeout(customWriteTimeout),
		server.WithShutdownTimeout(customShutdownTimeout),
	)

	assert.Equal(t, customAddr, sWithOptions.Addr(), "Custom address not set with WithAddr option")
	// Note: Testing the actual timeout values (ReadTimeout, WriteTimeout) on the internal http.Server
	// would require either exposing s.server (not ideal for encapsulation)
	// or more complex behavioral tests (e.g., a client that times out reading/writing).
	// The WithShutdownTimeout option's effect is behaviorally tested in TestServer_ShutdownTimeoutExceeded
	// and TestServer_GracefulShutdownWithLongRequest.


	sDefaults := server.NewServer(mux)
	assert.Equal(t, server.DefaultAddr(), sDefaults.Addr(), "Default address not set when no options provided")
	// To properly test defaultReadTimeout, defaultWriteTimeout, and defaultShutdownTimeout behaviorally
	// without options would involve:
	// 1. For defaultShutdownTimeout: A test similar to TestServer_ShutdownTimeoutExceeded,
	//    but without using WithShutdownTimeout, and expecting shutdown around the default value.
	// 2. For defaultRead/WriteTimeouts: Tests with clients that send/receive data slowly,
	//    expecting the server to close connections after the default timeout.
	// These behavioral tests for defaults can be complex to write reliably.
	// The options mechanism itself (that they *can* change these values) is tested.
}

func TestStart_ListenAndServeError(t *testing.T) {

	fixedAddr := getAvailablePort(t)
	listener, err := net.Listen("tcp", fixedAddr)
	require.NoError(t, err, "Could not listen on fixedAddr to block it")
	defer listener.Close()


	s := server.NewServer(http.NewServeMux(), server.WithAddr(fixedAddr))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startErr := s.Start(ctx)
	require.Error(t, startErr, "Start should return an error if ListenAndServe fails")
	// The error should not be http.ErrServerClosed, but a "bind: address already in use" type error.
	assert.NotEqual(t, http.ErrServerClosed, startErr, "Error should not be ErrServerClosed for a bind failure")
	// Check for a common part of bind errors
	var opError *net.OpError
	if assert.ErrorAs(t, startErr, &opError) {
		assert.Equal(t, "listen", opError.Op)
	}
}

func TestServer_GracefulShutdownWithLongRequest(t *testing.T) {
	handlerDone := make(chan struct{})
	requestDuration := 200 * time.Millisecond
	shutdownTimeout := 500 * time.Millisecond // Ensure this is longer than requestDuration

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(requestDuration) // Simulate long processing
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("done"))
		close(handlerDone)
	})

	ephemeralAddr := getAvailablePort(t)
	s := server.NewServer(h,
		server.WithAddr(ephemeralAddr),
		server.WithShutdownTimeout(shutdownTimeout),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- s.Start(ctx)
	}()


	time.Sleep(50 * time.Millisecond)
	require.NotEmpty(t, s.Addr(), "Server address should be set")

	clientErrChan := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + s.Addr() + "/long")
		if err != nil {
			clientErrChan <- fmt.Errorf("client GET error: %w", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			clientErrChan <- fmt.Errorf("client unexpected status: %d", resp.StatusCode)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "done" {
			clientErrChan <- fmt.Errorf("client unexpected body: %s", string(body))
			return
		}
		clientErrChan <- nil
	}()

	// Give the request a moment to be "in-flight"
	time.Sleep(requestDuration / 4)
	cancel() // Trigger shutdown

	// Check if handler completed
	select {
	case <-handlerDone:
		// Expected: handler finished
	case <-time.After(shutdownTimeout + requestDuration): // Generous timeout
		t.Fatal("Handler did not complete during graceful shutdown")
	}

	// Check if client completed
	select {
	case err := <-clientErrChan:
		assert.NoError(t, err, "Client request should complete successfully")
	case <-time.After(shutdownTimeout + requestDuration): // Generous timeout
		t.Fatal("Client did not complete its request")
	}

	// Check server shutdown error
	select {
	case err := <-serverErrChan:
		assert.NoError(t, err, "Server Start should return nil on graceful shutdown")
	case <-time.After(shutdownTimeout + 50*time.Millisecond):
		t.Fatal("Server did not shutdown gracefully in time")
	}
}

func TestServer_ShutdownTimeoutExceeded(t *testing.T) {
	handlerCanFinish := make(chan struct{})
	handlerStarted := make(chan struct{})
	requestHangDuration := 200 * time.Millisecond
	shutdownTimeout := 50 * time.Millisecond // Shorter than requestHangDuration

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		select {
		case <-time.After(requestHangDuration): // Simulate very long processing
			w.WriteHeader(http.StatusOK)
		case <-handlerCanFinish: // Allow test to unblock if needed
			w.WriteHeader(http.StatusServiceUnavailable) // Indicate it was cut short by test
		}
	})
	defer close(handlerCanFinish) // Ensure goroutine doesn't leak if test fails early

	ephemeralAddr := getAvailablePort(t)
	s := server.NewServer(h,
		server.WithAddr(ephemeralAddr),
		server.WithShutdownTimeout(shutdownTimeout),
	)


	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Main context for server Start

	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- s.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond) // Wait for server to likely be up
	require.NotEmpty(t, s.Addr(), "Server address should be set")


	go func() {
		// We don't care about the response, just that it's active
		// and causes the handler to hang.
		http.Get("http://" + s.Addr() + "/hang")
	}()

	// Wait for the handler to actually start processing the request
	select {
	case <-handlerStarted:
		// Handler is active, proceed to shutdown
	case <-time.After(1 * time.Second):
		t.Fatal("Handler did not start in time")
	}


	cancel()

	// Check server shutdown error
	// The error from s.Start() should reflect the shutdown failure due to timeout.
	select {
	case err := <-serverErrChan:
		require.Error(t, err, "Server Start should return an error when shutdown times out")
		assert.Contains(t, err.Error(), "failed to shutdown server", "Error message should indicate shutdown failure")
		// The underlying error from http.Server.Shutdown(ctx) when its context times out is context.DeadlineExceeded.
		assert.ErrorIs(t, err, context.DeadlineExceeded, "Underlying error should be context.DeadlineExceeded")
	case <-time.After(shutdownTimeout + requestHangDuration + 200*time.Millisecond): // Generous timeout
		t.Fatal("Server did not return an error or shutdown in time after timeout exceeded")
	}
}

func TestServer_StartAndManualShutdown(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ephemeralAddr := getAvailablePort(t)
	s := server.NewServer(h, server.WithAddr(ephemeralAddr))

	// Using a context that won't be canceled for this test
	// to ensure shutdown is triggered by direct s.Shutdown() call.
	ctx := context.Background()

	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- s.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond) // Wait for server to start
	require.NotEmpty(t, s.Addr(), "Server address should be set")


	resp, err := http.Get("http://" + s.Addr())
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)


	shutdownErr := s.Shutdown()
	assert.NoError(t, shutdownErr, "Manual Shutdown() call failed")

	select {
	case startErr := <-serverErrChan:
		// When server is shut down (e.g. by manual s.Shutdown()), ListenAndServe returns http.ErrServerClosed.
		// Your Start method forwards this error.
		// This is a valid behavior. If you want Start to return nil in this case,
		// you'd need to check for http.ErrServerClosed before sending to s.errors
		// or when reading from s.errors.
		// Current logic: ListenAndServe error goes to s.errors, Start returns it.
		assert.Equal(t, http.ErrServerClosed, startErr, "Start should return http.ErrServerClosed after manual shutdown")
	case <-time.After(2 * time.Second):
		t.Fatal("Server Start did not return after manual shutdown")
	}
}
