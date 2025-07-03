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
	// Create the main router
	router := mainRouter()

	// Start server in a goroutine
	serverURL := "http://localhost:" + port
	go func() {
		if err := http.ListenAndServe(":"+port, router); err != nil {
			fmt.Printf("Failed to start server: %v\n", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(1 * time.Second)

	// Open the web app
	if err := openBrowser(serverURL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	// Run the TUI
	if err := runWebServerTUI(serverURL); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
	}
}
