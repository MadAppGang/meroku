#!/usr/bin/env bash
#
# dev-reset.sh — tear a meroku test environment back down, so the next run is a
# genuine clean first install.
#
# Why this exists: a half-finished apply leaves an environment that is neither
# empty nor complete, and testing against it proves nothing about a first-run
# experience. Worse, a failed run can leave *another project's* resource recorded
# in this environment's state (AWS upserts EventBridge rules rather than
# rejecting duplicate names), and a naive destroy would then delete
# infrastructure belonging to that other project.
#
# Usage — the project directory is inferred from the working directory:
#   ./reset <env>              dry run: scan and plan, change nothing
#   ./reset <env> --yes        destroy the AWS resources, keep the config
#   ./reset <env> --greenfield destroy everything, back to the create-env wizard
#   ./reset <env> --greenfield --force
#                              proceed with local cleanup even if the destroy
#                              could not run (see "orphan risk" below)
#
# The explicit form `./reset <project-dir> <env> ...` also works.
#
# --yes leaves the environment deployable again: the config, the generated
# terraform and the state bucket all survive, so the next deploy recreates the
# same stack. That is the right mode for testing a redeploy.
#
# --greenfield additionally removes the NS delegation record from the parent
# zone (in whichever account holds it, read from dns.yaml), the S3 state bucket,
# and <env>.yaml / dns.yaml / env/.
#
# Leaving the NS record behind is the subtle one: the next run creates a zone
# with *different* nameservers, so the stale record makes the preflight see a
# mismatched delegation and route to Blocked rather than Bootstrap. The deploy
# still works, so the discrepancy is invisible — you simply test a different code
# path than intended.
#
# TWO FAILURE POLICIES, DELIBERATELY DIFFERENT
#
# Steps that could destroy the wrong thing abort immediately: the cross-project
# safety scan, and any name fence. Nothing recovers from deleting another
# project's infrastructure, so those are hard stops.
#
# Everything else is best effort and reported. A teardown that stops at the first
# failed step is worse than useless — it leaves the environment in exactly the
# half-cleaned state it was meant to eliminate, and the operator has to work out
# by hand which of the six things it had got to. So each cleanup step runs
# independently, records its own outcome, and the summary at the end says what
# succeeded and what did not. The exit code reflects any failure.
#
# ORPHAN RISK
#
# If the state cannot be read, the destroy cannot run, and the config files are
# then the only local record of what was deployed. Deleting them would leave AWS
# resources running with nothing pointing at them. So --greenfield keeps the
# config in that case and tells you why. --force overrides, for when you already
# know the environment is empty.
#
# Backups go to $HOME/.meroku-dev-backups. Those files contain real
# infrastructure data and must never be committed anywhere.

set -uo pipefail # NOT -e: see "two failure policies" above.

# --- arguments -----------------------------------------------------------------
#
# Told apart by whether the first argument is a directory. An env name never is,
# and a project directory never doubles as an env name, so there is nothing
# ambiguous to resolve and no flag is needed.
if [ -d "${1:-}" ]; then
	PROJECT="$1"
	shift
else
	PROJECT="."
fi
ENVNAME="${1:-}"
shift || true

DESTROY=no
GREENFIELD=no
FORCE=no
for arg in "$@"; do
	case "$arg" in
		--yes) DESTROY=yes ;;
		--greenfield) DESTROY=yes; GREENFIELD=yes ;;
		--force) FORCE=yes ;;
		*) echo "unknown option: $arg" >&2; exit 1 ;;
	esac
done

if [ -z "$ENVNAME" ]; then
	cat >&2 <<-EOF
		usage: $0 [project-dir] <env> [--yes|--greenfield] [--force]

		  run from inside a project and the directory can be omitted:
		    $0 dev                 dry run
		    $0 dev --greenfield    full teardown
	EOF
	exit 1
fi

PROJECT="$(cd "$PROJECT" 2>/dev/null && pwd)" || { echo "no such directory" >&2; exit 1; }
ENVDIR="$PROJECT/env/$ENVNAME"
YAML="$PROJECT/$ENVNAME.yaml"

[ -f "$YAML" ] || { echo "no such config: $YAML" >&2; exit 1; }

# --- refuse to touch production ------------------------------------------------
if grep -qE '^is_prod:[[:space:]]*true' "$YAML"; then
	echo "REFUSING: $YAML has is_prod: true. This script is for test environments only." >&2
	exit 1
fi

yaml_get() { grep -E "^$1:" "$YAML" | head -1 | sed "s/^$1:[[:space:]]*//" | tr -d '"'"'"' '; }

PROJECT_NAME="$(yaml_get project)"
[ -n "$PROJECT_NAME" ] || { echo "could not read 'project:' from $YAML" >&2; exit 1; }

