#!/bin/bash
​
# parametes are exported as:
# /${var.env}/${var.project}/backend/ - for backend
# /${var.env}/${var.project}/mockoon/ - for mockoon
# /${var.env}/${var.project}/task/{var.task_name} - for ecs and event_bridge tasks
customPath="/dev/instagram/backend/"
paramType="String" # Can be made "SecretString"
env_file="env.json"
​
jq -r 'to_entries[] | [.key, .value] | @tsv' $env_file | while IFS=$'\t' read -r paramName paramValue; do
    prefixedParamName="${customPath}${paramName}"

    echo $prefixedParamName​
    aws ssm put-parameter --name "$prefixedParamName" --value "$paramValue" --type "$paramType" --overwrite
dones