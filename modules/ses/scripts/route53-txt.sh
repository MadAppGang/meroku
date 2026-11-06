#!/bin/sh
# Adds, removes or re-times ONE value of a Route53 TXT record set, keeping
# every other value in the set. modules/ses runs it from terraform_data
# provisioners for zones meroku did not create, where the record set may hold
# values somebody else wrote (a Google site verification, another SPF include).
#
#   route53-txt.sh add|remove|ttl
#
# Input comes only from the environment, never from arguments or the shell
# command line, so no value is ever parsed by a shell:
#   R53_ZONE_ID  hosted zone id (a leading /hostedzone/ is stripped)
#   R53_NAME     record name, without the trailing dot
#   R53_VALUE    one value in Route53 form: "part1" "part2"     (add, remove)
#   R53_SLOT     "" | spf | dmarc                                (add)
#                With a slot, add also removes every other value of that
#                kind, so the name never publishes two SPF or two DMARC
#                records.
#   R53_TTL      seconds. add: TTL of a set it has to create (default 600);
#                an existing set keeps its TTL. ttl: the TTL to set.
#   AWS_REGION   any region; Route53 is global but the CLI needs one.
#
# Credentials are whatever the AWS CLI resolves from the environment it
# inherits, the same chain the Terraform AWS provider uses. No --profile is
# passed on purpose: the provider block meroku generates has none either.
#
# Every write is a compare-and-swap: DELETE of the exact set just read plus
# CREATE of the new set, in one change batch. Route53 refuses the batch if the
# set changed since the read, and the script then reads again and retries.
#
# Exit status: 0 done or nothing to do, 2 usage, 3 aws CLI missing or not v2,
# 4 the set kept changing under us, 5 any other AWS error or an unreadable set.

set -u

op=${1:-}
prog=route53-txt

say() { printf '%s: %s\n' "$prog" "$*" >&2; }
die() { code=$1; shift; say "$*"; exit "$code"; }

case "$op" in
add | remove | ttl) ;;
*) die 2 "usage: route53-txt.sh add|remove|ttl (inputs in R53_ZONE_ID, R53_NAME, R53_VALUE, R53_SLOT, R53_TTL)" ;;
esac

zone=${R53_ZONE_ID:-}
zone=${zone#/hostedzone/}
name=$(printf '%s' "${R53_NAME:-}" | tr 'ABCDEFGHIJKLMNOPQRSTUVWXYZ' 'abcdefghijklmnopqrstuvwxyz')
name=${name%.}
value=${R53_VALUE:-}
slot=${R53_SLOT:-}
want_ttl=${R53_TTL:-}

[ -n "$zone" ] || die 2 "R53_ZONE_ID is empty"
[ -n "$name" ] || die 2 "R53_NAME is empty"
case "$name" in
*[!a-z0-9_.-]*) die 2 "R53_NAME '$name' holds a character outside a-z 0-9 _ . -" ;;
esac
if [ "$op" != ttl ]; then
	case "$value" in
	\"*\") ;;
	*) die 2 "R53_VALUE must be in Route53 form, wrapped in double quotes" ;;
	esac
fi
case "$slot" in
"" | spf | dmarc) ;;
*) die 2 "R53_SLOT '$slot' is not one of: spf, dmarc" ;;
esac
case "$want_ttl" in
"") [ "$op" = ttl ] && die 2 "R53_TTL is empty" ;;
*[!0-9]*) die 2 "R53_TTL '$want_ttl' is not a whole number of seconds" ;;
esac

command -v aws >/dev/null 2>&1 ||
	die 3 "the AWS CLI (aws) is not installed or not on PATH. Install AWS CLI version 2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
case "$(aws --version 2>&1)" in
aws-cli/2.*) ;;
*) die 3 "AWS CLI version 2 is required; found: $(aws --version 2>&1 | head -n 1)" ;;
esac

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/route53-txt.XXXXXX") || die 5 "cannot create a temporary directory"
trap 'rm -rf "$tmpdir"' EXIT
trap 'exit 1' HUP INT TERM

nl='
'
tab=$(printf '\t')

# class VALUE -> prints spf, dmarc or nothing. The value's outer quotes are
# removed and its parts joined before matching, as a resolver reads it.
class() {
	text=$(printf '%s' "$1" | sed -e 's/^"//' -e 's/"$//' -e 's/" "//g')
	if printf '%s' "$text" | grep -Eiq '^v=spf1( |$)'; then
		echo spf
	elif printf '%s' "$text" | grep -Eiq '^v=dmarc1[ ]*;'; then
		echo dmarc
	fi
}

# read_set: sets exists (0/1), cur_ttl and cur (values, one per line).
read_set() {
	query="ResourceRecordSets[?Name=='$name.' && Type=='TXT'] | [0].[TTL, ResourceRecords[].Value][]"
	if ! out=$(aws route53 list-resource-record-sets --hosted-zone-id "$zone" \
		--start-record-name "$name." --start-record-type TXT --max-items 1 \
		--query "$query" --output text 2>"$tmpdir/err"); then
		last_error=$(cat "$tmpdir/err")
		return 1
	fi
	exists=0
	cur_ttl=
	cur=
	case "$out" in
	"" | None) return 0 ;;
	esac
	exists=1
	# One line: TTL, then each value, separated by tabs. Route53 returns any
	# byte outside printable ASCII as a \ooo escape, so a value never holds a
	# tab or a newline.
	lines=$(printf '%s\n' "$out" | tr "$tab" '\n')
	cur_ttl=$(printf '%s\n' "$lines" | sed -n 1p)
	cur=$(printf '%s\n' "$lines" | sed 1d)
	case "$cur_ttl" in
	"" | *[!0-9]*) die 5 "cannot read the TTL of $name TXT from the AWS CLI output; nothing was changed. Output: $out" ;;
	esac
	while IFS= read -r v; do
		case "$v" in
		\"*\") ;;
		*) die 5 "cannot read a value of $name TXT from the AWS CLI output; nothing was changed. Value: $v" ;;
		esac
	done <<EOF