# --- credentials, the same way meroku resolves them ----------------------------
#
# meroku exports AWS_PROFILE from the environment's YAML at startup, so a project
# never needs it in the shell. This script has to do the same or it fails on the
# very first AWS call with "No valid credential sources found" — which reads like
# a terraform problem and is really just a missing export.
PROFILE="$(yaml_get aws_profile)"
REGION="$(yaml_get region)"
[ -n "$PROFILE" ] && export AWS_PROFILE="$PROFILE"
[ -n "$REGION" ] && { export AWS_REGION="$REGION" AWS_DEFAULT_REGION="$REGION"; }

echo "project : $PROJECT_NAME"
echo "env     : $ENVNAME"
echo "dir     : $ENVDIR"
echo "profile : ${PROFILE:-<none in yaml; using ambient>}"
echo

# Fail early and legibly if those credentials do not work, rather than letting
# every later step fail for the same reason one at a time.
if ! aws sts get-caller-identity >/dev/null 2>&1; then
	cat >&2 <<-EOF
		Cannot authenticate with profile '${PROFILE:-<ambient>}'.

		  aws sso login --profile ${PROFILE:-<profile>}

		Nothing was changed.
	EOF
	exit 1
fi

BACKUP_DIR="$HOME/.meroku-dev-backups"
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"

# --- outcome tracking ----------------------------------------------------------
FAILURES=0
SUMMARY=()
record() { # record <ok|skip|fail> <message>
	SUMMARY+=("$1|$2")
	[ "$1" = "fail" ] && FAILURES=$((FAILURES + 1))
	return 0
}

# --- can we reach the state? ---------------------------------------------------
#
# Every answer here is legitimate. The environment may never have been deployed,
# may have been destroyed already, or may have had env/ deleted by hand. None of
# those should stop the cleanup — they only change whether a destroy is possible.
STATE_OK=no
BACKUP=""
RESOURCE_COUNT=0

if [ ! -d "$ENVDIR" ]; then
	echo "==> no env/$ENVNAME directory — nothing generated here"
	record skip "terraform destroy (env/$ENVNAME does not exist)"
elif [ ! -d "$ENVDIR/.terraform" ]; then
	echo "==> env/$ENVNAME exists but terraform was never initialised"
	record skip "terraform destroy (not initialised)"
else
	BACKUP="$BACKUP_DIR/${PROJECT_NAME}-${ENVNAME}-${STAMP}.tfstate"
	if (cd "$ENVDIR" && terraform state pull) >"$BACKUP" 2>/dev/null && [ -s "$BACKUP" ]; then
		chmod 600 "$BACKUP"
		STATE_OK=yes
		RESOURCE_COUNT="$(python3 -c '
import json, sys
s = json.load(open(sys.argv[1]))
print(sum(len(r.get("instances", [])) for r in s.get("resources", [])))
' "$BACKUP" 2>/dev/null || echo 0)"
		echo "state backed up: $BACKUP  ($RESOURCE_COUNT resources)"
	else
		rm -f "$BACKUP"
		BACKUP=""
		echo "==> could not pull state (backend unreachable, or state deleted)"
		record fail "terraform destroy (state unreadable)"
	fi
fi

# --- cross-project safety scan -------------------------------------------------
# HARD ABORT. Nothing recovers from destroying another project's infrastructure.
if [ "$STATE_OK" = "yes" ] && [ "$RESOURCE_COUNT" != "0" ]; then
	echo
	echo "==> safety scan"
	if ! python3 - "$BACKUP" "$PROJECT_NAME" <<'PY'; then
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
		echo
		echo "Nothing was changed." >&2
		exit 2
	fi
fi

# --- the CI lambda archive must exist, or destroy cannot even plan --------------
# data "archive_file" is evaluated during destroy; without bootstrap it errors.
LAMBDA_DIR="$PROJECT/infrastructure/modules/workloads/ci_lambda"
if [ "$STATE_OK" = "yes" ] && [ -d "$LAMBDA_DIR" ] && [ ! -f "$LAMBDA_DIR/bootstrap" ]; then
	echo
	echo "==> building CI lambda bootstrap (required to evaluate archive_file)"
	(cd "$LAMBDA_DIR" && GOOS=linux GOARCH=arm64 go build -o bootstrap . >/dev/null 2>&1) ||
		echo "    warning: build failed; destroy may report an archive error"
fi

