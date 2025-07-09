#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🍺 Push to Homebrew Tap${NC}"
echo "======================="

# Configuration
TAP_REPO="MadAppGang/homebrew-meroku"
FORMULA_NAME="meroku"
TEMP_DIR="homebrew-tap-tmp"

# Check if formula exists
if [ ! -f "app/dist/homebrew/${FORMULA_NAME}.rb" ]; then
    echo -e "${RED}Error: Formula not found at app/dist/homebrew/${FORMULA_NAME}.rb${NC}"
    echo "Please run goreleaser first to generate the formula"
    exit 1
fi

# Step 1: Clone the tap repository
echo -e "\n${YELLOW}Step 1: Cloning tap repository...${NC}"
if [ -d "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR"
fi

gh repo clone "$TAP_REPO" "$TEMP_DIR" -- --depth=1
cd "$TEMP_DIR"

# Step 2: Copy the formula
echo -e "\n${YELLOW}Step 2: Updating formula...${NC}"
cp "../app/dist/homebrew/${FORMULA_NAME}.rb" "${FORMULA_NAME}.rb"

# Step 3: Check if there are changes
if git diff --quiet; then
    echo -e "${YELLOW}No changes detected in formula${NC}"
    cd ..
    rm -rf "$TEMP_DIR"
    exit 0
fi

# Step 4: Commit and push
echo -e "\n${YELLOW}Step 3: Committing changes...${NC}"
git add "${FORMULA_NAME}.rb"

# Get version from formula
VERSION=$(grep -E '^\s*version\s+' "${FORMULA_NAME}.rb" | sed 's/.*version "\(.*\)".*/\1/')

git commit -m "Update ${FORMULA_NAME} to ${VERSION}"

echo -e "\n${YELLOW}Step 4: Pushing to tap...${NC}"
git push origin main

echo -e "\n${GREEN}✅ Successfully pushed to tap!${NC}"
echo -e "${GREEN}Users can now install with:${NC}"
echo "  brew tap MadAppGang/meroku"
echo "  brew install meroku"

# Cleanup
cd ..
rm -rf "$TEMP_DIR"