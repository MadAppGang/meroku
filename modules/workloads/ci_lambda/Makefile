.PHONY: build test integration-test clean help

# Build the Lambda function
build:
	@echo "🔨 Building Lambda function..."
	GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
	@echo "✅ Build complete: bootstrap"

# Build the integration test
build-test:
	@echo "🔨 Building integration test..."
	go build -o integration_test cmd/integration_test.go
	@echo "✅ Test binary created: integration_test"

# Run integration test (requires AWS credentials and deployed infrastructure)
integration-test: build-test
	@echo "🧪 Running integration tests..."
	@echo ""
	./integration_test

# Run unit tests
test:
	@echo "🧪 Running unit tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f bootstrap integration_test ci_lambda.zip
	@echo "✅ Clean complete"

# Package Lambda for deployment
package: build
	@echo "📦 Packaging Lambda function..."
	zip ci_lambda.zip bootstrap
	@echo "✅ Package created: ci_lambda.zip"

# Show help
help:
	@echo "Available targets:"
	@echo "  build             - Build Lambda function (bootstrap)"
	@echo "  build-test        - Build integration test binary"
	@echo "  integration-test  - Run integration tests against deployed infrastructure"
	@echo "  test              - Run unit tests"
	@echo "  package           - Build and package Lambda for deployment"
	@echo "  clean             - Remove build artifacts"
	@echo "  help              - Show this help message"
