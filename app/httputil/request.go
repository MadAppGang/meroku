package httputil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DecodeJSON decodes JSON request body into the target struct
func DecodeJSON(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}
	return nil
}

// ReadBody reads the entire request body and returns it as bytes
func ReadBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return body, nil
}

// QueryParam gets a query parameter with a default value if not found
func QueryParam(r *http.Request, key string, defaultValue string) string {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// RequiredQueryParam gets a required query parameter or returns an error via HTTP response
// Returns the value and true if found, empty string and false if not found (and writes error response)
func RequiredQueryParam(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	value := r.URL.Query().Get(key)
	if value == "" {
		RespondError(w, http.StatusBadRequest, fmt.Sprintf("%s parameter is required", key))
		return "", false
	}
	return value, true
}

// OptionalQueryParam gets an optional query parameter
func OptionalQueryParam(r *http.Request, key string) (string, bool) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return "", false
	}
	return value, true
}
