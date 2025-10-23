#!/bin/bash
# migrate-backups.sh
# Moves all existing backup files into a backup/ subdirectory
# Run this from your project infrastructure folder (where dev.yaml, prod.yaml are located)

set -e

echo "🔄 Backup File Migration Script"
echo "================================"
echo ""

# Check if we're in the right directory
if [ ! -f "dev.yaml" ] && [ ! -f "prod.yaml" ]; then
    echo "❌ Error: No YAML files found in current directory"
    echo "Please run this script from your project infrastructure folder"
    exit 1
fi

# Create backup directory
mkdir -p backup

# Count backup files
backup_count=$(find . -maxdepth 1 -name "*.backup*" -type f | wc -l)

if [ "$backup_count" -eq 0 ]; then
    echo "✅ No backup files found in current directory"
    echo "   All backups are already organized!"
    exit 0
fi

echo "Found $backup_count backup file(s) in current directory"
echo ""

# Move backup files
moved=0
for file in *.backup*; do
    if [ -f "$file" ]; then
        echo "  Moving: $file → backup/$file"
        mv "$file" "backup/"
        ((moved++))
    fi
done

echo ""
echo "✅ Successfully moved $moved backup file(s) to backup/ directory"
echo ""

# Check if .gitignore exists and add backup/ if needed
if [ -f ".gitignore" ]; then
    if grep -q "^backup/$" .gitignore 2>/dev/null; then
        echo "✓ .gitignore already contains backup/"
    else
        echo "backup/" >> .gitignore
        echo "✅ Added backup/ to .gitignore"
    fi
else
    echo "backup/" > .gitignore
    echo "✅ Created .gitignore with backup/"
fi

echo ""
echo "📁 Your project structure is now organized:"
echo "   project-folder/"
echo "   ├── dev.yaml"
echo "   ├── prod.yaml"
echo "   ├── .gitignore (contains backup/)"
echo "   └── backup/"
echo "       ├── dev.yaml.backup_20251022_155657"
echo "       ├── prod.yaml.backup_20251022_155647"
echo "       └── ..."
