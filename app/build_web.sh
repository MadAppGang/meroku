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

# Build the web app
echo "Running pnpm build..."
pnpm build

# Copy dist to app directory
echo "Copying dist files..."
rm -rf ../app/dist
cp -r dist ../app/

# Return to original directory
cd "$CURRENT_DIR"

echo "Web build complete! Now you can build the Go binary with:"
echo "go build -o meroku ."