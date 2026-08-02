#!/usr/bin/env bash
#
# dev-reset.sh — tear a meroku test environment back down to an empty state, so
# the next run is a genuine clean first install.
#
# Why this exists: a half-finished apply leaves an environment that is neither
# empty nor complete, and testing against it proves nothing about a first-run
# experience. Worse, a failed run can leave *another project's* resource recorded
# in this environment's state (AWS upserts EventBridge rules rather than
# rejecting duplicate names), and a naive destroy would then delete infrastructure
# belonging to that other project.
#
# So the destroy is gated behind a cross-project safety scan that aborts if any
# resource in the state is tagged for a different project, or carries one of the
# known account-global names.
#
# Usage:
#   scripts/dev-reset.sh <project-dir> <env>            # dry run: scan + plan only
#   scripts/dev-reset.sh <project-dir> <env> --yes      # actually destroy
#
# Always backs up the state to $HOME/.meroku-dev-backups first. That file
# contains real infrastructure data and must never be committed anywhere.

set -euo pipefail

PROJECT="${1:-}"
ENVNAME="${2:-}"
CONFIRM="${3:-}"

if [ -z "$PROJECT" ] || [ -z "$ENVNAME" ]; then
	echo "usage: $0 <project-dir> <env> [--yes]" >&2
	exit 1
fi

PROJECT="$(cd "$PROJECT" && pwd)"
ENVDIR="$PROJECT/env/$ENVNAME"
YAML="$PROJECT/$ENVNAME.yaml"

[ -d "$ENVDIR" ] || { echo "no such environment directory: $ENVDIR" >&2; exit 1; }
[ -f "$YAML" ] || { echo "no such config: $YAML" >&2; exit 1; }

# --- refuse to touch production ------------------------------------------------
if grep -qE '^is_prod:[[:space:]]*true' "$YAML"; then
	echo "REFUSING: $YAML has is_prod: true. This script is for test environments only." >&2
	exit 1
fi
PROJECT_NAME="$(grep -E '^project:' "$YAML" | head -1 | sed 's/^project:[[:space:]]*//' | tr -d '"'"'"' ')"
[ -n "$PROJECT_NAME" ] || { echo "could not read 'project:' from $YAML" >&2; exit 1; }

echo "project : $PROJECT_NAME"
echo "env     : $ENVNAME"
echo "dir     : $ENVDIR"
echo

cd "$ENVDIR"

# --- back up state -------------------------------------------------------------
BACKUP_DIR="$HOME/.meroku-dev-backups"
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
BACKUP="$BACKUP_DIR/${PROJECT_NAME}-${ENVNAME}-$(date +%Y%m%d-%H%M%S).tfstate"

if ! terraform state pull > "$BACKUP" 2>/dev/null || [ ! -s "$BACKUP" ]; then
	echo "could not pull state (is terraform initialised here?)" >&2
	rm -f "$BACKUP"
	exit 1
fi
chmod 600 "$BACKUP"
echo "state backed up: $BACKUP"

# --- cross-project safety scan -------------------------------------------------
# Aborts rather than destroying anything that looks like it belongs elsewhere.
echo
echo "==> safety scan"
python3 - "$BACKUP" "$PROJECT_NAME" <<'PY'
import json, sys

backup, project = sys.argv[1], sys.argv[2]
state = json.load(open(backup))

# Account-global names that meroku historically used without a project prefix.
# If one of these is in state, another project may own the live resource.
SHARED = {
    "ecr_events_cicd",
    "lambda_deploy_iam_dev", "LambdaKMSPolicy_dev", "LambdaECSDevPolicy_dev",
    "ci_lambda_dev", "/aws/lambda/ci_lambda_dev",
    "sqs-access-policy", "pgadmin",
    "ManageEndpointsAndPublishFirebaseCloudMessages",
    "AllowAdminConfirmSignUpForBackend",
}

foreign, shared_hits, total = [], [], 0
for res in state.get("resources", []):
    addr = ".".join(x for x in [res.get("module"), res.get("type"), res.get("name")] if x)
    for inst in res.get("instances", []):
        total += 1
        attrs = inst.get("attributes") or {}
        ident = str(attrs.get("name") or attrs.get("id") or "")
        tags = attrs.get("tags") or attrs.get("tags_all") or {}
        if isinstance(tags, dict):
            owner = tags.get("Project")
            if owner and owner != project:
                foreign.append((addr, owner, ident))
        if ident in SHARED:
            shared_hits.append((addr, ident))

print(f"    {total} state entries scanned")

problems = False
if foreign:
    problems = True
    print("\n    ABORT: state contains resources tagged for another project:")
    for addr, owner, ident in foreign:
        print(f"      {addr}\n        Project={owner}  name={ident}")
if shared_hits:
    problems = True
    print("\n    ABORT: state contains account-global names that another project may own:")
    for addr, ident in shared_hits:
        print(f"      {addr}  ->  {ident}")

if problems:
    print("\n    Destroying would remove infrastructure this project does not own.")
    print("    Remove the offending entries from state first, e.g.:")
    print("      terraform state rm '<address>'")
    print("    (state rm changes nothing in AWS — it only drops the bookkeeping entry.)")
    sys.exit(2)

print("    clean: every tagged resource belongs to this project")
PY

# --- the CI lambda archive must exist, or destroy cannot even plan --------------
# data "archive_file" is evaluated during destroy; without bootstrap it errors.
LAMBDA_DIR="$PROJECT/infrastructure/modules/workloads/ci_lambda"
if [ -d "$LAMBDA_DIR" ] && [ ! -f "$LAMBDA_DIR/bootstrap" ]; then
	echo
	echo "==> building CI lambda bootstrap (required to evaluate archive_file)"
	(cd "$LAMBDA_DIR" && GOOS=linux GOARCH=arm64 go build -o bootstrap . >/dev/null 2>&1) \
		|| echo "    warning: build failed; destroy may report an archive error"
fi

# --- dry run unless --yes ------------------------------------------------------
if [ "$CONFIRM" != "--yes" ]; then
	echo
	echo "==> DRY RUN (pass --yes to actually destroy)"
	terraform plan -destroy -no-color -refresh=false 2>&1 | grep -E "^Plan:|will be destroyed" | head -40
	echo
	echo "Nothing was changed. Re-run with --yes to destroy."
	exit 0
fi

echo
echo "==> destroying $PROJECT_NAME/$ENVNAME"
terraform destroy -auto-approve -no-color

# --- verify the environment really is empty ------------------------------------
echo
echo "==> verifying empty state"
REMAINING="$(terraform state list 2>/dev/null | wc -l | tr -d ' ')"
echo "    resources remaining in state: $REMAINING"
if [ "$REMAINING" != "0" ]; then
	echo "    WARNING: state is not empty — the next run will not be a clean first install." >&2
	terraform state list | head -20
	exit 1
fi

echo
echo "Environment is empty. The next deploy is a genuine first run."
echo "State backup kept at: $BACKUP"
