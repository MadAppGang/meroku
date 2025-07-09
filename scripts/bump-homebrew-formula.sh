#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}🍺 Homebrew Formula Bump Script${NC}"
echo "=================================="

# Configuration
REPO_OWNER="MadAppGang"
REPO_NAME="infrastructure"
FORMULA_NAME="meroku"
APP_DIR="app"

# Get version
if [ -z "$1" ]; then
    echo -e "${RED}Error: Please provide new version${NC}"
    echo "Usage: $0 <new_version>"
    echo "Example: $0 3.1.7"
    exit 1
fi

NEW_VERSION="${1#v}"
TAG="v${NEW_VERSION}"

echo -e "${BLUE}Updating to version: ${NEW_VERSION}${NC}"

# Step 1: Create and push tag
echo -e "\n${YELLOW}Step 1: Creating release tag...${NC}"
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo -e "${YELLOW}Tag $TAG already exists${NC}"
else
    git tag -a "$TAG" -m "Release $TAG"
    git push origin "$TAG"
    echo -e "${GREEN}✓ Tag $TAG created and pushed${NC}"
fi

# Step 2: Create GitHub release
echo -e "\n${YELLOW}Step 2: Creating GitHub release...${NC}"
if gh release view "$TAG" &>/dev/null; then
    echo -e "${YELLOW}Release $TAG already exists${NC}"
else
    cd "$APP_DIR"
    if command -v goreleaser &> /dev/null; then
        echo "Using goreleaser to create release..."
        goreleaser release --clean --skip=brew
    else
        echo "Creating release with gh CLI..."
        gh release create "$TAG" \
            --title "${FORMULA_NAME} ${TAG}" \
            --notes "Release ${TAG}" \
            --generate-notes
    fi
    cd ..
fi

# Wait a moment for GitHub to process the release
echo -e "\n${YELLOW}Waiting for GitHub to process the release...${NC}"
sleep 5

# Step 3: Use brew bump-formula-pr
echo -e "\n${YELLOW}Step 3: Creating PR with brew bump-formula-pr...${NC}"
echo -e "${BLUE}This will:${NC}"
echo "  - Fork homebrew-core (if needed)"
echo "  - Calculate new SHA256"
echo "  - Update the formula"
echo "  - Run tests"
echo "  - Create a pull request"

# Run brew bump-formula-pr
brew bump-formula-pr \
    --tag="$TAG" \
    --revision="$TAG" \
    --no-browse \
    --verbose \
    "$FORMULA_NAME"

echo -e "\n${GREEN}✅ Done!${NC}"
echo -e "${YELLOW}The PR has been created. Check your GitHub notifications for the PR link.${NC}"

# Additional notes
echo -e "\n${BLUE}What happens next:${NC}"
echo "1. Homebrew CI will automatically test your formula"
echo "2. Maintainers will review the changes"
echo "3. Once approved, it will be merged"
echo "4. Users can update with: brew upgrade ${FORMULA_NAME}"

echo -e "\n${BLUE}To check PR status:${NC}"
echo "gh pr list --repo homebrew/homebrew-core --author @me --search \"${FORMULA_NAME}\""