# --- dry run -------------------------------------------------------------------
if [ "$DESTROY" != "yes" ]; then
	echo
	echo "==> DRY RUN (pass --yes to destroy, --greenfield to also erase config/state/DNS)"
	if [ "$STATE_OK" = "yes" ]; then
		(cd "$ENVDIR" && terraform plan -destroy -no-color -refresh=false 2>&1) |
			grep -E "^Plan:|will be destroyed" | head -40
	else
		echo "    no readable state — there is nothing to destroy"
	fi

	echo
	echo "    --greenfield would additionally remove:"
	if [ -f "$PROJECT/dns.yaml" ]; then
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
	fi
	echo "      the S3 state bucket ($(yaml_get state_bucket))"
	echo "      $ENVNAME.yaml, dns.yaml, env/$ENVNAME"
	echo
	echo "Nothing was changed."
	exit 0
fi

# --- destroy -------------------------------------------------------------------
DESTROY_OK=no
if [ "$STATE_OK" != "yes" ]; then
	echo
	echo "==> skipping destroy (no readable state)"
elif [ "$RESOURCE_COUNT" = "0" ]; then
	echo
	echo "==> state is already empty, nothing to destroy"
	DESTROY_OK=yes
	record ok "terraform destroy (already empty)"
else
	echo
	echo "==> destroying $PROJECT_NAME/$ENVNAME"
	if (cd "$ENVDIR" && terraform destroy -auto-approve -no-color); then
		REMAINING="$( (cd "$ENVDIR" && terraform state list 2>/dev/null) | wc -l | tr -d ' ')"
		if [ "$REMAINING" = "0" ]; then
			DESTROY_OK=yes
			record ok "terraform destroy ($RESOURCE_COUNT resources)"
		else
			record fail "terraform destroy (left $REMAINING resources in state)"
			echo "    WARNING: $REMAINING resources remain in state" >&2
		fi
	else
		record fail "terraform destroy (apply failed)"
		echo "    WARNING: destroy failed; AWS resources may still exist" >&2
	fi
fi

if [ "$GREENFIELD" != "yes" ]; then
	echo
	if [ "$FAILURES" = "0" ]; then
		echo "Environment is empty. The next deploy recreates the same stack."
	else
		echo "Finished with $FAILURES problem(s) — see above." >&2
	fi
	[ -n "$BACKUP" ] && echo "State backup kept at: $BACKUP"
	exit "$([ "$FAILURES" = 0 ] && echo 0 || echo 1)"
fi

# --- greenfield ----------------------------------------------------------------
#
# From here every step is independent: one failure must not prevent the others
# from running, or the teardown leaves exactly the half-cleaned environment it
# exists to prevent.
echo
echo "==> greenfield teardown"

# Config files are the only local record of what was deployed. Erasing them while
# AWS resources may still exist orphans those resources with nothing pointing at
# them, so that combination needs an explicit --force.
ERASE_CONFIG=yes
if [ "$DESTROY_OK" != "yes" ] && [ "$FORCE" != "yes" ]; then
	ERASE_CONFIG=no
fi

# Keep copies regardless — this is the file that cannot be regenerated.
cp "$YAML" "$BACKUP_DIR/${PROJECT_NAME}-${ENVNAME}-${STAMP}.yaml" 2>/dev/null &&
	echo "    config backed up to $BACKUP_DIR"
[ -f "$PROJECT/dns.yaml" ] &&
	cp "$PROJECT/dns.yaml" "$BACKUP_DIR/${PROJECT_NAME}-dns-${STAMP}.yaml" 2>/dev/null

# 1. NS delegation record, in whichever account holds the parent zone.
#
# This writes to an account other than the one being torn down, so it is fenced
# hard: the record must be the exact subdomain recorded in dns.yaml, and it is
# deleted by feeding back the values AWS currently returns. Route53 rejects a
# DELETE whose values do not match the live record, so a record changed since we
# wrote it fails rather than being clobbered.
if [ -f "$PROJECT/dns.yaml" ]; then
	PARENT_PROFILE="$(grep -A3 '^parent_zones:' "$PROJECT/dns.yaml" | grep 'profile:' | head -1 | sed 's/.*profile:[[:space:]]*//')"
	PARENT_ZONE="$(grep -A4 '^parent_zones:' "$PROJECT/dns.yaml" | grep 'zone_id:' | head -1 | sed 's/.*zone_id:[[:space:]]*//')"
	SUBDOMAIN="$(grep -A2 '^delegated_zones:' "$PROJECT/dns.yaml" | grep 'subdomain:' | head -1 | sed 's/.*subdomain:[[:space:]]*//')"

	if [ -n "$PARENT_PROFILE" ] && [ -n "$PARENT_ZONE" ] && [ -n "$SUBDOMAIN" ]; then
		echo "    NS $SUBDOMAIN in $PARENT_ZONE (profile $PARENT_PROFILE)"
		LIVE="$(AWS_PROFILE="$PARENT_PROFILE" aws route53 list-resource-record-sets \
			--hosted-zone-id "$PARENT_ZONE" \
			--query "ResourceRecordSets[?Name=='${SUBDOMAIN}.' && Type=='NS'] | [0]" \
			--output json 2>/dev/null || echo null)"

		if [ "$LIVE" = "null" ] || [ -z "$LIVE" ]; then
			echo "      already gone"
			record ok "NS record (already absent)"
		else
			BATCH="$(python3 -c '
