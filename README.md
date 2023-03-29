# Reference Cloud infrastructure (IaC)

This repository declares infrastructure of Gigit cloud as a code using [Terraform](https://www.terraform.io/).

## Dependencies

- Terraform v1.2.6
- AWS credentials for accessing Terraform state (hosted in S3 bucket)

1. Init Terraform:

```sh
    cd infra
    terraform init
```

Make sure your AWS CLI is configured for accessing `project-terraform-state` bucket, which hosts Terraform configuration.

2. Run and save terraform plan:

```sh
    terraform plan -out=plan.out
```

3. Apply when you're happy with the plan:

```sh
    terraform apply -out=plan.out
```


