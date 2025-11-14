package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
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

	// Register API routes - Environment Management
	mux.HandleFunc("/api/environments", corsMiddleware(getEnvironments))
	mux.HandleFunc("/api/environment", corsMiddleware(getEnvironmentConfig))
	mux.HandleFunc("/api/environment/update", corsMiddleware(updateEnvironmentConfig))
	
	// Account & AWS
	mux.HandleFunc("/api/account", corsMiddleware(getCurrentAccount))
	mux.HandleFunc("/api/aws/profiles", corsMiddleware(getAWSProfiles))
	
	// Positions
	mux.HandleFunc("/api/positions", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getNodePositions(w, r)
		} else {
			saveNodePositions(w, r)
		}
	}))
	
	// ECS
	mux.HandleFunc("/api/ecs/cluster", corsMiddleware(getECSClusterInfo))
	mux.HandleFunc("/api/ecs/network", corsMiddleware(getECSNetworkInfo))
	mux.HandleFunc("/api/ecs/services", corsMiddleware(getECSServicesInfo))
	mux.HandleFunc("/api/ecs/tasks", corsMiddleware(getServiceTasks))
	mux.HandleFunc("/api/ecs/autoscaling", corsMiddleware(getServiceAutoscaling))
	mux.HandleFunc("/api/ecs/scaling-history", corsMiddleware(getServiceScalingHistory))
	mux.HandleFunc("/api/ecs/metrics", corsMiddleware(getServiceMetrics))

	// API Gateway
	mux.HandleFunc("/api/apigateway/info", corsMiddleware(getAPIGatewayInfo))

	// RDS
	mux.HandleFunc("/api/rds/endpoint", corsMiddleware(getDatabaseEndpoint))
	mux.HandleFunc("/api/rds/info", corsMiddleware(getDatabaseInfo))
	
	// SSM Parameters
	mux.HandleFunc("/api/ssm/parameter", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getSSMParameter(w, r)
		case http.MethodPut:
			putSSMParameter(w, r)
		case http.MethodDelete:
			deleteSSMParameter(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/ssm/parameters", corsMiddleware(listSSMParameters))
	
	// S3
	mux.HandleFunc("/api/s3/file", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getS3File(w, r)
		case http.MethodPut:
			putS3File(w, r)
		case http.MethodDelete:
			deleteS3File(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/s3/files", corsMiddleware(listS3Files))
	mux.HandleFunc("/api/s3/buckets", corsMiddleware(listProjectS3Buckets))
	
	// SES
	mux.HandleFunc("/api/ses/status", corsMiddleware(getSESStatus))
	mux.HandleFunc("/api/ses/sandbox-info", corsMiddleware(getSESSandboxInfo))
	mux.HandleFunc("/api/ses/send-test-email", corsMiddleware(sendTestEmail))
	mux.HandleFunc("/api/ses/production-access-prefill", corsMiddleware(getProductionAccessPrefill))
	mux.HandleFunc("/api/ses/request-production", corsMiddleware(submitSESProductionAccess))
	
	// EventBridge
	mux.HandleFunc("/api/eventbridge/send-test-event", corsMiddleware(sendTestEvent))
	mux.HandleFunc("/api/eventbridge/event-tasks", corsMiddleware(getEventTaskInfo))
	
	// GitHub OAuth
	mux.HandleFunc("/api/github/oauth/device", corsMiddleware(initiateGitHubDeviceFlow))
	mux.HandleFunc("/api/github/oauth/status", corsMiddleware(checkGitHubDeviceFlowStatus))
	mux.HandleFunc("/api/github/oauth/session", corsMiddleware(deleteGitHubDeviceFlowSession))
	
	// Amplify
	mux.HandleFunc("/api/amplify/apps", corsMiddleware(getAmplifyApps))
	mux.HandleFunc("/api/amplify/build-logs", corsMiddleware(getAmplifyBuildLogs))
	mux.HandleFunc("/api/amplify/trigger-build", corsMiddleware(triggerAmplifyBuild))
	
	// SSH
	mux.HandleFunc("/api/ssh/capability", corsMiddleware(getSSHCapability))
	
	// Logs
	mux.HandleFunc("/api/logs", corsMiddleware(getServiceLogs))
	
	// Pricing
	mux.HandleFunc("/api/pricing", corsMiddleware(getPricing))
	mux.HandleFunc("/api/pricing/rates", corsMiddleware(getPricingRates))
	
	// Buckets
	mux.HandleFunc("/api/buckets", corsMiddleware(listBuckets))

	// ECR Cross-Account Configuration
	mux.HandleFunc("/api/environments/ecr-sources", corsMiddleware(getECRSources))
	mux.HandleFunc("/api/environments/configure-cross-account-ecr", corsMiddleware(configureCrossAccountECR))
	mux.HandleFunc("/api/environments/check-ecr-trust-policy", corsMiddleware(checkECRTrustPolicyDeployedInAWS))

	// WebSocket endpoints (these handle their own CORS)
	mux.HandleFunc("/ws/logs", streamServiceLogs)
	mux.HandleFunc("/ws/ssh", startSSHSession)
	mux.HandleFunc("/ws/ssh-pty", startSSHSessionPTY)

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
	// printEmbeddedFiles(fsys, "Embedded webapp files") // Disabled debug output

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

// isPortAvailable checks if a port is available for use
func isPortAvailable(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// findAvailablePort finds an available port starting from the preferred port
func findAvailablePort(preferredPort string) (string, bool) {
	// Try preferred port first
	if isPortAvailable(preferredPort) {
		return preferredPort, false
	}

	// Try ports 8081-8089
	for port := 8081; port <= 8089; port++ {
		portStr := fmt.Sprintf("%d", port)
		if isPortAvailable(portStr) {
			return portStr, true
		}
	}

	return "", false
}

func startSPAServer(preferredPort string) {
	// Find an available port
	port, portWasInUse := findAvailablePort(preferredPort)
	if port == "" {
		fmt.Printf("\n⚠️  ERROR: No available ports found between 8080-8089.\n")
		fmt.Printf("    Please close other meroku instances or services using these ports.\n\n")
		fmt.Printf("Press Enter to return to main menu...")
		fmt.Scanln()
		return
	}

	// Warn user if preferred port was already in use
	if portWasInUse {
		fmt.Printf("\n⚠️  WARNING: Port %s is already in use (possibly another meroku instance).\n", preferredPort)
		fmt.Printf("    Using port %s instead.\n", port)
		fmt.Printf("    You may want to close the other instance to avoid confusion.\n\n")
		time.Sleep(2 * time.Second)
	}

	// Create the main router
	router := mainRouter()

	// Channel to receive server start errors
	errChan := make(chan error, 1)

	// Start server in a goroutine
	serverURL := "http://localhost:" + port
	go func() {
		if err := http.ListenAndServe(":"+port, router); err != nil {
			errChan <- err
		}
	}()

	// Give the server a moment to start and check for errors
	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		fmt.Printf("\n⚠️  ERROR: Failed to start server: %v\n", err)
		fmt.Printf("Press Enter to return to main menu...")
		fmt.Scanln()
		return
	default:
		// Server started successfully
	}

	// Show success message with the actual port being used
	if port != preferredPort {
		fmt.Printf("✓ Server started successfully on port %s\n", port)
	}

	// Open the web app
	if err := openBrowser(serverURL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	// Run the TUI
	if err := runWebServerTUI(serverURL); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
	}
}

func startSPAServerWithAutoOpen(preferredPort string, autoOpen bool, runTUI bool) {
	// Find an available port
	port, portWasInUse := findAvailablePort(preferredPort)
	if port == "" {
		fmt.Printf("\n⚠️  ERROR: No available ports found between 8080-8089.\n")
		fmt.Printf("    Please close other meroku instances or services using these ports.\n\n")
		if runTUI {
			fmt.Printf("Press Enter to return to main menu...")
			fmt.Scanln()
		}
		return
	}

	// Warn user if preferred port was already in use
	if portWasInUse {
		fmt.Printf("\n⚠️  WARNING: Port %s is already in use (possibly another meroku instance).\n", preferredPort)
		fmt.Printf("    Using port %s instead.\n", port)
		fmt.Printf("    You may want to close the other instance to avoid confusion.\n\n")
		time.Sleep(2 * time.Second)
	}

	// Create the main router
	router := mainRouter()

	// Channel to receive server start errors
	errChan := make(chan error, 1)

	// Start server in a goroutine
	serverURL := "http://localhost:" + port
	go func() {
		if err := http.ListenAndServe(":"+port, router); err != nil {
			errChan <- err
		}
	}()

	// Give the server a moment to start and check for errors
	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		fmt.Printf("\n⚠️  ERROR: Failed to start server: %v\n", err)
		if runTUI {
			fmt.Printf("Press Enter to return to main menu...")
			fmt.Scanln()
		}
		return
	default:
		// Server started successfully
	}

	// Show success message with the actual port being used
	if port != preferredPort {
		fmt.Printf("✓ Server started successfully on port %s\n", port)
	}

	// Open the web app if requested
	if autoOpen {
		if err := openBrowser(serverURL); err != nil {
			fmt.Printf("Failed to open browser: %v\n", err)
		}
	}

	// Run the TUI if requested
	if runTUI {
		if err := runWebServerTUI(serverURL); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
		}
	}
}
