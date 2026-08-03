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
#   scripts/dev-reset.sh <project-dir> <env>              # dry run: scan + plan only
#   scripts/dev-reset.sh <project-dir> <env> --yes        # destroy the AWS resources
#   scripts/dev-reset.sh <project-dir> <env> --greenfield # ...and erase every trace,
#                                                         # so the next run starts at
#                                                         # the "create environment"
#                                                         # wizard with nothing on disk
#
# --yes leaves the environment deployable again: the config, the generated
# terraform and the state bucket all survive, so the next deploy recreates the
# same stack. That is the right mode for testing a redeploy.
#
# --greenfield additionally removes:
#   - the NS delegation record from the parent zone (in whichever account holds
#     it, read from dns.yaml)
#   - the S3 state bucket
#   - <env>.yaml, dns.yaml and env/
#
# Use --greenfield when the thing being tested is the first-run experience
# itself. Leaving the NS record behind is the subtle one: the next run creates a
# zone with *different* nameservers, so the stale record makes the preflight see
# a mismatched delegation and route to Blocked rather than Bootstrap. The deploy
# still works, but it is no longer the code path you meant to test.
#
# Always backs up the state to $HOME/.meroku-dev-backups first, and in
# --greenfield mode backs up the YAML configs alongside it. Those files contain
# real infrastructure data and must never be committed anywhere.

set -euo pipefail

PROJECT="${1:-}"
ENVNAME="${2:-}"
CONFIRM="${3:-}"

if [ -z "$PROJECT" ] || [ -z "$ENVNAME" ]; then
	echo "usage: $0 <project-dir> <env> [--yes|--greenfield]" >&2
	exit 1
fi

case "$CONFIRM" in
	"" | --yes | --greenfield) ;;
	*) echo "unknown option: $CONFIRM (expected --yes or --greenfield)" >&2; exit 1 ;;
esac

# --greenfield implies --yes; everything below that destroys keys off DESTROY.
DESTROY=no
GREENFIELD=no
[ "$CONFIRM" = "--yes" ] && DESTROY=yes
[ "$CONFIRM" = "--greenfield" ] && { DESTROY=yes; GREENFIELD=yes; }

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
if [ "$DESTROY" != "yes" ]; then
	echo
	echo "==> DRY RUN (pass --yes to destroy, --greenfield to also erase config/state/DNS)"
	terraform plan -destroy -no-color -refresh=false 2>&1 | grep -E "^Plan:|will be destroyed" | head -40
	if [ "$GREENFIELD" = "no" ] && [ -f "$PROJECT/dns.yaml" ]; then
		echo
		echo "    --greenfield would additionally remove:"
		python3 - "$PROJECT/dns.yaml" 2>/dev/null <<'PY' || true
import sys, re
# Deliberately regex rather than a yaml import: this script must run with a bare
# python3, and the two fields needed are unambiguous single-line scalars.
text = open(sys.argv[1]).read()
sub = re.search(r'^\s*-?\s*subdomain:\s*(\S+)', text, re.M)
prof = re.search(r'^\s*profile:\s*(\S+)', text, re.M)
if sub and prof:
    print(f"      NS record {sub.group(1)} from the parent zone (profile {prof.group(1)})")
PY
		echo "      the S3 state bucket"
		echo "      $ENVNAME.yaml, dns.yaml, env/"
	fi
	echo
	echo "Nothing was changed."
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

if [ "$GREENFIELD" != "yes" ]; then
	echo
	echo "Environment is empty. The next deploy recreates the same stack."
	echo "State backup kept at: $BACKUP"
	exit 0
fi

# --- greenfield: erase every remaining trace -----------------------------------
echo
echo "==> greenfield teardown"

# Keep the configs; they are the only files here that cannot be regenerated.
cp "$YAML" "$BACKUP_DIR/${PROJECT_NAME}-${ENVNAME}-$(date +%Y%m%d-%H%M%S).yaml"
[ -f "$PROJECT/dns.yaml" ] &&
	cp "$PROJECT/dns.yaml" "$BACKUP_DIR/${PROJECT_NAME}-dns-$(date +%Y%m%d-%H%M%S).yaml"
