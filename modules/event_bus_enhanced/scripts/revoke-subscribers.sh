#!/bin/sh
# Revokes subscribers (and, at teardown, event sources) that OTHER accounts
# attached to this account's enhanced EventBridge bus. modules/event_bus_enhanced
# runs it from terraform_data provisioners:
#
#   change    in the apply that changes who may subscribe: revoke the
#             subscribers of accounts that no current share entry entitles.
#   teardown  before the bus is deleted (cleanup or terraform destroy): revoke
#             every foreign subscriber and every foreign event source.
#
# A revoke is permanent: the subscriber stops delivering and its owner can only
# delete it. Resources of this account are never touched.
#
# Input comes only from the environment, never from arguments or the shell
# command line, so no value is ever parsed by a shell:
#   EBV2_MODE           change | teardown
#   EBV2_BUS_ARN        the bus, by its exact ARN; it must belong to
#                       EBV2_OWN_ACCOUNT. Nothing is looked up by name.
#   EBV2_OWN_ACCOUNT    the deploying account (12 digits)
#   EBV2_PROJECT        the project (teardown: tells meroku's own subscriber
#                       names, <project>-events-* and events-*, from others)
#   AWS_REGION          the bus's region
#   change mode only:
#   EBV2_KEEP_ACCOUNTS  space-separated account ids that may still subscribe
#   EBV2_KEEP_ORG       true | false: every account of this organization may
#   EBV2_KEEP_OUS       space-separated OU ids whose accounts may
#   EBV2_LOST           space-separated principals (organization, account:<id>,
#                       ou:<id>) that lost the right to subscribe. Non-empty
#                       means an account Organizations cannot classify stops
#                       the apply instead of being kept.
#
# Credentials are whatever the AWS CLI resolves from the environment it
# inherits, the same chain the Terraform providers use. No --profile is passed
# on purpose: the provider blocks meroku generates have none either.
#
# Exit status: 0 done or nothing to do, 2 usage, 3 aws CLI missing or not v2,
# 5 any other AWS error or unreadable CLI output, 6 (change only) an account
# could not be classified while a principal was lost.

set -u
# The space-separated inputs are split by unquoted expansion; globbing would
# turn a malformed value into file names that then pass validation.
set -f

prog=revoke-subscribers

say() { printf '%s: %s\n' "$prog" "$*"; }
warn() { printf '%s: %s\n' "$prog" "$*" >&2; }
die() {
	code=$1
	shift
	warn "$*"
	exit "$code"
}

mode=${EBV2_MODE:-}
bus=${EBV2_BUS_ARN:-}
own=${EBV2_OWN_ACCOUNT:-}
project=${EBV2_PROJECT:-}
keep_accounts=${EBV2_KEEP_ACCOUNTS:-}
keep_org=${EBV2_KEEP_ORG:-false}
keep_ous=${EBV2_KEEP_OUS:-}
lost=${EBV2_LOST:-}

# --- 1. inputs ---------------------------------------------------------------

case "$mode" in
change | teardown) ;;
*) die 2 "EBV2_MODE must be change or teardown, got '$mode'" ;;
esac

is_account() {
	case "$1" in
	[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]) return 0 ;;
	esac
	return 1
}

is_ou() { printf '%s\n' "$1" | grep -Eq '^ou-[a-z0-9]{4,32}-[a-z0-9]{8,32}$'; }

is_account "$own" || die 2 "EBV2_OWN_ACCOUNT '$own' is not a 12-digit AWS account id"
printf '%s\n' "$bus" | grep -Eq "^arn:aws[a-z-]*:events:[a-z0-9-]+:$own:event-busv2/[^/[:space:]]+/[^/[:space:]]+\$" ||
	die 2 "EBV2_BUS_ARN '$bus' is not an enhanced bus of account $own"
[ -n "${AWS_REGION:-}" ] || die 2 "AWS_REGION is empty"

if [ "$mode" = teardown ]; then
	[ -n "$project" ] || die 2 "EBV2_PROJECT is empty"
