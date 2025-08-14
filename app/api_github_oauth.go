package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// GitHub OAuth Device Flow constants
const (
	githubDeviceCodeURL = "https://github.com/login/device/code"
	githubAccessTokenURL = "https://github.com/login/oauth/access_token"
	githubClientID = "Ov23liWgbmfmd4SeoL6c" // GitHub OAuth App Client ID for device flow
)

// DeviceFlowSession stores the state of an ongoing device flow authorization
type DeviceFlowSession struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresIn       int       `json:"expires_in"`
	Interval        int       `json:"interval"`
	CreatedAt       time.Time `json:"created_at"`
	AccessToken     string    `json:"access_token,omitempty"`
	TokenType       string    `json:"token_type,omitempty"`
	Scope           string    `json:"scope,omitempty"`
	Status          string    `json:"status"` // pending, authorized, expired, error
	Error           string    `json:"error,omitempty"`
	AppName         string    `json:"app_name,omitempty"`
	Project         string    `json:"project,omitempty"`
	Environment     string    `json:"environment,omitempty"`
}

// In-memory storage for device flow sessions (in production, use Redis or similar)
var (
	deviceFlowSessions = make(map[string]*DeviceFlowSession)
	sessionsMutex      sync.RWMutex
)

// GitHub API response structures
type GitHubDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type GitHubAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
}

