#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}🍺 Homebrew Core Update Script${NC}"
echo "================================"

# Configuration
REPO_OWNER="MadAppGang"
REPO_NAME="infrastructure"
FORMULA_NAME="meroku"
APP_DIR="app"

# Get version
if [ -z "$1" ]; then
    echo -e "${RED}Error: Please provide new version${NC}"
    echo "Usage: $0 <new_version> [old_version]"
    echo "Example: $0 3.1.7 3.1.6"
    exit 1
fi

NEW_VERSION="${1#v}"
OLD_VERSION="${2#v}"

echo -e "${BLUE}New version: ${NEW_VERSION}${NC}"

# Step 1: Create and push new tag
echo -e "\n${YELLOW}Step 1: Creating release tag...${NC}"
TAG="v${NEW_VERSION}"

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
            --notes "Release ${TAG}"
    fi
    cd ..
fi

# Step 3: Calculate new SHA256
echo -e "\n${YELLOW}Step 3: Calculating SHA256 for new version...${NC}"
TARBALL_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/archive/refs/tags/${TAG}.tar.gz"
TARBALL_FILE="${FORMULA_NAME}-${NEW_VERSION}.tar.gz"

curl -L "$TARBALL_URL" -o "$TARBALL_FILE"
NEW_SHA256=$(shasum -a 256 "$TARBALL_FILE" | awk '{print $1}')
rm -f "$TARBALL_FILE"

echo -e "${GREEN}✓ New SHA256: ${NEW_SHA256}${NC}"

# Step 4: Fork and sync homebrew-core
echo -e "\n${YELLOW}Step 4: Preparing homebrew-core fork...${NC}"
USERNAME=$(gh api user -q .login)
TEMP_DIR="homebrew-core-update-${FORMULA_NAME}-${NEW_VERSION}"

# Ensure fork exists
gh repo fork homebrew/homebrew-core --clone=false 2>/dev/null || true

# Clone the fork
if [ -d "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR"
fi

gh repo clone "${USERNAME}/homebrew-core" "$TEMP_DIR" -- --depth=50
cd "$TEMP_DIR"

# Sync with upstream
git remote remove upstream 2>/dev/null || true
git remote add upstream https://github.com/Homebrew/homebrew-core.git
git fetch upstream master --depth=50
git checkout master
git reset --hard upstream/master

# Step 5: Create branch and update formula
echo -e "\n${YELLOW}Step 5: Updating formula...${NC}"
BRANCH_NAME="${FORMULA_NAME}-${NEW_VERSION}"
git checkout -b "$BRANCH_NAME"

# Update the formula file
FORMULA_PATH="Formula/m/${FORMULA_NAME}.rb"

if [ ! -f "$FORMULA_PATH" ]; then
    echo -e "${RED}Error: Formula not found at $FORMULA_PATH${NC}"
    echo "Make sure your initial PR was merged first!"
    exit 1
fi

# If old version not provided, detect it from the formula
if [ -z "$OLD_VERSION" ]; then
    OLD_VERSION=$(grep -E '^\s*url' "$FORMULA_PATH" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/^v//')
    echo -e "${BLUE}Detected old version: ${OLD_VERSION}${NC}"
fi

# Update URL and SHA256
sed -i.bak "s|/tags/v${OLD_VERSION}\.tar\.gz|/tags/v${NEW_VERSION}.tar.gz|g" "$FORMULA_PATH"
sed -i.bak "s|sha256 \"[a-f0-9]*\"|sha256 \"${NEW_SHA256}\"|" "$FORMULA_PATH"
rm -f "${FORMULA_PATH}.bak"

# Show the changes
echo -e "\n${YELLOW}Changes made:${NC}"
git diff --color "$FORMULA_PATH"

# Step 6: Test the formula
echo -e "\n${YELLOW}Step 6: Testing formula...${NC}"
brew install --build-from-source "$FORMULA_PATH"
brew test "$FORMULA_NAME"
brew audit --new "$FORMULA_NAME" || true

# Step 7: Commit and push
echo -e "\n${YELLOW}Step 7: Committing changes...${NC}"
git add "$FORMULA_PATH"
git commit -m "${FORMULA_NAME} ${NEW_VERSION}

Release notes: https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/tag/v${NEW_VERSION}"

git push origin "$BRANCH_NAME"

# Step 8: Create pull request
echo -e "\n${YELLOW}Step 8: Creating pull request...${NC}"
PR_BODY=$(cat << EOF
## Description
Update ${FORMULA_NAME} from ${OLD_VERSION} to ${NEW_VERSION}

## Release Notes
https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/tag/v${NEW_VERSION}

## Checklist
- [ ] \`brew audit --strict ${FORMULA_NAME}\` passes
- [ ] \`brew test ${FORMULA_NAME}\` passes
- [ ] The formula builds on all Homebrew-supported macOS versions
EOF
)

PR_URL=$(gh pr create \
    --repo homebrew/homebrew-core \
    --title "${FORMULA_NAME} ${NEW_VERSION}" \
    --body "$PR_BODY" \
    --base master \
    --head "${USERNAME}:${BRANCH_NAME}")

echo -e "\n${GREEN}✅ Update PR created!${NC}"
echo -e "${GREEN}PR URL: ${PR_URL}${NC}"

# Cleanup
cd ..
echo -e "\n${YELLOW}Cleanup: The cloned repository is at ${TEMP_DIR}${NC}"
echo "You can delete it with: rm -rf ${TEMP_DIR}"

# Additional tips
echo -e "\n${BLUE}Tips for faster PR approval:${NC}"
echo "1. Make sure all CI checks pass"
echo "2. The PR should only update version and SHA256"
echo "3. Include release notes link in the PR"
echo "4. Respond promptly to maintainer feedback"