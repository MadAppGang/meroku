#!/bin/sh
echo $ENV | sed 's/^"//g; s/"$//g; s=\\n=\n=g;' > .env || true
cat .env
set -o allexport 
source .env || true 
set +o allexport
yarn start