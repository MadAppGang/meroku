#!/bin/sh
# Tests for ../route53-txt.sh against the fake AWS CLI in ./fakebin.
# No AWS account, no network. Fixtures are synthetic.
#
# Run: sh modules/ses/scripts/test/run.sh

set -u

here=$(cd "$(dirname "$0")" && pwd)
script="$here/../route53-txt.sh"
fakebin="$here/fakebin"

pass=0
fail=0
work=$(mktemp -d "${TMPDIR:-/tmp}/route53-txt-test.XXXXXX")
trap 'rm -rf "$work"' EXIT

Z=Z000000000000A
SPF_GOOGLE='"v=spf1 include:_spf.google.com ~all"'
SPF_SES='"v=spf1 include:amazonses.com ~all"'
SPF_BOTH='"v=spf1 include:_spf.google.com include:amazonses.com ~all"'
SITE='"google-site-verification=fake-token-do-not-use"'

# reset: a fresh fake Route53 for the next case.
reset() {
	FAKE_R53_DIR="$work/r53.$pass.$fail.$$"
	rm -rf "$FAKE_R53_DIR"
	mkdir -p "$FAKE_R53_DIR/$Z"
	export FAKE_R53_DIR
}

# seed NAME TTL VALUE...: write a live record set.
seed() {
	n=$1 t=$2
	shift 2
	{
		echo "$t"
		for v in "$@"; do printf '%s\n' "$v"; done
	} >"$FAKE_R53_DIR/$Z/$n.TXT"
}

# r53 OP NAME VALUE [SLOT] [TTL]: run the script; sets rc and log.
r53() {
	log=$(env PATH="$fakebin:$PATH" R53_RETRY_SLEEP=0 \
		R53_ZONE_ID="/hostedzone/$Z" R53_NAME="$2" R53_VALUE="$3" \
		R53_SLOT="${4:-}" R53_TTL="${5:-}" AWS_REGION=us-east-1 \
		sh "$script" "$1" 2>&1)
	rc=$?
}

live() { cat "$FAKE_R53_DIR/$Z/$1.TXT" 2>/dev/null; }
changes() { grep -c '^change ' "$FAKE_R53_DIR/calls.log" 2>/dev/null || true; }

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

# expect_live CASE NAME EXPECTED (newline-separated, TTL first, values in any order).
expect_live() {
	got=$(live "$2" | { read -r t && echo "$t" && sort; })
	want=$(printf '%s\n' "$3" | { read -r t && echo "$t" && sort; })
	if [ "$rc" = 0 ] && [ "$got" = "$want" ]; then
		ok "$1"
	else
		bad "$1" "rc=$rc" "want: $(echo "$want" | tr '\n' '|')" "got:  $(echo "$got" | tr '\n' '|')" "log: $log"
	fi
}

expect_absent() {
	if [ "$rc" = 0 ] && [ ! -e "$FAKE_R53_DIR/$Z/$2.TXT" ]; then ok "$1"; else bad "$1" "rc=$rc" "live: $(live "$2" | tr '\n' '|')" "log: $log"; fi
}

expect_changes() {
	n=$(changes)
	if [ "$n" = "$2" ]; then ok "$1"; else bad "$1" "change calls: $n, want $2" "log: $log"; fi
}

expect_rc() {
	if [ "$rc" = "$2" ]; then ok "$1"; else bad "$1" "rc=$rc, want $2" "log: $log"; fi
}

nl='
'

# --- add -------------------------------------------------------------------

reset
r53 add example.com "$SPF_SES" spf 300
expect_live "add to an absent set creates it with the given TTL" example.com "300$nl$SPF_SES"

reset
r53 add example.com "$SITE" "" ""
expect_live "add to an absent set without a TTL uses 600" example.com "600$nl$SITE"

reset
seed example.com 3600 "$SPF_GOOGLE" "$SITE"
r53 add example.com '"x-owned=1"' "" 600
expect_live "add keeps foreign values and the live TTL" example.com "3600$nl$SPF_GOOGLE$nl$SITE$nl\"x-owned=1\""

