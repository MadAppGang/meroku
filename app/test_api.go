package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIRoutes(t *testing.T) {
	// Create a test server with our router
	router := mainRouter()
	server := httptest.NewServer(router)
	defer server.Close()

	// Test /api/environments endpoint
	resp, err := http.Get(server.URL + "/api/environments")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	fmt.Printf("API test passed! Server is working correctly.\n")
}