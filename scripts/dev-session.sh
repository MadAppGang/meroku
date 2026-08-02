#!/usr/bin/env bash
#
# dev-session.sh — point a meroku project at this working copy, for one shell session.
#
# A meroku project vendors its own copy of the Terraform modules in
# <project>/infrastructure, and normally runs an installed `meroku` binary. That
# means changes made here have no effect on a real deploy until they are copied
# across. This script wires a project to this checkout instead, so you can test
# uncommitted work against a real environment:
#
#   * <project>/infrastructure  ->  symlink to this repo (modules + env/main.hbs)
#   * PATH                      ->  prefers a freshly built ./bin/meroku
#
# Both changes are undoable and neither is committed anywhere. The PATH change
# lives only in the shell you sourced this into.
#
# Usage — MUST be sourced, not executed, or the PATH change is lost:
#
#   source scripts/dev-session.sh /path/to/project
#   source scripts/dev-session.sh --undo
#   source scripts/dev-session.sh --status
#
# Undo also happens automatically if you just close the shell and delete the
# symlink; --undo restores the vendored directory it moved aside.

# --- must be sourced -----------------------------------------------------------
# $0 differs from the script path when sourced; in zsh ZSH_EVAL_CONTEXT contains ":file".
__meroku_sourced=0
if [ -n "${ZSH_VERSION:-}" ]; then
	case "${ZSH_EVAL_CONTEXT:-}" in *:file*) __meroku_sourced=1 ;; esac
	__meroku_self="${(%):-%x}"
elif [ -n "${BASH_VERSION:-}" ]; then
	[ "${BASH_SOURCE[0]}" != "$0" ] && __meroku_sourced=1
	__meroku_self="${BASH_SOURCE[0]}"
fi

if [ "$__meroku_sourced" != "1" ]; then
	echo "dev-session.sh must be sourced so it can change PATH in your shell:" >&2
	echo "    source ${0} ${1:-/path/to/project}" >&2
	exit 1
fi

__meroku_root="$(cd "$(dirname "$__meroku_self")/.." && pwd)"
__meroku_bin="$__meroku_root/bin"
__meroku_state="$HOME/.meroku-dev-session"

__meroku_status() {
	echo "meroku dev session"
	echo "  repo:    $__meroku_root"
	if [ -f "$__meroku_state" ]; then
		# shellcheck disable=SC1090
		. "$__meroku_state"
		echo "  project: ${MEROKU_DEV_PROJECT:-<none>}"
		echo "  link:    ${MEROKU_DEV_PROJECT:-}/infrastructure -> $(readlink "${MEROKU_DEV_PROJECT:-}/infrastructure" 2>/dev/null || echo '<none>')"
		echo "  backup:  ${MEROKU_DEV_BACKUP:-<none>}"
	else
		echo "  project: <not linked>"
	fi
	echo "  meroku:  $(command -v meroku || echo '<not on PATH>')"
}

__meroku_undo() {
	if [ ! -f "$__meroku_state" ]; then
		echo "No dev session recorded — nothing to undo."
		return 0
	fi
	# shellcheck disable=SC1090
	. "$__meroku_state"

	local link="$MEROKU_DEV_PROJECT/infrastructure"
	if [ -L "$link" ]; then
		rm "$link"
		echo "removed symlink $link"
	elif [ -e "$link" ]; then
		echo "WARNING: $link is not a symlink; leaving it alone." >&2
	fi

	if [ -n "${MEROKU_DEV_BACKUP:-}" ] && [ -d "$MEROKU_DEV_BACKUP" ]; then
		mv "$MEROKU_DEV_BACKUP" "$link"
		echo "restored vendored copy from $MEROKU_DEV_BACKUP"
	fi

	# Drop our bin from PATH without disturbing the rest of it.
	PATH="$(printf '%s' "$PATH" | tr ':' '\n' | grep -vxF "$__meroku_bin" | paste -sd: -)"
	export PATH
	rm -f "$__meroku_state"
	echo "PATH restored. meroku: $(command -v meroku || echo '<not on PATH>')"
}

case "${1:-}" in
--undo)
	__meroku_undo
	return 0 2>/dev/null || true
	;;
--status)
	__meroku_status
	return 0 2>/dev/null || true
	;;
esac

__meroku_project="${1:-}"
if [ -z "$__meroku_project" ]; then
	echo "usage: source scripts/dev-session.sh /path/to/project" >&2
	echo "       source scripts/dev-session.sh --undo | --status" >&2
	return 1 2>/dev/null || true
fi

__meroku_project="$(cd "$__meroku_project" 2>/dev/null && pwd)" || {
	echo "no such directory: ${1}" >&2
	return 1 2>/dev/null || true
}

# A meroku project is identified by its <env>.yaml files.
if ! ls "$__meroku_project"/*.yaml >/dev/null 2>&1; then
	echo "WARNING: no *.yaml in $__meroku_project — is this a meroku project directory?" >&2
fi

echo "==> building meroku from $__meroku_root"
mkdir -p "$__meroku_bin"
if ! (cd "$__meroku_root/app" && go build -o "$__meroku_bin/meroku" .); then
	echo "build failed — not changing anything" >&2
	return 1 2>/dev/null || true
fi

# --- link infrastructure/ ------------------------------------------------------
__meroku_link="$__meroku_project/infrastructure"
__meroku_backup=""

if [ -L "$__meroku_link" ]; then
	rm "$__meroku_link"
elif [ -d "$__meroku_link" ]; then
	# Never delete the vendored copy: move it aside so --undo can put it back.
	__meroku_backup="$__meroku_link.vendored.$(date +%Y%m%d-%H%M%S)"
	mv "$__meroku_link" "$__meroku_backup"
	echo "==> moved vendored copy aside: $(basename "$__meroku_backup")"
fi

ln -s "$__meroku_root" "$__meroku_link"
echo "==> linked $__meroku_link -> $__meroku_root"

# --- PATH, for this shell only -------------------------------------------------
case ":$PATH:" in
*":$__meroku_bin:"*) ;;
*) export PATH="$__meroku_bin:$PATH" ;;
esac

cat >"$__meroku_state" <<EOF
MEROKU_DEV_PROJECT="$__meroku_project"
MEROKU_DEV_BACKUP="$__meroku_backup"
MEROKU_DEV_REPO="$__meroku_root"
EOF

# --- prove the wiring actually delivers this checkout --------------------------
echo
echo "==> verifying"
printf '    meroku binary: %s\n' "$(command -v meroku)"
if grep -q 'var.project}_lambda_deploy_iam' "$__meroku_link/modules/workloads/lambda.tf" 2>/dev/null; then
	printf '    modules:       %s\n' "project-scoped IAM names present (fixes are live)"
else
	printf '    modules:       %s\n' "WARNING: project-scoped IAM names NOT found — link may be wrong"
fi

echo
echo "Dev session active for $__meroku_project"
echo "  cd $__meroku_project && meroku"
echo "  source $__meroku_root/scripts/dev-session.sh --undo   # restore"
echo
echo "Note: env/<env>/main.tf is generated and still references ../../infrastructure/…,"
echo "      which now resolves through the symlink — no regeneration needed."

unset __meroku_sourced __meroku_self __meroku_link __meroku_backup __meroku_project