reset
seed example.com 3600 "$SPF_GOOGLE" "$SITE" '"V=SPF1 include:old.example.com -all"'
r53 add example.com "$SPF_BOTH" spf 600
expect_live "slot add replaces every SPF value, any case, keeps the rest" example.com "3600$nl$SPF_BOTH$nl$SITE"

reset
seed example.com 3600 '"v=spf1 include:_spf.google.com" " ~all"' "$SITE"
r53 add example.com "$SPF_BOTH" spf 600
expect_live "slot add recognises an SPF value split into parts" example.com "3600$nl$SPF_BOTH$nl$SITE"

reset
seed example.com 3600 "$SPF_BOTH" "$SITE"
r53 add example.com "$SPF_BOTH" spf 600
expect_live "slot add of the value already there leaves the set alone" example.com "3600$nl$SPF_BOTH$nl$SITE"
expect_changes "slot add of the value already there makes no change call" 0

reset
seed example.com 600 "$SITE"
r53 add example.com "$SITE" "" 600
expect_changes "add of a value already present makes no change call" 0

reset
seed _dmarc.example.com 300 '"v=DMARC1; p=reject; rua=mailto:sec@example.com"' '"unrelated=1"'
r53 add _dmarc.example.com '"v=DMARC1; p=none; pct=100; rua=mailto:dmarc-reports@example.com"' dmarc 300
expect_live "dmarc slot add replaces the DMARC value only" _dmarc.example.com "300$nl\"v=DMARC1; p=none; pct=100; rua=mailto:dmarc-reports@example.com\"$nl\"unrelated=1\""

reset
seed example.com 600 "$SPF_GOOGLE"
r53 add example.com '"v=DMARC1; p=none"' "" 600
expect_live "add without a slot never removes an SPF value" example.com "600$nl$SPF_GOOGLE$nl\"v=DMARC1; p=none\""

# --- values that need escaping ----------------------------------------------

LONG='"v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAfakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefakefake" "fakefakefakefakefakefakefakefakefakefakeIDAQAB"'
reset
r53 add google._domainkey.example.com "$LONG" "" 3600
expect_live "a split value keeps its delimiter" google._domainkey.example.com "3600$nl$LONG"

reset
ESC='"caf\344 \\ back\\slash"'
r53 add example.com "$ESC" "" 600
expect_live "backslash escapes reach Route53 unchanged" example.com "600$nl$ESC"
r53 remove example.com "$ESC"
expect_absent "a value with backslashes is matched exactly on remove" example.com

reset
# shellcheck disable=SC2016 # literal shell syntax, on purpose
INJ='"$(touch pwned) `touch pwned2` ; touch pwned3"'
( cd "$work" && r53 add example.com "$INJ" "" 600 )
if [ ! -e "$work/pwned" ] && [ ! -e "$work/pwned2" ] && [ ! -e "$work/pwned3" ] && [ ! -e pwned ]; then
	ok "a value is never executed"
else
	bad "a value is never executed" "a pwned file exists"
fi
r53 add example.com "$INJ" "" 600
expect_live "a value with shell syntax is stored as text" example.com "600$nl$INJ"

# --- remove ------------------------------------------------------------------

reset
seed example.com 3600 "$SPF_BOTH" "$SITE" '"x-owned=1"'
r53 remove example.com '"x-owned=1"'
expect_live "remove takes out only that value" example.com "3600$nl$SPF_BOTH$nl$SITE"

reset
seed example.com 3600 '"x-owned=1"'
r53 remove example.com '"x-owned=1"'
expect_absent "remove of the last value deletes the set" example.com

reset
seed example.com 3600 "$SITE"
r53 remove example.com '"x-owned=1"'
expect_live "remove of a value that is not there leaves the set alone" example.com "3600$nl$SITE"
expect_changes "remove of a value that is not there makes no change call" 0

reset
r53 remove example.com '"x-owned=1"'
expect_rc "remove from an absent set succeeds" 0
expect_changes "remove from an absent set makes no change call" 0

reset
seed example.com 3600 '"x-owned=10"' '"x-owned=1"'
r53 remove example.com '"x-owned=1"'
expect_live "remove matches whole values, not prefixes" example.com "3600$nl\"x-owned=10\""

# --- ttl ---------------------------------------------------------------------

