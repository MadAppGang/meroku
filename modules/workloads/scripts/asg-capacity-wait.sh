#!/bin/sh
# Waits until an Auto Scaling group has at least ASG_MIN_IN_SERVICE instances
# InService. modules/workloads runs it from a terraform_data provisioner after
# creating a compute pool's ASG.
#
# The AWS provider's own wait (wait_for_capacity_timeout) ends the apply on the
# FIRST failed scaling activity and taints the ASG, even when the failure is
# temporary and the ASG's own retry succeeds a minute later. The common case is
# the first ASG in an account: AWS creates the Auto Scaling service-linked role
# as the ASG is created, and the first launch, seconds later, fails with
# "Authentication Failure" while that role and the new instance profile
# propagate. This script waits on the condition that matters instead: failed
# launches are printed and waited out, and only a group that stays short of its
# minimum until ASG_TIMEOUT fails the apply, with the last reason AWS gave.
#
# Input comes only from the environment:
#   ASG_NAME            Auto Scaling group name
#   ASG_MIN_IN_SERVICE  instances that must be InService (0: return at once)
#   ASG_TIMEOUT         seconds before giving up (default 900)
#   ASG_POLL_INTERVAL   seconds between checks (default 10)
#   AWS_REGION          the group's region
#
# Credentials are whatever the AWS CLI resolves from the environment it
# inherits, the same chain the Terraform AWS provider uses.
#
# Exit status: 0 satisfied, 2 usage, 3 aws CLI missing or not v2,
# 4 still short of the minimum at the deadline.

set -u

prog=asg-capacity-wait

say() { printf '%s: %s\n' "$prog" "$*" >&2; }
die() {
	code=$1
	shift
	say "$*"
	exit "$code"
}

name=${ASG_NAME:-}
min=${ASG_MIN_IN_SERVICE:-}
timeout=${ASG_TIMEOUT:-900}
interval=${ASG_POLL_INTERVAL:-10}

[ -n "$name" ] || die 2 "ASG_NAME is empty"
for pair in "ASG_MIN_IN_SERVICE=$min" "ASG_TIMEOUT=$timeout" "ASG_POLL_INTERVAL=$interval"; do
	case "${pair#*=}" in
	"" | *[!0-9]*) die 2 "${pair%%=*} '${pair#*=}' is not a whole number" ;;
	esac
done

[ "$min" -eq 0 ] && exit 0

command -v aws >/dev/null 2>&1 ||
	die 3 "the AWS CLI (aws) is not installed or not on PATH. Install AWS CLI version 2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
case "$(aws --version 2>&1)" in
aws-cli/2.*) ;;
*) die 3 "AWS CLI version 2 is required; found: $(aws --version 2>&1 | head -n 1)" ;;
esac

deadline=$(($(date +%s) + timeout))
seen=""
last_failure=""

while :; do
	if n=$(aws autoscaling describe-auto-scaling-groups \
		--auto-scaling-group-names "$name" \
		--query "length(AutoScalingGroups[0].Instances[?LifecycleState=='InService'])" \
		--output text 2>&1); then
		case "$n" in
		"" | *[!0-9]*) say "unexpected answer for $name: $n" ;;
		*)
			if [ "$n" -ge "$min" ]; then
				say "$name: $n of $min instances in service"
				exit 0
			fi
			;;
		esac
	else
		say "describing $name failed, retrying: $n"
	fi

	# The latest activity only: an older failure the ASG has already retried
	# past says nothing new, and each new one is printed exactly once.
	if act=$(aws autoscaling describe-scaling-activities \
		--auto-scaling-group-name "$name" --max-items 1 \
		--query 'Activities[0].[ActivityId,StatusCode,StatusMessage]' \
		--output text 2>/dev/null); then
		id=$(printf '%s' "$act" | cut -f1)
		status=$(printf '%s' "$act" | cut -f2)
		message=$(printf '%s' "$act" | cut -f3-)
		if [ "$status" = Failed ] && [ "$id" != "$seen" ]; then
			seen=$id
			last_failure=$message
			say "$name: launch failed, the group retries by itself: $message"
		fi
	fi

	if [ "$(date +%s)" -ge "$deadline" ]; then
		if [ -n "$last_failure" ]; then
			die 4 "$name still has fewer than $min instances in service after ${timeout}s. Last failed launch: $last_failure"
		fi
		die 4 "$name still has fewer than $min instances in service after ${timeout}s, and no launch has failed. Check the group's activity history: aws autoscaling describe-scaling-activities --auto-scaling-group-name $name"
	fi

	sleep "$interval"
done
