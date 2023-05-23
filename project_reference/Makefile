
.PHONY: dev
dev: 
	gomplate -c vars=dev.yaml -f ./architecture/env/main.tmpl   -o ./architecture/env/dev/main.tf


.PHONY: prod
prod: 
	gomplate -c vars=prod.yaml -f ./architecture/env/main.tmpl   -o ./architecture/env/prod/main.tf