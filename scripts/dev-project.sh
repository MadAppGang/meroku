#!/usr/bin/env bash
#
# dev-project.sh — prepare a meroku project folder that runs THIS working copy.
#
# A meroku project normally vendors its own copy of the Terraform modules in
# <project>/infrastructure and runs an installed `meroku`. That means nothing you
# change here affects a real deploy until it is copied across. This wires a
# folder — new or existing — directly to this checkout:
#
#   <project>/infrastructure  ->  symlink to this repo (modules + env/main.hbs)
#   <project>/meroku          ->  symlink to a freshly built dev binary
#
# Then just `cd <project> && ./meroku`. No PATH changes, nothing to source,
# nothing installed system-wide. Delete the two symlinks to undo.
#
# (scripts/dev-session.sh does the same thing via PATH instead, if you would
# rather type `meroku` than `./meroku`.)
#
# Usage:
#   scripts/dev-project.sh <project-dir>
#
# The AWS profile is not passed here — meroku reads it from aws_profile in the
# environment's YAML and exports AWS_PROFILE itself.

set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT="${1:-}"

if [ -z "$PROJECT" ]; then
	echo "usage: $0 <project-dir>" >&2
	exit 1
fi

# --- build the dev binary ------------------------------------------------------
echo "==> building meroku from $REPO"
mkdir -p "$REPO/bin"
(cd "$REPO/app" && go build -o "$REPO/bin/meroku" .)
echo "    $REPO/bin/meroku"

# --- prepare the folder --------------------------------------------------------
if [ ! -d "$PROJECT" ]; then
	mkdir -p "$PROJECT"
	echo "==> created $PROJECT"
fi
PROJECT="$(cd "$PROJECT" && pwd)"

# infrastructure/ -> this repo. If a real vendored directory is already there,
# move it aside rather than destroying it.
LINK="$PROJECT/infrastructure"
if [ -L "$LINK" ]; then
	rm "$LINK"
elif [ -d "$LINK" ]; then
	BACKUP="$LINK.vendored.$(date +%Y%m%d-%H%M%S)"
	mv "$LINK" "$BACKUP"
	echo "==> moved existing vendored copy aside: $(basename "$BACKUP")"
fi
ln -s "$REPO" "$LINK"
echo "==> infrastructure -> $REPO"

# ./meroku -> the dev binary, so `./meroku` always runs what you just built.
ln -sf "$REPO/bin/meroku" "$PROJECT/meroku"
echo "==> meroku -> $REPO/bin/meroku"

# Keep the symlinks and generated output out of the project's git history.
if [ -d "$PROJECT/.git" ]; then
	for entry in "/infrastructure" "/meroku" "/bin/"; do
		grep -qxF "$entry" "$PROJECT/.gitignore" 2>/dev/null || echo "$entry" >>"$PROJECT/.gitignore"
	done
	echo "==> added symlinks to $PROJECT/.gitignore"
fi

# --- verify the wiring actually delivers this checkout -------------------------
echo
echo "==> verifying"
if [ -f "$LINK/modules/workloads/lambda.tf" ] && grep -q 'var.project}_lambda_deploy_iam' "$LINK/modules/workloads/lambda.tf"; then
	echo "    modules:  project-scoped IAM names present (this checkout is live)"
else
	echo "    modules:  WARNING — project-scoped names not found; link may be wrong" >&2
fi
[ -f "$LINK/env/main.hbs" ] && echo "    template: env/main.hbs reachable" || echo "    template: WARNING — env/main.hbs missing" >&2
echo "    binary:   $("$PROJECT/meroku" --help 2>&1 | head -1 | cut -c1-60)"

cat <<EOF

Ready.

  cd $PROJECT
  ./meroku

To undo:  rm $PROJECT/infrastructure $PROJECT/meroku
To reset an environment to empty:
  $REPO/scripts/dev-reset.sh $PROJECT <env>
EOF
