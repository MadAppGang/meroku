

init: ./env/dev ./env/prod
	mkdir env
	mkdir env/dev
	mdkir env/prod
	cp ./architecture/env/output.tf env/dev
	cp ./architecture/env/output.tf env/prod

.PHONY: update
update: 
	rm -rf ./architecture
	rm env/dev/output.tf
	rm env/prod/output.tf
	git clone --depth=1 --branch=main https://github.com/MadAppGang/architecture.git ./architecture
	rm -rf ./architecture/.git
	cp ./architecture/env/output.tf env/dev
	cp ./architecture/env/output.tf env/prod

.PHONY: dev
dev: 
	gomplate -c vars=dev.yaml -f ./architecture/env/main.tmpl   -o ./env/dev/main.tf


.PHONY: prod
prod: 
	gomplate -c vars=prod.yaml -f ./architecture/env/main.tmpl   -o ./env/prod/main.tf

.PHONY: versions
version:
	cat ./architecture/version.txt

tdp:
	cd env/dev/
	terraform init
	terraform plan

tpp:
	cd env/prod/
	terraform init
	terraform plan
