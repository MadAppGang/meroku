#!/bin/bash
# $1 - first argument, we use it for env: dev or prod

for d in plugins/*/ ; do
    echo \ >> ./env/$1/main.tf
    echo \ >> ./env/$1/main.tf
    echo //$d >> ./env/$1/main.tf
    gomplate -c vars=$1.yaml -f "./$d/module.tmpl"  >> ./env/$1/main.tf
    echo \ >> ./env/$1/main.tf    
done