#!/bin/bash
# $1 - first argument, we use it for env: dev or prod

cp -rf ./infrastructure/env/outputs.tf  ./env/$1/outputs.tf

for d in plugins/*/ ; do
    echo \ >> ./env/$1/main.tf
    echo \ >> ./env/$1/main.tf
    echo //$d >> ./env/$1/main.tf
    # Uses Handlebars templating via Raymond to generate module config
    meroku template -c vars=$1.yaml -f "./$d/module.tmpl" >> ./env/$1/main.tf
    echo \ >> ./env/$1/main.tf
    
    if test -f "$d/outputs.tf"; then
        plugin_name=$(basename $d)
        echo \ >> ./env/$1/outputs.tf
        echo \ >> ./env/$1/outputs.tf
        echo //$d >> ./env/$1/outputs.tf
        cat ./$d/outputs.tf | grep output | sed -E "s/^[[:space:]]*(output[[:space:]]*\"([^\"]+)\"(.+))/output \"\2\" {\n\tvalue = module.$plugin_name.\2\n\tsensitive = true\n}\n/g" >> ./env/$1/outputs.tf
        echo \ >> ./env/$1/outputs.tf
    fi
done