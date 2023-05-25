.PHONY: clean
.PHONY: init
.PHONY: update
.PHONY: dev
.PHONY: prod
.PHONY: version


UNAME := $(shell uname -s)
ifeq ($(UNAME), Darwin)
    sc = sed -i ''
else 
    sc = sed -i
endif

clean:
	rm -rf env
	rm -rf architecture


init:
	mkdir -p env/dev
	mkdir -p env/prod
	git clone --depth=1 --branch=main https://github.com/MadAppGang/architecture.git ./architecture
	rm -rf ./architecture/.git
	cp -rf ./architecture/env/outputs.tf env/dev
	cp -rf ./architecture/env/outputs.tf env/prod

update: 
	rm -rf ./architecture
	git clone --depth=1 --branch=main https://github.com/MadAppGang/architecture.git ./architecture
	rm -rf ./architecture/.git
	cp -rf ./architecture/env/outputs.tf env/dev
	cp -rf ./architecture/env/outputs.tf env/prod

dev: 
	gomplate -c vars=dev.yaml -f ./architecture/env/main.tmpl   -o ./env/dev/main.tf


prod: 
	gomplate -c vars=prod.yaml -f ./architecture/env/main.tmpl   -o ./env/prod/main.tf

version:
	cat ./architecture/version.txt

devplan:
	cd env/dev/l; \
	terraform init; \
	terraform plan	

prodplan:
	cd env/prod/; \
	terraform init; \
	terraform plan

devapply:
	cd env/dev/; \
	terraform init; \
	terraform apply; \
	echo "Setting ECR repos values for prod ..."; \
	${sc} "s/ecr_account_id:.*/ecr_account_id: `terraform output -raw account_id`/g; s/ecr_account_region:.*/ecr_account_region: `terraform output -raw region`/g" ../../prod.yaml 

prodapply:
	cd env/prod/; \
	terraform init; \
	terraform apply