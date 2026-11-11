#!/bin/sh
# Tests for ../asg-capacity-wait.sh against the fake AWS CLI in ./fakebin.
# No AWS account, no network. Fixtures are synthetic.
#
# Run: sh modules/workloads/scripts/test/run.sh
#
# Each check's condition is a single-quoted string that `check` evals later,
# after the run has set rc and log.
# shellcheck disable=SC2016

set -u

here=$(cd "$(dirname "$0")" && pwd)
script="$here/../asg-capacity-wait.sh"
fakebin="$here/fakebin"

pass=0
fail=0
work=$(mktemp -d "${TMPDIR:-/tmp}/asg-capacity-wait-test.XXXXXX")
trap 'rm -rf "$work"' EXIT

# reset COUNT...: a fresh fake ASG whose describe calls return these counts.
reset() {
	FAKE_ASG_DIR="$work/asg.$pass.$fail"
	rm -rf "$FAKE_ASG_DIR"
	mkdir -p "$FAKE_ASG_DIR"
	: >"$FAKE_ASG_DIR/activity"
	for n in "$@"; do echo "$n"; done >"$FAKE_ASG_DIR/inservice"
	export FAKE_ASG_DIR
}

activity() { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >"$FAKE_ASG_DIR/activity"; }

# wait_asg MIN [TIMEOUT]: run the script; sets rc and log.
wait_asg() {
	log=$(env PATH="$fakebin:$PATH" ASG_NAME=acme_general_dev ASG_MIN_IN_SERVICE="$1" \
		ASG_TIMEOUT="${2:-30}" ASG_POLL_INTERVAL=0 AWS_REGION=us-east-1 \
		sh "$script" 2>&1)
	rc=$?
}

ok() {
	pass=$((pass + 1))
	printf 'ok   %s\n' "$1"
}
bad() {
	fail=$((fail + 1))
	printf 'FAIL %s (exit %s)\n' "$1" "$rc"
	printf '%s\n' "$log" | sed 's/^/     /'
}
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }
has() {
	case "$log" in *"$1"*) return 0 ;; esac
	return 1
}
calls() { grep -c "^$1\$" "$FAKE_ASG_DIR/calls.log" 2>/dev/null || true; }

# The deploy that failed: the first launch is refused, the ASG's retry works.
reset 0 0 1
activity a-1 Failed "Authentication Failure. Launching EC2 instance failed."
wait_asg 1
check "a failed first launch is reported and waited out" \
	'[ "$rc" -eq 0 ] && has "Authentication Failure" && has "1 of 1"'

reset 2
wait_asg 2
check "returns at once when enough instances are in service" \
	'[ "$rc" -eq 0 ] && [ "$(calls groups)" -eq 1 ] && [ "$(calls activities)" -eq 0 ]'

reset 0 0 0 0 1
activity a-7 Failed "We currently do not have sufficient t3.small capacity"
wait_asg 1
check "the same failed activity is printed once" \
	'[ "$rc" -eq 0 ] && [ "$(printf "%s\n" "$log" | grep -c "sufficient t3.small")" -eq 1 ]'

reset ERR 1
wait_asg 1
check "a failed describe call is retried" '[ "$rc" -eq 0 ]'

reset 0
activity a-9 Failed "The image id '[ami-00000000000000000]' does not exist"
wait_asg 1 0
check "times out with exit 4 naming the ASG and the last failure" \
	'[ "$rc" -eq 4 ] && has "does not exist" && has "acme_general_dev"'

reset 0
wait_asg 1 0
check "a timeout with no failed activity says so" \
	'[ "$rc" -eq 4 ] && has "no launch has failed"'

reset 0 1
activity a-2 Successful "Launching a new EC2 instance: i-00000000000000000"
wait_asg 1
check "a successful activity is not printed as a failure" \
	'[ "$rc" -eq 0 ] && ! has "launch failed"'

reset 0
wait_asg 0
check "min 0 makes no AWS call" '[ "$rc" -eq 0 ] && [ ! -e "$FAKE_ASG_DIR/calls.log" ]'

reset 1
log=$(env PATH="$fakebin:$PATH" ASG_NAME= ASG_MIN_IN_SERVICE=1 sh "$script" 2>&1)
rc=$?
check "an empty ASG_NAME is a usage error" '[ "$rc" -eq 2 ]'

log=$(env PATH="$fakebin:$PATH" ASG_NAME=x ASG_MIN_IN_SERVICE=one sh "$script" 2>&1)
rc=$?
check "a non-numeric minimum is a usage error" '[ "$rc" -eq 2 ]'

reset 1
log=$(env PATH="$fakebin:$PATH" FAKE_AWS_VERSION="aws-cli/1.29.0 Python/3.11" \
	ASG_NAME=x ASG_MIN_IN_SERVICE=1 ASG_POLL_INTERVAL=0 sh "$script" 2>&1)
rc=$?
check "AWS CLI v1 is refused with exit 3" '[ "$rc" -eq 3 ]'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
