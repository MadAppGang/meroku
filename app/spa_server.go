package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:webapp
var webFiles embed.FS

// printEmbeddedFiles prints the contents of an embedded filesystem for debugging
func printEmbeddedFiles(fsys fs.FS, title string) {
	fmt.Printf("\n=== %s ===\n", title)
	fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error walking path %s: %v\n", path, err)
			return err
		}
		if d.IsDir() {
			fmt.Printf("DIR:  %s/\n", path)
		} else {
			info, _ := d.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			fmt.Printf("FILE: %s (size: %d bytes)\n", path, size)
		}
		return nil
	})
	fmt.Println("=========================")
}

// mainRouter handles routing between API and SPA requests
func mainRouter() http.Handler {
	mux := http.NewServeMux()

	// Register API routes
	mux.HandleFunc("/api/environments", corsMiddleware(getEnvironments))
	mux.HandleFunc("/api/environment", corsMiddleware(getEnvironmentConfig))
	mux.HandleFunc("/api/environment/update", corsMiddleware(updateEnvironmentConfig))
	mux.HandleFunc("/api/account", corsMiddleware(getCurrentAccount))
	mux.HandleFunc("/api/profiles", corsMiddleware(getAWSProfiles))
	mux.HandleFunc("/api/ecs/cluster", corsMiddleware(getECSClusterInfo))
	mux.HandleFunc("/api/ecs/network", corsMiddleware(getECSNetworkInfo))
	mux.HandleFunc("/api/ecs/services", corsMiddleware(getECSServicesInfo))
	// Autoscaling endpoints
	mux.HandleFunc("/api/ecs/autoscaling", corsMiddleware(getServiceAutoscaling))
	mux.HandleFunc("/api/ecs/scaling-history", corsMiddleware(getServiceScalingHistory))
	mux.HandleFunc("/api/ecs/metrics", corsMiddleware(getServiceMetrics))
	// Logs endpoints
	mux.HandleFunc("/api/logs", corsMiddleware(getServiceLogs))
	mux.HandleFunc("/ws/logs", streamServiceLogs) // WebSocket doesn't need CORS middleware
	// Tasks endpoints
	mux.HandleFunc("/api/ecs/tasks", corsMiddleware(getServiceTasks))
	// SSH endpoints
	mux.HandleFunc("/api/ssh/capability", corsMiddleware(getSSHCapability))
	mux.HandleFunc("/ws/ssh", startSSHSessionPTY) // WebSocket doesn't need CORS middleware - Using PTY version
	// SSM Parameter endpoints
	mux.HandleFunc("/api/ssm/parameter", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getSSMParameter(w, r)
		case http.MethodPut, http.MethodPost:
			putSSMParameter(w, r)
		case http.MethodDelete:
			deleteSSMParameter(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/ssm/parameters", corsMiddleware(listSSMParameters))
	// S3 File Management endpoints
	mux.HandleFunc("/api/s3/file", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getS3File(w, r)
		case http.MethodPut, http.MethodPost:
			putS3File(w, r)
		case http.MethodDelete:
			deleteS3File(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/s3/files", corsMiddleware(listS3Files))
	mux.HandleFunc("/api/s3/buckets", corsMiddleware(listBuckets))
	// Node positions endpoints
	mux.HandleFunc("/api/positions", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getNodePositions(w, r)
		case http.MethodPost, http.MethodPut:
			saveNodePositions(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	// EventBridge endpoints
	mux.HandleFunc("/api/eventbridge/send-test-event", corsMiddleware(sendTestEvent))
	mux.HandleFunc("/api/eventbridge/event-tasks", corsMiddleware(getEventTaskInfo))
	// SES endpoints
	mux.HandleFunc("/api/ses/status", corsMiddleware(getSESStatus))
	mux.HandleFunc("/api/ses/sandbox-info", corsMiddleware(getSESSandboxInfo))
	mux.HandleFunc("/api/ses/send-test-email", corsMiddleware(sendTestEmail))
	mux.HandleFunc("/api/ses/request-production", corsMiddleware(submitSESProductionAccess))
	mux.HandleFunc("/api/ses/production-access-prefill", corsMiddleware(getProductionAccessPrefill))
	
	// GitHub OAuth Device Flow endpoints
	mux.HandleFunc("/api/github/oauth/device", corsMiddleware(initiateGitHubDeviceFlow))
	mux.HandleFunc("/api/github/oauth/status", corsMiddleware(checkGitHubDeviceFlowStatus))
	mux.HandleFunc("/api/github/oauth/session", corsMiddleware(deleteGitHubDeviceFlowSession))

	// Amplify endpoints
	mux.HandleFunc("/api/amplify/apps", corsMiddleware(getAmplifyApps))
	mux.HandleFunc("/api/amplify/build-logs", corsMiddleware(getAmplifyBuildLogs))
	mux.HandleFunc("/api/amplify/trigger-build", corsMiddleware(triggerAmplifyBuild))

	// SPA handler for all other routes
	mux.HandleFunc("/", spaHandler())

	return mux
}

func spaHandler() http.HandlerFunc {
	// Get the embedded filesystem, stripping the "dist" prefix
	fsys, err := fs.Sub(webFiles, "webapp")
	if err != nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Failed to load web files", http.StatusInternalServerError)
		}
	}

	fileServer := http.FileServer(http.FS(fsys))
	// Print the content of the embedded folder for debugging
	printEmbeddedFiles(fsys, "Embedded webapp files")

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Remove leading slash for fs.Stat
		trimmedPath := strings.TrimPrefix(path, "/")
		if trimmedPath == "" {
			trimmedPath = "index.html"
		}

		// Check if the file exists
		_, err := fs.Stat(fsys, trimmedPath)
		// If file doesn't exist, serve index.html for client-side routing
		if err != nil {
			// Serve index.html for SPA routing
			r.URL.Path = "/"
			trimmedPath = "index.html"
		}

		// Set proper content type for known file types
		switch {
		case strings.HasSuffix(trimmedPath, ".js"):
			w.Header().Set("Content-Type", "application/javascript")
		case strings.HasSuffix(trimmedPath, ".css"):
			w.Header().Set("Content-Type", "text/css")
		case strings.HasSuffix(trimmedPath, ".html"):
			w.Header().Set("Content-Type", "text/html")
		case strings.HasSuffix(trimmedPath, ".json"):
			w.Header().Set("Content-Type", "application/json")
		case strings.HasSuffix(trimmedPath, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		}

		// Serve the file
		fileServer.ServeHTTP(w, r)
	}
}

func startSPAServer(port string) {
	startSPAServerWithAutoOpen(port, true, true)
}

func startSPAServerWithAutoOpen(port string, autoOpen bool, showTUI bool) {
	// Create the main router
	router := mainRouter()

	// Wrap the router with a global CORS handler
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for ALL requests
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")
		
		// Handle preflight requests for any path
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		
		// Otherwise, serve the request with the router
		router.ServeHTTP(w, r)
	})

	// Start server in a goroutine
	serverURL := "http://localhost:" + port
	go func() {
		if err := http.ListenAndServe(":"+port, corsHandler); err != nil {
			fmt.Printf("Failed to start server: %v\n", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(1 * time.Second)

	fmt.Printf("Web server started at %s\n", serverURL)

	// Open the web app if requested
	if autoOpen {
		if err := openBrowser(serverURL); err != nil {
			fmt.Printf("Failed to open browser: %v\n", err)
		}
	}

	// Run the TUI if requested
	if showTUI {
		if err := runWebServerTUI(serverURL); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
		}
	}
}