else
	case "$keep_org" in
	true | false) ;;
	*) die 2 "EBV2_KEEP_ORG must be true or false, got '$keep_org'" ;;
	esac
	for a in $keep_accounts; do
		is_account "$a" || die 2 "EBV2_KEEP_ACCOUNTS holds '$a', not a 12-digit AWS account id"
	done
	for o in $keep_ous; do
		is_ou "$o" || die 2 "EBV2_KEEP_OUS holds '$o', not an organizational unit id"
	done
	for p in $lost; do
		case "$p" in
		organization) ;;
		account:*) is_account "${p#account:}" || die 2 "EBV2_LOST holds '$p', not account:<12-digit id>" ;;
		ou:*) is_ou "${p#ou:}" || die 2 "EBV2_LOST holds '$p', not ou:<organizational unit id>" ;;
		*) die 2 "EBV2_LOST holds '$p', not organization, account:<id> or ou:<id>" ;;
		esac
	done
fi

command -v aws >/dev/null 2>&1 ||
	die 3 "the AWS CLI (aws) is not installed or not on PATH. Install AWS CLI version 2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
case "$(aws --version 2>&1)" in
aws-cli/2.*) ;;
*) die 3 "AWS CLI version 2 is required; found: $(aws --version 2>&1 | head -n 1)" ;;
esac

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/revoke-subscribers.XXXXXX") || die 5 "cannot create a temporary directory"
trap 'rm -rf "$tmpdir"' EXIT
trap 'exit 1' HUP INT TERM

tab=$(printf '\t')

# --- 2. list -----------------------------------------------------------------

# revoked_so_far is what a failure says of this run's revocations, which are
# permanent: a later listing can fail after subscribers were revoked.
revoked_so_far() {
	if [ "${revoked:-0}" -eq 0 ]; then
		printf 'nothing was revoked'
	else
		printf '%s revoked before it (revocation is permanent; run the apply again to finish)' "$revoked"
	fi
}

# list KIND OUT: KIND is subscribers or event-sources. Writes one line per
# resource to OUT: arn TAB account TAB revoked TAB name. The CLI follows
# NextToken itself. A bus that no longer exists ends the run: nothing is left
# to revoke.
list() {
	if [ "$1" = subscribers ]; then
		set -- "$1" "$2" list-subscribers 'Subscribers[].[SubscriberArn,SubscriberAccountId,Revoked,Name]'
	else
		set -- "$1" "$2" list-event-sources 'EventSources[].[EventSourceArn,EventSourceAccountId,Revoked,Name]'
	fi
	if ! aws eventsv2 "$3" --event-bus-arn "$bus" --query "$4" --output text >"$tmpdir/out" 2>"$tmpdir/err"; then
		if grep -q ResourceNotFoundException "$tmpdir/err"; then
			say "bus $bus not found; nothing to revoke"
			exit 0
		fi
		die 5 "aws eventsv2 $3 failed on $bus; $(revoked_so_far): $(cat "$tmpdir/err")"
	fi
	: >"$2"
	while IFS="$tab" read -r arn acct rev name; do
		case "$arn" in
		"" | None) continue ;;
		arn:*) ;;
		*) die 5 "cannot read the output of aws eventsv2 $3; nothing more was revoked. Line: $arn" ;;
		esac
		is_account "$acct" || die 5 "cannot read the account of $arn from aws eventsv2 $3; nothing more was revoked"
		printf '%s\t%s\t%s\t%s\n' "$arn" "$acct" "$rev" "$name" >>"$2"
	done <"$tmpdir/out"
}

# --- 3. decide (change only) -------------------------------------------------

