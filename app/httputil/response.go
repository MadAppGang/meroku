package httputil

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// RespondJSON sends a JSON response with the specified status code
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// RespondError sends a JSON error response
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, ErrorResponse{Error: message})
}

// RespondErrorWithDetail sends a JSON error response with additional details
func RespondErrorWithDetail(w http.ResponseWriter, status int, message string, detail string) {
	RespondJSON(w, status, ErrorResponse{
		Error:  message,
		Detail: detail,
	})
}

// RespondErrorWithCode sends a JSON error response with an error code
func RespondErrorWithCode(w http.ResponseWriter, status int, message string, code string) {
	RespondJSON(w, status, ErrorResponse{
		Error: message,
		Code:  code,
	})
}

// MethodGuard creates middleware that ensures only specified HTTP methods are allowed
func MethodGuard(allowedMethods ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			for _, method := range allowedMethods {
				if r.Method == method {
					next(w, r)
					return
				}
			}
			RespondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

// WithCORS adds CORS headers to HTTP responses
func WithCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

// Chain applies multiple middleware functions to a handler
func Chain(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
