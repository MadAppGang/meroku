#!/bin/sh

# Expose all variables from $ENV1
while IFS= read -r line || [ -n "$line" ]; do
    # Skip empty lines and comments
    case "$line" in
        \#*|"") continue ;;
    esac
    # Remove leading/trailing whitespace and export the variable
    line=${line#"${line%%[![:space:]]*}"}
    line=${line%"${line##*[![:space:]]}"}
    [ -n "$line" ] && export "$line"
done << EOF
$ENV
EOF

# Check if at least one argument (the app name) is provided
if [ $# -eq 0 ]; then
    printf "Usage: %s <app_name> [app_arguments...]\n" "$0" >&2
    exit 1
fi

# The first argument is the app name
APP="$1"
shift

# Run the app with the remaining arguments
exec "$APP" "$@"