reset
seed example.com 3600 "$SPF_BOTH" "$SITE"
r53 ttl example.com "" "" 300
expect_live "ttl changes the TTL and keeps every value" example.com "300$nl$SPF_BOTH$nl$SITE"

reset
seed example.com 300 "$SITE"
r53 ttl example.com "" "" 300
expect_changes "ttl already right makes no change call" 0

reset
r53 ttl example.com "" "" 300
expect_rc "ttl of an absent set is a no-op" 0
expect_absent "ttl of an absent set creates nothing" example.com

# --- concurrency and conflicts ----------------------------------------------

reset
seed example.com 3600 "$SITE"
for i in 1 2 3 4 5; do
	env PATH="$fakebin:$PATH" R53_RETRY_SLEEP=0 R53_ZONE_ID="$Z" R53_NAME=example.com \
		R53_VALUE="\"parallel=$i\"" R53_SLOT= R53_TTL= AWS_REGION=us-east-1 \
		sh "$script" add >"$work/par.$i.log" 2>&1 &
done
wait
rc=0
log=$(cat "$work"/par.*.log)
expect_live "five parallel adds to one set all land" example.com "3600$nl$SITE$nl\"parallel=1\"$nl\"parallel=2\"$nl\"parallel=3\"$nl\"parallel=4\"$nl\"parallel=5\""

reset
seed example.com 600 "$SITE"
echo 3 >"$FAKE_R53_DIR/fail_next"
r53 add example.com '"x-owned=1"' "" 600
expect_live "a conflict is retried until the write lands" example.com "600$nl$SITE$nl\"x-owned=1\""

reset
seed example.com 600 "$SITE"
echo 99 >"$FAKE_R53_DIR/fail_next"
r53 add example.com '"x-owned=1"' "" 600
expect_rc "conflicts beyond six attempts exit 4" 4
expect_changes "it gives up after six change calls" 6

# --- environment and usage ---------------------------------------------------

reset
log=$(env PATH="/usr/bin:/bin" R53_ZONE_ID="$Z" R53_NAME=example.com R53_VALUE="$SITE" sh "$script" add 2>&1)
rc=$?
expect_rc "a missing aws CLI exits 3" 3

reset
log=$(env PATH="$fakebin:$PATH" FAKE_AWS_VERSION="aws-cli/1.29.0 Python/3.11" R53_ZONE_ID="$Z" R53_NAME=example.com R53_VALUE="$SITE" sh "$script" add 2>&1)
rc=$?
expect_rc "AWS CLI version 1 exits 3" 3

reset
r53 frobnicate example.com "$SITE"
expect_rc "an unknown operation exits 2" 2
r53 add example.com 'unquoted' "" 600
expect_rc "a value not in Route53 form exits 2" 2
r53 add 'example.com;rm' "$SITE" "" 600
expect_rc "a name with shell characters exits 2" 2
r53 add example.com "$SITE" bogus 600
expect_rc "an unknown slot exits 2" 2

reset
mkdir -p "$work/badbin"
# shellcheck disable=SC2016 # a script body, written verbatim
printf '#!/bin/sh\ncase "$1" in --version) echo aws-cli/2.17.0;; *) printf "%%s\\n" "garbage output";; esac\n' >"$work/badbin/aws"
chmod +x "$work/badbin/aws"
log=$(env PATH="$work/badbin:$PATH" R53_ZONE_ID="$Z" R53_NAME=example.com R53_VALUE="$SITE" sh "$script" add 2>&1)
rc=$?
expect_rc "output it cannot read is refused with exit 5, nothing written" 5

reset
# shellcheck disable=SC2016 # a script body, written verbatim
printf '#!/bin/sh\ncase "$1" in --version) echo aws-cli/2.17.0;; *) echo "An error occurred (AccessDenied) when calling the ListResourceRecordSets operation" >&2; exit 254;; esac\n' >"$work/badbin/aws"
log=$(env PATH="$work/badbin:$PATH" R53_ZONE_ID="$Z" R53_NAME=example.com R53_VALUE="$SITE" sh "$script" add 2>&1)
rc=$?
expect_rc "an AWS error that is not a conflict exits 5" 5

echo
echo "$pass passed, $fail failed"
[ "$fail" = 0 ]
