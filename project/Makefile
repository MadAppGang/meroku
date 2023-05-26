
.PHONY: clean

clean:
	rm -rf env
	rm -rf infrastructure

.PHONY: init
init:
	mkdir -p env/dev
	mkdir -p env/prod
	git clone --depth=1 --branch=main https://github.com/MadAppGang/infrastructure.git ./infrastructure
	rm -rf ./infrastructure/.git
	cp -rf ./infrastructure/env/outputs.tf env/dev
	cp -rf ./infrastructure/env/outputs.tf env/prod

.PHONY: update
update: 
	rm -rf ./infrastructure
	git clone --depth=1 --branch=main https://github.com/MadAppGang/infrastructure.git ./infrastructure
	rm -rf ./infrastructure/.git
	cp -rf ./infrastructure/env/outputs.tf env/dev
	cp -rf ./infrastructure/env/outputs.tf env/prod

.PHONY: dev
dev: 
	gomplate -c vars=dev.yaml -f ./infrastructure/env/main.tmpl   -o ./env/dev/main.tf


.PHONY: prod
prod: 
	gomplate -c vars=prod.yaml -f ./infrastructure/env/main.tmpl   -o ./env/prod/main.tf

.PHONY: versions
version:
	cat ./infrastructure/version.txt

devplan:
	cd env/dev/ && terraform init && terraform plan

prodplan:
	cd env/prod/ && terraform init && terraform plan

devapply:
	cd env/dev/ && terraform init && terraform apply

prodapply:
	cd env/prod/ && terraform init && terraform apply