import json, sys
rr = json.load(sys.stdin)
print(json.dumps({"Changes": [{"Action": "DELETE", "ResourceRecordSet": rr}]}))
' <<<"$LIVE" 2>/dev/null)"
			if [ -n "$BATCH" ] && AWS_PROFILE="$PARENT_PROFILE" aws route53 change-resource-record-sets \
				--hosted-zone-id "$PARENT_ZONE" --change-batch "$BATCH" >/dev/null 2>&1; then
				echo "      deleted"
				record ok "NS record $SUBDOMAIN"
			else
				echo "      FAILED — remove it by hand" >&2
				record fail "NS record $SUBDOMAIN (delete failed)"
			fi
		fi
	else
		record skip "NS record (dns.yaml has no parent-zone entry)"
	fi
else
	record skip "NS record (no dns.yaml)"
fi

# 2. State bucket. meroku generates a fresh random suffix per environment, so
#    leaving it behind orphans one bucket per test cycle.
BUCKET="$(yaml_get state_bucket)"
if [ -z "$BUCKET" ]; then
	record skip "state bucket (none in $ENVNAME.yaml)"
elif [[ "$BUCKET" != *"$PROJECT_NAME"* ]]; then
	# Fence: only ever touch a bucket whose name carries this project's name.
	echo "    REFUSING to delete bucket '$BUCKET' — name does not contain '$PROJECT_NAME'" >&2
	record fail "state bucket $BUCKET (name fence)"
elif [ "$DESTROY_OK" != "yes" ] && [ "$FORCE" != "yes" ]; then
	echo "    keeping bucket $BUCKET — the destroy did not complete"
	record skip "state bucket $BUCKET (destroy incomplete)"
else
	echo "    bucket $BUCKET"
	aws s3 rm "s3://$BUCKET" --recursive >/dev/null 2>&1
	if aws s3api delete-bucket --bucket "$BUCKET" >/dev/null 2>&1; then
		echo "      deleted"
		record ok "state bucket $BUCKET"
	elif ! aws s3api head-bucket --bucket "$BUCKET" >/dev/null 2>&1; then
		echo "      already gone"
		record ok "state bucket (already absent)"
	else
		echo "      FAILED" >&2
		record fail "state bucket $BUCKET (delete failed)"
	fi
fi

# 3. Generated output. Always safe: it is derived from the config, and the real
#    state lives in S3, not here.
if [ -d "$ENVDIR" ]; then
	rm -rf "$ENVDIR" && record ok "env/$ENVNAME" || record fail "env/$ENVNAME (rm failed)"
	rmdir "$PROJECT/env" 2>/dev/null || true
else
	record skip "env/$ENVNAME (already absent)"
fi

# 4. Config. Last, because everything above reads it.
if [ "$ERASE_CONFIG" = "yes" ]; then
	rm -f "$YAML" "$PROJECT/dns.yaml"
	record ok "$ENVNAME.yaml, dns.yaml"
else
	record skip "$ENVNAME.yaml, dns.yaml (kept: destroy did not complete)"
fi

# --- summary -------------------------------------------------------------------
echo
echo "==> summary"
for line in "${SUMMARY[@]}"; do
	status="${line%%|*}"
	what="${line#*|}"
	case "$status" in
		ok) printf "    \033[32m✓\033[0m %s\n" "$what" ;;
		skip) printf "    \033[90m·\033[0m %s\n" "$what" ;;
		fail) printf "    \033[31m✗\033[0m %s\n" "$what" ;;
	esac
done

echo
if [ "$FAILURES" = "0" ] && [ "$ERASE_CONFIG" = "yes" ]; then
	echo "Greenfield. Nothing left on disk or in AWS for $PROJECT_NAME/$ENVNAME."
	echo "Next: cd $PROJECT && ./meroku  ->  Create new environment"
elif [ "$ERASE_CONFIG" != "yes" ]; then
	cat >&2 <<-EOF
		Config kept on purpose: the destroy did not complete, so AWS resources may
		still exist and $ENVNAME.yaml is the only local record of them.

		  Fix the cause, then re-run:   ./reset $ENVNAME --greenfield
		  Or, if you know it is empty:  ./reset $ENVNAME --greenfield --force
	EOF
else
	echo "Finished with $FAILURES problem(s) — see the summary above." >&2
fi
echo "Backups in $BACKUP_DIR"

exit "$([ "$FAILURES" = 0 ] && echo 0 || echo 1)"
