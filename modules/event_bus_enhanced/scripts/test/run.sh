#!/bin/sh
# Tests for ../revoke-subscribers.sh against the fake AWS CLI in ./fakebin.
# No AWS account, no network. Every id and ARN is synthetic: this account is
# 000000000000, other accounts are 0000000000NN.
#
# Run: sh modules/event_bus_enhanced/scripts/test/run.sh
#
# A case is: reset; seed the fake (bus, sub, src, account, parent); run the
# script with revoke VAR=value ...; then check with the expect_* helpers.

set -u

here=$(cd "$(dirname "$0")" && pwd)
script="$here/../revoke-subscribers.sh"
fakebin="$here/fakebin"

pass=0
fail=0
work=$(mktemp -d "${TMPDIR:-/tmp}/revoke-subscribers-test.XXXXXX")
trap 'rm -rf "$work"' EXIT

OWN=000000000000
REGION=ap-southeast-2
BUS="arn:aws:events:$REGION:$OWN:event-busv2/myapp-dev/0000000000000000000000000"
tab=$(printf '\t')

# reset: a fresh fake account with the bus $BUS and nothing attached to it.
reset() {
	FAKE_EBV2_DIR="$work/case.$pass.$fail.$$"
	rm -rf "$FAKE_EBV2_DIR"
	mkdir -p "$FAKE_EBV2_DIR/accounts" "$FAKE_EBV2_DIR/parents"
	printf '%s\n' "$BUS" >"$FAKE_EBV2_DIR/bus"
	export FAKE_EBV2_DIR
}

# sub ACCOUNT NAME [REVOKED]: a subscriber on the bus; prints nothing.
sub() { attach subscribers subscriber "$@"; }
# src ACCOUNT NAME [REVOKED]: an event source on the bus.
src() { attach event-sources event-sourcev2/aws.service "$@"; }
attach() {
	printf 'arn:aws:events:%s:%s:%s/%s/0000000000000000000000000%s%s%s%s%s%s\n' \
		"$REGION" "$3" "$2" "$4" "$tab" "$3" "$tab" "${5:-False}" "$tab" "$4" >>"$FAKE_EBV2_DIR/$1"
}
# arn_of KIND ACCOUNT NAME: the ARN sub/src gave that resource.
arn_of() {
	case "$1" in
	sub) printf 'arn:aws:events:%s:%s:subscriber/%s/0000000000000000000000000' "$REGION" "$2" "$3" ;;
	src) printf 'arn:aws:events:%s:%s:event-sourcev2/aws.service/%s/0000000000000000000000000' "$REGION" "$2" "$3" ;;
	esac
}
# account ID member|absent|denied: what describe-account answers.
account() { printf '%s\n' "$2" >"$FAKE_EBV2_DIR/accounts/$1"; }
# parent CHILD PARENT TYPE | parent CHILD absent|denied: what list-parents answers.
parent() {
	if [ $# -eq 3 ]; then
		printf '%s\t%s\n' "$2" "$3" >"$FAKE_EBV2_DIR/parents/$1"
	else
		printf '%s\n' "$2" >"$FAKE_EBV2_DIR/parents/$1"
	fi
}
no_bus() { rm -f "$FAKE_EBV2_DIR/bus"; }
revoke_error() { printf '%s\n' "$1" >"$FAKE_EBV2_DIR/revoke_error"; }

# revoke [VAR=value ...]: run the script in teardown mode on $BUS unless the
# arguments override it (FAKE_AWS_VERSION too: the environment is cleared);
# sets rc and log (stdout and stderr together).
revoke() {
	log=$(env -i PATH="$fakebin:/usr/bin:/bin" FAKE_EBV2_DIR="$FAKE_EBV2_DIR" \
		TMPDIR="$work" EBV2_MODE=teardown EBV2_BUS_ARN="$BUS" EBV2_OWN_ACCOUNT="$OWN" \
		EBV2_PROJECT=myapp AWS_REGION="$REGION" "$@" sh "$script" 2>&1)
	rc=$?
}

calls() { cat "$FAKE_EBV2_DIR/calls.log" 2>/dev/null; }
revoked_flag() { grep -F "$1$tab" "$FAKE_EBV2_DIR/subscribers" "$FAKE_EBV2_DIR/event-sources" 2>/dev/null | cut -f 3; }

ok() {
	pass=$((pass + 1))
	printf 'ok   %s\n' "$1"
}
bad() {
	fail=$((fail + 1))
	printf 'FAIL %s\n' "$1"
	shift
	for line in "$@"; do printf '     %s\n' "$line"; done
}

# expect CASE RC [TEXT...]: the exit status is RC and the output holds every TEXT.
expect() {
	name=$1 want=$2
	shift 2
	if [ "$rc" != "$want" ]; then
		bad "$name" "rc=$rc, want $want" "log: $log"
		return
	fi
	for t in "$@"; do
		case "$log" in
		*"$t"*) ;;
		*)
			bad "$name" "output lacks: $t" "log: $log"
			return
			;;
		esac
	done
	ok "$name"
}

# expect_revoked CASE ARN...: each ARN was revoked, and only those.
expect_revoked() {
	name=$1
	shift
	got=$(calls | sed -n 's/^revoke-resource //p' | sort)
	want=$(for a in "$@"; do printf '%s\n' "$a"; done | sed '/^$/d' | sort)
	if [ "$got" = "$want" ]; then ok "$name"; else bad "$name" "revoked: $(echo "$got" | tr '\n' ' ')" "want:    $(echo "$want" | tr '\n' ' ')" "log: $log"; fi
}

# --- smoke -------------------------------------------------------------------

reset
sub "$OWN" myapp-events-orders.created-dev
sub 000000000001 partner-audit
revoke EBV2_MODE=teardown
expect "teardown revokes another account's subscriber and exits 0" 0 \
	"revoked $(arn_of sub 000000000001 partner-audit) (account 000000000001)"
expect_revoked "teardown leaves this account's subscriber alone" "$(arn_of sub 000000000001 partner-audit)"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
