.PHONY: build build-web build-all clean run release release-snapshot release-check

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

# Release commands
release-check:
	@echo "Checking release requirements..."
	@which goreleaser > /dev/null || (echo "Error: goreleaser not installed. Run: brew install goreleaser" && exit 1)
	@echo "✓ GoReleaser installed"
	@echo "✓ Ready to release"

release-snapshot:
	@echo "Creating snapshot release (local only)..."
	cd .. && goreleaser release --snapshot --clean

release:
	@echo "Creating release..."
	@echo "Make sure you've tagged the release: git tag -a v1.0.0 -m 'Release v1.0.0'"
	@echo "And pushed the tag: git push origin v1.0.0"
	cd .. && goreleaser release --clean