$cur
EOF
	return 0
}

# has LIST VALUE: is VALUE a line of LIST?
has() {
	while IFS= read -r v; do
		[ "$v" = "$2" ] && return 0
	done <<EOF
$1
EOF
	return 1
}

# without LIST VALUE: LIST minus the lines equal to VALUE.
without() {
	while IFS= read -r v; do
		[ -n "$v" ] && [ "$v" != "$2" ] && printf '%s\n' "$v"
	done <<EOF
$1
EOF
}

# without_class LIST CLASS KEEP: LIST minus the values of CLASS, except KEEP,
# so a value already in place keeps its position and nothing is rewritten.
without_class() {
	while IFS= read -r v; do
		[ -n "$v" ] || continue
		if [ "$v" = "$3" ] || [ "$(class "$v")" != "$2" ]; then
			printf '%s\n' "$v"
		fi
	done <<EOF
$1
EOF
}

json_escape() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

# change ACTION TTL LIST: one Change object.
change() {
	printf '{"Action":"%s","ResourceRecordSet":{"Name":"%s.","Type":"TXT","TTL":%s,"ResourceRecords":[' "$1" "$name" "$2"
	sep=
	while IFS= read -r v; do
		[ -n "$v" ] || continue
		printf '%s{"Value":"%s"}' "$sep" "$(json_escape "$v")"
		sep=,
	done <<EOF
$3
EOF
	printf ']}}'
}

retryable() {
	case "$1" in
	*InvalidChangeBatch* | *PriorRequestNotComplete* | *Throttling* | *Rate\ exceeded*) return 0 ;;
	esac
	return 1
}

jitter() {
	awk 'BEGIN { srand(); print int(rand() * 2) }'
}

# For tests: R53_RETRY_SLEEP=0 retries without waiting.
backoff() {
	[ "${R53_RETRY_SLEEP:-1}" = 0 ] && return 0
	sleep $(($1 + $(jitter)))
}

attempt=1
max_attempts=6
last_error=
for wait in 1 2 4 8 8 8; do
	if ! read_set; then
		if retryable "$last_error"; then
			say "$op $name TXT: read refused, retrying (attempt $attempt of $max_attempts): $last_error"
			backoff "$wait"
			attempt=$((attempt + 1))
			continue
		fi
		die 5 "$op $name TXT: cannot read the record set: $last_error"
	fi

	new=$cur
	new_ttl=$cur_ttl
	case "$op" in
	add)
		[ -n "$slot" ] && new=$(without_class "$new" "$slot" "$value")
		if ! has "$new" "$value"; then
			if [ -n "$new" ]; then new="$new$nl$value"; else new=$value; fi
		fi
		[ "$exists" = 1 ] || new_ttl=${want_ttl:-600}
		;;
	remove)
		new=$(without "$new" "$value")
		;;
	ttl)
		if [ "$exists" = 0 ]; then
			say "ttl $name TXT: no record set, nothing to do"
			exit 0
		fi
		new_ttl=$want_ttl
		;;
	esac

	if [ "$exists" = 1 ] && [ "$new" = "$cur" ] && [ "$new_ttl" = "$cur_ttl" ]; then
		say "$op $name TXT: already as wanted, nothing written"
		exit 0
	fi
	if [ "$exists" = 0 ] && [ -z "$new" ]; then
		say "$op $name TXT: no record set, nothing to do"
		exit 0
	fi

	if [ "$exists" = 0 ]; then
		changes=$(change CREATE "$new_ttl" "$new")
		what="create the record set"
	elif [ -z "$new" ]; then
		changes=$(change DELETE "$cur_ttl" "$cur")
		what="delete the record set, nothing else is left in it"
	else
		changes="$(change DELETE "$cur_ttl" "$cur"),$(change CREATE "$new_ttl" "$new")"
		what="update the record set"
	fi
	printf '{"Comment":"meroku %s","Changes":[%s]}\n' "$op" "$changes" >"$tmpdir/batch.json"

	if aws route53 change-resource-record-sets --hosted-zone-id "$zone" \
		--change-batch "file://$tmpdir/batch.json" >/dev/null 2>"$tmpdir/err"; then
		say "$op $name TXT: $what"
		exit 0
	fi
	last_error=$(cat "$tmpdir/err")
	retryable "$last_error" || die 5 "$op $name TXT: Route53 refused the change: $last_error"
	say "$op $name TXT: the record set changed while writing, retrying (attempt $attempt of $max_attempts)"
	backoff "$wait"
	attempt=$((attempt + 1))
done

die 4 "$op $name TXT: gave up after $max_attempts attempts, the record set kept changing. Last error: $last_error"
