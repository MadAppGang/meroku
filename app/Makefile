.PHONY: build build-web build-all clean run

# Build the Go binary only
build:
	go build -o meroku .

# Build the web app and prepare for embedding
build-web:
	./build_web.sh

# Build everything (web app + Go binary with embedded files)
build-all: build-web
	go build -o meroku .

# Clean build artifacts
clean:
	rm -rf dist meroku

# Run the application
run:
	go run .

# Development mode - build and run
dev: build-all
	./meroku