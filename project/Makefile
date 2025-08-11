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
	rm -rf infrastructure

init:
	mkdir -p env/dev
	mkdir -p env/prod
	git clone --depth=1 --branch=main https://github.com/MadAppGang/infrastructure.git ./infrastructure
	rm -rf ./infrastructure/.git

update: 
	rm -rf ./infrastructure
	git clone --depth=1 --branch=main https://github.com/MadAppGang/infrastructure.git ./infrastructure
	rm -rf ./infrastructure/.git

dev: 
	# Uses Handlebars templating via Raymond to generate Terraform files
	meroku template -c vars=dev.yaml -f ./infrastructure/env/main.hbs -o ./env/dev/main.tf


prod: 
	# Uses Handlebars templating via Raymond to generate Terraform files
	meroku template -c vars=prod.yaml -f ./infrastructure/env/main.hbs -o ./env/prod/main.tf

version:
	cat ./infrastructure/version.txt

devplan:
	cd env/dev/; \
	terraform init; \
	terraform plan	

prodplan:
	cd env/prod/; \
	terraform init; \
	terraform plan

devapply:
	cd env/dev; \
	terraform init; \
	terraform apply; \
	echo "Setting ECR repos values for prod ..."; \
	${sc} "s/ecr_account_id:.*/ecr_account_id: \"`terraform output -raw account_id`\"/g; s/ecr_account_region:.*/ecr_account_region: `terraform output -raw region`/g;" ../../prod.yaml 


prodapply:
	cd env/prod/; \
	terraform init; \
	terraform apply


buildlambda:
	cd infrastructure/modules/workloads/ci_lambda/; \
	rm bootstrap; \
	GOOS=linux GOARCH=amd64 go build -o bootstrap; \
	rm ../../../../env/dev/ci_lambda.zip || true; \
	rm ../../../../env/prod/ci_lambda.zip || true