echo "    configs backed up to $BACKUP_DIR"

# 1. NS delegation record, in whichever account holds the parent zone.
#
# This writes to an account other than the one being torn down, so it is fenced
# hard: the record must be the exact subdomain recorded in dns.yaml, and it is
# deleted by feeding back the values AWS currently returns. Route53 rejects a
# DELETE whose values do not match the live record, so a record that has been
# changed since we wrote it fails rather than being clobbered.
if [ -f "$PROJECT/dns.yaml" ]; then
	PARENT_PROFILE="$(grep -A3 '^parent_zones:' "$PROJECT/dns.yaml" | grep 'profile:' | head -1 | sed 's/.*profile:[[:space:]]*//')"
	PARENT_ZONE="$(grep -A4 '^parent_zones:' "$PROJECT/dns.yaml" | grep 'zone_id:' | head -1 | sed 's/.*zone_id:[[:space:]]*//')"
	SUBDOMAIN="$(grep -A2 '^delegated_zones:' "$PROJECT/dns.yaml" | grep 'subdomain:' | head -1 | sed 's/.*subdomain:[[:space:]]*//')"

	if [ -n "$PARENT_PROFILE" ] && [ -n "$PARENT_ZONE" ] && [ -n "$SUBDOMAIN" ]; then
		echo "    removing NS $SUBDOMAIN from $PARENT_ZONE (profile $PARENT_PROFILE)"
		LIVE="$(AWS_PROFILE="$PARENT_PROFILE" aws route53 list-resource-record-sets \
			--hosted-zone-id "$PARENT_ZONE" \
			--query "ResourceRecordSets[?Name=='${SUBDOMAIN}.' && Type=='NS'] | [0]" \
			--output json 2>/dev/null || echo null)"

		if [ "$LIVE" = "null" ] || [ -z "$LIVE" ]; then
			echo "    (no such record — nothing to remove)"
		else
			BATCH="$(python3 -c '
import json, sys
rr = json.load(sys.stdin)
print(json.dumps({"Changes": [{"Action": "DELETE", "ResourceRecordSet": rr}]}))
' <<<"$LIVE")"
			if AWS_PROFILE="$PARENT_PROFILE" aws route53 change-resource-record-sets \
				--hosted-zone-id "$PARENT_ZONE" --change-batch "$BATCH" >/dev/null 2>&1; then
				echo "    NS record deleted"
			else
				echo "    WARNING: could not delete the NS record — remove it by hand" >&2
			fi
		fi
	else
		echo "    (dns.yaml has no parent-zone record; skipping NS cleanup)"
	fi
fi

# 2. State bucket. meroku generates a fresh random suffix per environment, so
#    leaving it behind orphans one bucket per test cycle.
BUCKET="$(grep -E '^state_bucket:' "$YAML" | head -1 | sed 's/^state_bucket:[[:space:]]*//' | tr -d '"'"'"' ')"
if [ -n "$BUCKET" ]; then
	# Fence: only ever touch a bucket whose name carries this project's name.
	if [[ "$BUCKET" != *"$PROJECT_NAME"* ]]; then
		echo "    REFUSING to delete bucket '$BUCKET' — name does not contain '$PROJECT_NAME'" >&2
	else
		echo "    deleting state bucket $BUCKET"
		aws s3 rm "s3://$BUCKET" --recursive >/dev/null 2>&1 || true
		aws s3api delete-bucket --bucket "$BUCKET" >/dev/null 2>&1 &&
			echo "    bucket deleted" ||
			echo "    (bucket already gone or not empty)"
	fi
fi

# 3. Generated + config files. Done last: everything above reads them.
cd "$PROJECT"
rm -rf "env/$ENVNAME"
rmdir env 2>/dev/null || true
rm -f "$ENVNAME.yaml" dns.yaml
echo "    removed env/$ENVNAME, $ENVNAME.yaml, dns.yaml"

echo
echo "Greenfield. Nothing left on disk or in AWS for $PROJECT_NAME/$ENVNAME."
echo "Next: cd $PROJECT && ./meroku  ->  Create new environment"
echo "Backups kept in $BACKUP_DIR"
