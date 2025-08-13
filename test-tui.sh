#!/bin/bash

# Test script for the full-screen diff view
# This creates a fake TTY environment to test the TUI

echo "Testing Terraform Plan TUI with full-screen diff view..."
echo "Instructions:"
echo "1. Use arrow keys to navigate the tree"
echo "2. Press Enter on a resource to open full-screen diff view"
echo "3. In full-screen view:"
echo "   - Use ↑/↓ or j/k to scroll"
echo "   - Use PgUp/PgDn for faster scrolling"
echo "   - Press Enter or Esc to return to main view"
echo "4. Press 'q' to quit"
echo ""
echo "Press any key to continue..."
read -n 1

# Run the TUI with the test plan
./meroku --renderdiff /Users/jack/mag/infrastructure/test-complex-plan.json