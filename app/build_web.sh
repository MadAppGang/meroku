#!/bin/bash

# Build script for embedding web app into Go binary

echo "Building web application..."

# Save current directory
CURRENT_DIR=$(pwd)

# Navigate to web directory
cd ../web

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "Installing web dependencies..."
    pnpm install
fi

# Build the web app (vite outputs directly to ../app/webapp)
echo "Running pnpm build..."
pnpm build

# Return to original directory
cd "$CURRENT_DIR"

echo ""
echo "✓ Web build complete!"
echo "  Output: app/webapp/"
echo ""
echo "Now you can build the Go binary with:"
echo "  cd app && go build -o ../meroku ."