# decide ACCOUNT: prints keep, revoke or undecided. Any other Organizations
# error stops the run with exit 5.
decide() {
	for k in $keep_accounts; do
		[ "$k" = "$1" ] && echo keep && return 0
	done
	if [ "$keep_org" = true ]; then
		if aws organizations describe-account --account-id "$1" --query Account.Id --output text </dev/null >/dev/null 2>"$tmpdir/oerr"; then
			echo keep
		elif grep -q AccountNotFoundException "$tmpdir/oerr"; then
			echo revoke
		elif grep -q AccessDenied "$tmpdir/oerr"; then
			echo undecided
		else
			die 5 "aws organizations describe-account failed for account $1: $(cat "$tmpdir/oerr")"
		fi
		return 0
	fi
	if [ -n "$keep_ous" ]; then
		child=$1
		# An organization is at most five OUs deep under its root.
		depth=0
		while [ "$depth" -lt 10 ]; do
			depth=$((depth + 1))
			if ! parent=$(aws organizations list-parents --child-id "$child" \
				--query 'Parents[0].[Id,Type]' --output text </dev/null 2>"$tmpdir/oerr"); then
				if grep -q ChildNotFoundException "$tmpdir/oerr"; then
					echo revoke
				elif grep -q AccessDenied "$tmpdir/oerr"; then
					echo undecided
				else
					die 5 "aws organizations list-parents failed for $child: $(cat "$tmpdir/oerr")"
				fi
				return 0
			fi
			pid=$(printf '%s\n' "$parent" | cut -f 1)
			ptype=$(printf '%s\n' "$parent" | cut -f 2)
			for o in $keep_ous; do
				[ "$o" = "$pid" ] && echo keep && return 0
			done
			case "$ptype" in
			ROOT) echo revoke && return 0 ;;
			ORGANIZATIONAL_UNIT) child=$pid ;;
			*) die 5 "cannot read the parent of $child from aws organizations list-parents: $parent" ;;
			esac
		done
		die 5 "account $1 is more than 10 levels below its organization root"
	fi
	echo revoke
}

# --- 4. revoke ---------------------------------------------------------------

revoked=0

# revoke ARN ACCOUNT. Already revoked, or gone since the listing, counts as done.
revoke() {
	if aws eventsv2 revoke-resource --arn "$1" </dev/null >/dev/null 2>"$tmpdir/rerr"; then
		say "revoked $1 (account $2)"
	elif grep -q InvalidStateException "$tmpdir/rerr"; then
		say "revoked $1 (account $2; it was already revoked)"
	elif grep -q ResourceNotFoundException "$tmpdir/rerr"; then
		say "$1 (account $2) no longer exists; nothing to revoke"
		return 0
	else
		die 5 "aws eventsv2 revoke-resource failed for $1 (account $2): $(cat "$tmpdir/rerr")"
	fi
	revoked=$((revoked + 1))
}

kinds=subscribers
[ "$mode" = teardown ] && kinds="subscribers event-sources"

undecided=
for kind in $kinds; do
	list "$kind" "$tmpdir/$kind"
	while IFS="$tab" read -r arn acct rev name; do
		if [ "$acct" = "$own" ]; then
			# meroku's own subscribers are Terraform's to delete. Anything else
			# of this account was made outside meroku, and a non-revoked
			# resource blocks the bus delete.
			if [ "$mode" = teardown ]; then
				case "$name" in
				"$project"-events-* | events-*) ;;
				*) say "$arn was not created by meroku and will block the bus delete; delete it or keep the bus" ;;
				esac
			fi
			continue
		fi
		case "$rev" in
		True | true) continue ;;
		esac
		if [ "$mode" = teardown ]; then
			revoke "$arn" "$acct"
			continue
		fi
		d=$(decide "$acct") || exit $?
		case "$d" in
		keep) say "kept $arn (account $acct may still subscribe)" ;;
		revoke) revoke "$arn" "$acct" ;;
		undecided) undecided="$undecided$arn$tab$acct
" ;;
		*) die 5 "internal error: no decision for account $acct" ;;
		esac
	done <"$tmpdir/$kind"
done

if [ -n "$undecided" ]; then
	if [ -n "$lost" ]; then
		warn "cannot tell whether these subscribers' accounts may still subscribe (Organizations denied the lookup), and access was removed from: $lost"
		printf '%s' "$undecided" | while IFS="$tab" read -r arn acct; do
			warn "  $arn (account $acct)"
		done
		warn "Nothing was guessed. Either deploy with credentials that can call organizations:DescribeAccount and organizations:ListParents (the management account or a delegated administrator), or add an explicit account entry with access: subscribe for each account that is still entitled and revoke the rest by hand: aws eventsv2 revoke-resource --arn <arn>"
		exit 6
	fi
	printf '%s' "$undecided" | while IFS="$tab" read -r arn acct; do
		say "kept $arn (account $acct): Organizations denied the lookup, and no access was removed"
	done
fi

if [ "$revoked" -eq 0 ]; then
	if [ "$mode" = teardown ]; then
		say "no subscribers or event sources of other accounts to revoke on $bus"
	else
		say "no subscribers of other accounts to revoke on $bus"
	fi
fi
exit 0