// POST /api/github/oauth/device
// Initiates GitHub device flow and returns user code
func initiateGitHubDeviceFlow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppName     string `json:"app_name"`
		Scope       string `json:"scope"`
		Project     string `json:"project"`
		Environment string `json:"environment"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.AppName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "app_name is required"})
		return
	}

	if req.Project == "" || req.Environment == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "project and environment are required"})
		return
	}

	// Default scope for Amplify
	scope := "repo"
	if req.Scope != "" {
		scope = req.Scope
	}

	// Request device code from GitHub
	payload := fmt.Sprintf("client_id=%s&scope=%s", githubClientID, scope)
	
	httpReq, err := http.NewRequest("POST", githubDeviceCodeURL, bytes.NewBufferString(payload))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to create request"})
		return
	}
	
	// Set headers as per GitHub docs - form-encoded content, JSON response
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to contact GitHub"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read GitHub response"})
		return
	}

	// Check if response is an error (status code not 2xx)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			ErrorURI         string `json:"error_uri"`
			Message          string `json:"message"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if errorResp.Error != "" || errorResp.Message != "" {
				errMsg := errorResp.Error
				if errMsg == "" {
					errMsg = errorResp.Message
				}
				if resp.StatusCode == 404 {
					errMsg = fmt.Sprintf("GitHub API returned 404. This usually means the OAuth app (client_id: %s) doesn't support device flow or the client ID is invalid. Please ensure your GitHub OAuth App has device flow enabled.", githubClientID)
				}
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{Error: errMsg})
				return
			}
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("GitHub returned status %d: %s", resp.StatusCode, string(body))})
		return
	}

	// Parse JSON response
	var deviceResp GitHubDeviceCodeResponse
	if err := json.Unmarshal(body, &deviceResp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to parse GitHub response"})
		return
	}

	// Create session
	session := &DeviceFlowSession{
		DeviceCode:      deviceResp.DeviceCode,
		UserCode:        deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		ExpiresIn:       deviceResp.ExpiresIn,
		Interval:        deviceResp.Interval,
		CreatedAt:       time.Now(),
		Status:          "pending",
		AppName:         req.AppName,
		Project:         req.Project,
		Environment:     req.Environment,
	}

	// Store session
	sessionsMutex.Lock()
	deviceFlowSessions[session.UserCode] = session
	sessionsMutex.Unlock()

	// Start background polling for access token
	go pollForAccessToken(session)

	// Return user code and verification URL to frontend
	response := map[string]interface{}{
		"user_code":        session.UserCode,
		"verification_uri": session.VerificationURI,
		"expires_in":       session.ExpiresIn,
		"interval":         session.Interval,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /api/github/oauth/status?user_code=<code>
// Check the status of device flow authorization
func checkGitHubDeviceFlowStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userCode := r.URL.Query().Get("user_code")
	if userCode == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "user_code is required"})
		return
	}

	sessionsMutex.RLock()
	session, exists := deviceFlowSessions[userCode]
	sessionsMutex.RUnlock()

	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Check if session expired
	if time.Since(session.CreatedAt) > time.Duration(session.ExpiresIn)*time.Second {
		session.Status = "expired"
	}

	response := map[string]interface{}{
		"status":     session.Status,
		"app_name":   session.AppName,
		"created_at": session.CreatedAt,
	}

	if session.Status == "authorized" {
		response["message"] = "GitHub authorization successful! Token has been stored."
	} else if session.Status == "error" {
		response["error"] = session.Error
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Background function to poll GitHub for access token
func pollForAccessToken(session *DeviceFlowSession) {
	interval := time.Duration(session.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second // GitHub minimum interval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeout := time.After(time.Duration(session.ExpiresIn) * time.Second)

	for {
		select {
		case <-ticker.C:
			// Poll GitHub for access token
			payload := fmt.Sprintf("client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code", 
				githubClientID, session.DeviceCode)
			
			httpReq, err := http.NewRequest("POST", githubAccessTokenURL, bytes.NewBufferString(payload))
			if err != nil {
				continue // Keep trying
			}
			
			// Request JSON response
			httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			httpReq.Header.Set("Accept", "application/json")
			
			client := &http.Client{}
			resp, err := client.Do(httpReq)
			if err != nil {
				continue // Keep trying
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				continue
			}

			// Parse JSON response
			var tokenResp GitHubAccessTokenResponse
			if err := json.Unmarshal(body, &tokenResp); err != nil {
				continue
			}

			// Check for errors
			if tokenResp.Error != "" {
				if tokenResp.Error == "authorization_pending" {
					continue // User hasn't authorized yet
				} else if tokenResp.Error == "slow_down" {
					// Increase interval
					ticker.Reset(interval + 5*time.Second)
					continue
				} else {
					// Other errors are terminal
					sessionsMutex.Lock()
					session.Status = "error"
					session.Error = tokenResp.Error
					sessionsMutex.Unlock()
					return
				}
			}

			// Check for access token
			if tokenResp.AccessToken != "" {
				sessionsMutex.Lock()
				session.AccessToken = tokenResp.AccessToken
				session.TokenType = tokenResp.TokenType
				session.Scope = tokenResp.Scope
				session.Status = "authorized"
				sessionsMutex.Unlock()

				// Store token in SSM
				if err := storeTokenInSSM(session); err != nil {
					sessionsMutex.Lock()
					session.Status = "error"
					session.Error = fmt.Sprintf("failed to store token: %v", err)
					sessionsMutex.Unlock()
				}

				return
			}

		case <-timeout:
			// Session expired
			sessionsMutex.Lock()
			session.Status = "expired"
			sessionsMutex.Unlock()
			return
		}
	}
}

// Store the access token in SSM Parameter Store
func storeTokenInSSM(session *DeviceFlowSession) error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	ssmClient := ssm.NewFromConfig(cfg)
	
	// Use a shared token for all Amplify apps
	paramName := fmt.Sprintf("/%s/%s/github/amplify-token", session.Project, session.Environment)
	
	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:        aws.String(paramName),
		Value:       aws.String(session.AccessToken),
		Type:        types.ParameterTypeSecureString,
		Overwrite:   aws.Bool(true),
		Description: aws.String(fmt.Sprintf("Shared GitHub OAuth token for all Amplify apps in %s/%s", session.Project, session.Environment)),
	})

	return err
}

// DELETE /api/github/oauth/session?user_code=<code>
// Clean up a device flow session
func deleteGitHubDeviceFlowSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userCode := r.URL.Query().Get("user_code")
	if userCode == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "user_code is required"})
		return
	}

	sessionsMutex.Lock()
	delete(deviceFlowSessions, userCode)
	sessionsMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "session deleted"})
}

// Cleanup expired sessions periodically
func cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sessionsMutex.Lock()
		for code, session := range deviceFlowSessions {
			if time.Since(session.CreatedAt) > time.Duration(session.ExpiresIn)*time.Second+10*time.Minute {
				delete(deviceFlowSessions, code)
			}
		}
		sessionsMutex.Unlock()
	}
}

// Initialize cleanup goroutine
func init() {
	// Only start cleanup if not running DNS commands
	if len(os.Args) < 2 || os.Args[1] != "dns" {
		go cleanupExpiredSessions()
	}
}