#!/bin/sh
# Runs tests/lifecycle against the fake AWS CLI and checks what the final
# destroy leaves behind. Needs `terraform init -backend=false` in modules/ses
# first. No AWS account, no network.
#
#   timeout 600 sh modules/ses/tests/lifecycle/run.sh [terraform-binary]

set -u

here=$(cd "$(dirname "$0")" && pwd)
module=$(cd "$here/../.." && pwd)
tf=${1:-terraform}

FAKE_R53_DIR="$here/.r53"
export FAKE_R53_DIR
rm -rf "$FAKE_R53_DIR"
zone="$FAKE_R53_DIR/Z000000000000A"
mkdir -p "$zone"

printf '%s\n' 3600 '"v=spf1 include:_spf.google.com ~all"' '"google-site-verification=fake-token-do-not-use"' >"$zone/example.com.TXT"
printf '%s\n' 3600 '"v=DMARC1; p=reject; rua=mailto:sec@example.com"' >"$zone/_dmarc.example.com.TXT"
printf '%s\n' 120 '"foreign=1"' >"$zone/_acme.example.com.TXT"

PATH="$module/scripts/test/fakebin:$PATH"
R53_RETRY_SLEEP=0
export PATH R53_RETRY_SLEEP

cd "$module" || exit 1
"$tf" test -test-directory=tests/lifecycle -no-color
rc=$?

fail=0
check() {
	if [ "$(cat "$zone/$1" 2>/dev/null)" = "$2" ]; then
		echo "ok   after destroy: $1"
	else
		echo "FAIL after destroy: $1"
		echo "     want: $(printf '%s' "$2" | tr '\n' '|')"
		echo "     got:  $(tr '\n' '|' <"$zone/$1" 2>/dev/null)"
		fail=1
	fi
}
absent() {
	if [ -e "$zone/$1" ]; then
		echo "FAIL after destroy: $1 must be gone, holds: $(tr '\n' '|' <"$zone/$1")"
		fail=1
	else
		echo "ok   after destroy: $1 gone"
	fi
}

# The apex keeps its foreign verification at the TTL meroku set; meroku's SPF
# is gone, and so is the foreign SPF it replaced (destroy does not restore it).
check example.com.TXT "300
\"google-site-verification=fake-token-do-not-use\""
# Same for the DMARC policy meroku replaced: nothing foreign was left.
absent _dmarc.example.com.TXT
check _acme.example.com.TXT "120
\"foreign=1\""
absent _amazonses.example.com.TXT

rm -rf "$FAKE_R53_DIR"
[ "$rc" = 0 ] && [ "$fail" = 0 ]
