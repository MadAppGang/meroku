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
	mux.HandleFunc("/api/environments", getEnvironments)
	mux.HandleFunc("/api/environment", getEnvironmentConfig)
	mux.HandleFunc("/api/environment/update", updateEnvironmentConfig)

	// Account & AWS
	mux.HandleFunc("/api/account", getCurrentAccount)
	mux.HandleFunc("/api/aws/profiles", getAWSProfiles)

	// DNS — delegation status, parent-zone discovery and the delegation write.
	// Until these existed the DNS subsystem was CLI-only, so the web UI had to
	// guess at the records it displayed.
	registerDNSRoutes(mux)

	// Positions
	mux.HandleFunc("/api/positions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getNodePositions(w, r)
		} else {
			saveNodePositions(w, r)
		}
	})

	// VPC range: validation, overlap report and free-range suggestions
	mux.HandleFunc("/api/vpc-cidr", getVPCCIDRReport)

	// Configuration warnings: what generate would warn about, per env
	mux.HandleFunc("/api/config-warnings", getConfigWarnings)

	// Neon: live choices for the Neon panels. The handlers repeat the
	// router-wide access check (web_access.go) themselves.
	mux.HandleFunc("/api/neon/lookup", getNeonLookup)
	mux.HandleFunc("/api/neon/schemas", getNeonSchemas)

	// SES DNS records: zone origin, live apex values, suggested sender record
	mux.HandleFunc("/api/ses-dns", getSESDNSReport)

	// ECS
	mux.HandleFunc("/api/ecs/cluster", getECSClusterInfo)
	mux.HandleFunc("/api/ecs/network", getECSNetworkInfo)
	mux.HandleFunc("/api/ecs/services", getECSServicesInfo)
	mux.HandleFunc("/api/ecs/tasks", getServiceTasks)
	mux.HandleFunc("/api/ecs/autoscaling", getServiceAutoscaling)
	mux.HandleFunc("/api/ecs/scaling-history", getServiceScalingHistory)
	mux.HandleFunc("/api/ecs/metrics", getServiceMetrics)

	// Fargate
	mux.HandleFunc("/api/fargate/options", getFargateOptions)

	// Compute pools (EC2 capacity) — read-only. Pool CRUD is a YAML write and
	// goes through /api/environment/update like every other config change, so
	// nothing here mutates anything.
	//
	// Every variable travels as a query parameter, never as a path segment: this
	// is a plain http.ServeMux with a catch-all at "/" below, so
	// /api/compute/pools/{name} would match the SPA handler and return HTML
	// with a 200 (CON-3).
	mux.HandleFunc("/api/compute/instance-types", getComputeInstanceTypes)
	mux.HandleFunc("/api/compute/spot-prices", getComputeSpotPrices)
	mux.HandleFunc("/api/compute/recommendation", getComputeRecommendation)

	// API Gateway
	mux.HandleFunc("/api/apigateway/info", getAPIGatewayInfo)

	// RDS
	mux.HandleFunc("/api/rds/endpoint", getDatabaseEndpoint)
	mux.HandleFunc("/api/rds/info", getDatabaseInfo)

	// SSM Parameters
	mux.HandleFunc("/api/ssm/parameter", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("/api/ssm/parameters", listSSMParameters)

	// Vault secret values, write-only (api_vault_secrets.go)
	mux.HandleFunc("/api/vaults/secrets", handleVaultSecrets)

	// S3
	mux.HandleFunc("/api/s3/file", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("/api/s3/files", listS3Files)
	mux.HandleFunc("/api/s3/buckets", listProjectS3Buckets)

	// SES
	mux.HandleFunc("/api/ses/status", getSESStatus)
	mux.HandleFunc("/api/ses/sandbox-info", getSESSandboxInfo)
	mux.HandleFunc("/api/ses/send-test-email", sendTestEmail)
	mux.HandleFunc("/api/ses/production-access-prefill", getProductionAccessPrefill)
	mux.HandleFunc("/api/ses/request-production", submitSESProductionAccess)

	// EventBridge
	mux.HandleFunc("/api/eventbridge/send-test-event", sendTestEvent)
	mux.HandleFunc("/api/eventbridge/event-tasks", getEventTaskInfo)
	mux.HandleFunc("/api/event-bus/share-targets", getEventBusShareTargets)

	// GitHub OAuth
	mux.HandleFunc("/api/github/oauth/device", initiateGitHubDeviceFlow)
	mux.HandleFunc("/api/github/oauth/status", checkGitHubDeviceFlowStatus)
	mux.HandleFunc("/api/github/oauth/session", deleteGitHubDeviceFlowSession)

	// Amplify
	mux.HandleFunc("/api/amplify/apps", getAmplifyApps)
	mux.HandleFunc("/api/amplify/build-logs", getAmplifyBuildLogs)
	mux.HandleFunc("/api/amplify/trigger-build", triggerAmplifyBuild)

	// SSH
	mux.HandleFunc("/api/ssh/capability", getSSHCapability)

	// Logs
	mux.HandleFunc("/api/logs", getServiceLogs)

	// Pricing
	mux.HandleFunc("/api/pricing", getPricing)
	mux.HandleFunc("/api/pricing/rates", getPricingRates)

	// Buckets
	mux.HandleFunc("/api/buckets", listBuckets)

	// ECR Cross-Account Configuration
	mux.HandleFunc("/api/environments/ecr-sources", getECRSources)
	mux.HandleFunc("/api/environments/configure-cross-account-ecr", configureCrossAccountECR)
	mux.HandleFunc("/api/environments/check-ecr-trust-policy", checkECRTrustPolicyDeployedInAWS)

	// GitHub Actions OIDC: one provider per AWS account, so a second project
	// sharing the account federates against the first project's.
	mux.HandleFunc("/api/environments/github-oidc-status", getGithubOIDCStatus)
	// And a read-only scan for the other half of the problem: two projects in
	// one account whose OIDC subject lists overlap can assume each other's
	// roles, and AWS says nothing about it.
	mux.HandleFunc("/api/environments/github-oidc-subject-conflicts", getGitHubOIDCSubjectConflicts)

	// Custom Terraform Extensions
	mux.HandleFunc("/api/custom-terraform/files", listCustomTerraformFiles)
	mux.HandleFunc("/api/custom-terraform/file", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getCustomTerraformFile(w, r)
		case http.MethodPost:
			saveCustomTerraformFile(w, r)
		case http.MethodDelete:
			deleteCustomTerraformFile(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/custom-terraform/bridge-vars", getBridgeVariables)
	mux.HandleFunc("/api/custom-terraform/modules", getCustomModuleStatus)

	// WebSocket endpoints; their upgraders apply the same access rules
	mux.HandleFunc("/ws/logs", streamServiceLogs)
	mux.HandleFunc("/ws/ssh", startSSHSession)
	mux.HandleFunc("/ws/ssh-pty", startSSHSessionPTY)

	// SPA handler for all other routes
	mux.HandleFunc("/", spaHandler())

	// Every route passes the access guard (web_access.go).
	return webAccessGuard(mux)
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
	ln, err := net.Listen("tcp", webListenAddr(port))
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

// serveWebOnPort serves the web app on the --listen address at exactly the
// given port and blocks. Unlike startSPAServer it never moves to another port,
// so a caller that already knows the URL (a test runner, a script) cannot end
// up talking to a different meroku instance that holds the preferred port.
func serveWebOnPort(port string) error {
	addr := webListenAddr(port)
	fmt.Printf("Serving the web app on http://%s\n", addr)
	shown, _ := webServerURLs(port)
	printWebAccessURL(shown)
	return http.ListenAndServe(addr, mainRouter())
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
	shownURL, openURL := webServerURLs(port)
	serverURL := webURLWithoutToken(openURL)
	go func() {
		if err := http.ListenAndServe(webListenAddr(port), router); err != nil {
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
	printWebAccessURL(shownURL)

	// Open the web app
	if err := openBrowser(openURL); err != nil {
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
	shownURL, openURL := webServerURLs(port)
	serverURL := webURLWithoutToken(openURL)
	go func() {
		if err := http.ListenAndServe(webListenAddr(port), router); err != nil {
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
	printWebAccessURL(shownURL)

	// Open the web app if requested
	if autoOpen {
		if err := openBrowser(openURL); err != nil {
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
