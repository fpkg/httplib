package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fpkg/httplib/client"
	"github.com/stretchr/testify/assert"
)

// dummyResponse is a helper struct for JSON responses.
type dummyResponse struct {
	Message string `json:"message"`
}

func TestGetJSON_Success(t *testing.T) {
	testData := dummyResponse{Message: "hello"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testData)
	}))
	defer server.Close()

	cli := client.NewClient()
	var out dummyResponse
	err := cli.GetJSON(context.Background(), server.URL, &out)
	assert.NoError(t, err)
	assert.Equal(t, testData.Message, out.Message)
}

func TestGetJSON_RetryOnServerError(t *testing.T) {
	var calls int32
	// Fail first two, then succeed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dummyResponse{Message: "ok"})
	}))
	defer server.Close()

	cli := client.NewClient(
		client.WithRetries(3),
		// use zero backoff to speed up test
		client.WithBackoffStrategy(func(attempt int) time.Duration { return 0 }),
	)
	var out dummyResponse
	err := cli.GetJSON(context.Background(), server.URL, &out)
	assert.NoError(t, err)
	assert.Equal(t, "ok", out.Message)
	assert.Equal(t, int32(3), calls)
}

func TestGetJSON_ErrorResponse(t *testing.T) {
	// Return 400 with body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	cli := client.NewClient()
	var out dummyResponse
	err := cli.GetJSON(context.Background(), server.URL, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 400")
}

func TestGetJSON_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json"))
	}))
	defer server.Close()

	cli := client.NewClient()
	var out dummyResponse
	err := cli.GetJSON(context.Background(), server.URL, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode JSON")
}

func TestPostJSON_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		var req dummyResponse
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "payload", req.Message)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dummyResponse{Message: "posted"})
	}))
	defer server.Close()

	cli := client.NewClient()
	var out dummyResponse
	err := cli.PostJSON(context.Background(), server.URL, dummyResponse{Message: "payload"}, &out)
	assert.NoError(t, err)
	assert.Equal(t, "posted", out.Message)
}

func TestPostJSON_InvalidBody(t *testing.T) {
	cli := client.NewClient()
	// Create a channel, which json.Marshal cannot encode
	type C struct{ Ch chan int }
	var out dummyResponse
	err := cli.PostJSON(context.Background(), "http://example.com", C{Ch: make(chan int)}, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal request body")
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// sleep longer than context deadline
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set timeout shorter than handler
	d, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	cli := client.NewClient(
		client.WithTimeout(0), // disable http.Client timeout
		client.WithBackoffStrategy(func(attempt int) time.Duration { return 0 }),
	)
	var out dummyResponse
	err := cli.GetJSON(d, server.URL, &out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}
