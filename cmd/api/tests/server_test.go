package tests

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/kartik7120/booking_broker-service/cmd/api"
)

func TestServer(t *testing.T) {
	t.Run("Test if server is pingable", func(t *testing.T) {
		// Testing if /ping route is pingable and returns a 200 status code

		app := api.Config{}

		l, err := net.Listen("tcp", ":8080")
		if err != nil {
			t.Fatalf("Error initializing listener: %v", err)
		}

		srv := &http.Server{
			Handler: app.Routes(),
		}

		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.Background())
		wg.Add(1)

		go func() {
			defer wg.Done()
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				t.Errorf("Error starting server: %v", err)
			}
		}()

		// Give the server a moment to start
		// time.Sleep(1 * time.Second)

		// Perform any necessary tests here
		// Example: make an HTTP request to the server
		resp, err := http.Get("http://localhost:8080/ping")
		if err != nil {
			t.Fatalf("Error making request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %v", resp.StatusCode)
		}

		// Cancel the context to shut down the server
		cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Fatalf("Error shutting down server: %v", err)
		}

		// Wait for the server goroutine to finish
		wg.Wait()
	})
}
