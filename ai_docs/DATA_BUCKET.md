# Data bucket

Every environment has a private S3 bucket for application data, created by
`modules/data_bucket`. Each workload — the backend, every service, every scheduled
task and every event task — reads and writes its own folder in it, and the YAML
can open shared folders to the workloads it names. No custom Terraform is needed.

## YAML

```yaml
data_bucket:                 # optional; the bucket always exists
  shared:                    # → shared/<name>/
    - name: uploads
      read_write: [api, backend]
      read: [reports]
```

- `name`: 1 to 32 lower-case letters, digits and hyphens, starting and ending
  with a letter or a digit. Unique.
- `read_write`, `read`: workload names — `backend`, or the name of a service,
  scheduled task or event task. A workload appears at most once per folder.

`meroku generate` refuses a name that is not a workload. A workload that is
switched off (`enabled: false`) is dropped from the folder with a warning and
gets access back when it is switched on. The web UI's "Data bucket" tab on the
S3 node edits the same list, and deleting a service or task in the UI removes it
from every folder.

There is no schema migration: absent means no shared folders.

## The bucket

| Setting | Value |
|---|---|
| Name | `<project>-data-<env>-<account id>` through `modules/naming` (limit 63, the account ID is a suffix that is never truncated) |
| Public access | all four blocks on |
| Object ownership | `BucketOwnerEnforced` (ACLs disabled) |
| Encryption | SSE-S3 (AES256) |
| Versioning | on |
| Lifecycle | noncurrent versions expire after 30 days; incomplete multipart uploads are aborted after 7 days |
| Bucket policy | denies every request not made over TLS |
| CORS | none |
| `force_destroy` | `false`, fixed, no YAML field |

## What each workload gets

Environment variables, appended after the workload's own:

| Variable | Value | Who |
|---|---|---|
| `DATA_BUCKET_NAME` | the bucket name | every workload |
| `DATA_BUCKET_PATH` | `<name>/` | every workload |
| `DATA_BUCKET_SHARED_<NAME>` | `shared/<name>/` (`-` in the name becomes `_`) | each workload listed on the folder |
| `AWS_S3_BUCKET` | the backend bucket | every workload except the backend, which has always had it |

A YAML env var on a workload that sets `DATA_BUCKET_NAME`, `DATA_BUCKET_PATH` or a
`DATA_BUCKET_SHARED_*` name is refused at generate, naming the field
(`app/workload_env_reserved.go`). The exception is `AWS_S3_BUCKET` on a service or
task: those never had it, so a YAML value most likely names a bucket of their own.
Its value is kept, meroku does not add the backend bucket, and generate prints a
warning. The grant on the backend bucket is the same either way.

One inline policy on the workload's **task role**,
`<project>_data_bucket_<name>_<env>`:

- `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:AbortMultipartUpload` on
  `<bucket>/<name>/*` and on each `read_write` folder `<bucket>/shared/<folder>/*`.
- `s3:GetObject` on each `read` folder.
- `s3:ListBucket` on the bucket with `StringLike` `s3:prefix` = `<name>/*` and
  `shared/<folder>/*` for each folder it can use. The `/` before the `*` keeps
  `api` from listing `api-billing/`. Listing without one of those prefixes is
  denied.
- Never `s3:DeleteObjectVersion`: a deleted or overwritten object stays a
  noncurrent version for 30 days, and no task can remove that copy.
- Every workload except the backend also gets object-level read-write on the
  backend bucket: `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`,
  `s3:AbortMultipartUpload`, `s3:ListMultipartUploadParts`, `s3:PutObjectAcl`
  (effective only when the backend bucket is public) on its objects, and
  `s3:ListBucket`, `s3:GetBucketLocation` on the bucket. No bucket-level write.

The backend keeps its existing managed attachment on the backend bucket
(`backend_task_backend_bucket`, `s3:*`). The services' old execution-role
attachment on the backend bucket is gone; their env files keep their own grant
(`services_s3_env`).

## Things to know

- **Presigned URLs die with the role session.** A URL presigned with task-role
  credentials stops working when those temporary credentials expire, whatever
  expiry the URL itself asks for.
- **No CORS, so uploads go through a service.** A browser cannot PUT to the bucket
  directly from another origin. Upload to a service, which writes to the bucket.
- **ACL headers fail.** With `BucketOwnerEnforced`, a request that sets an ACL
  (`x-amz-acl`, `--acl`) other than `bucket-owner-full-control` is refused.
- **Destroying an environment fails while the bucket holds any object version.**
  `force_destroy` is `false`. Empty every version and delete marker first, for
  example with `aws s3api list-object-versions` and `delete-objects`, then destroy.

## Custom Terraform

Root-level files in `custom/terraform/` read `local.bridge.data_bucket_name`,
`local.bridge.data_bucket_arn` and `local.bridge.backend_bucket_arn`. The module
name `data_bucket` and the locals `workload_roles`, `workload_extra_env` and
`workload_extra_secrets` belong to the generated `main.tf`; a custom file that
declares one of them is refused at generate.

## The workload contract

`module.data_bucket` is the first capability module built on the workload
contract (`modules/workloads/workload_contract.tf`, `modules/ecs_task/workload.tf`,
`modules/event_bridge_task/workload.tf`). Each workload exports `task_role_name`,
`execution_role_name`, `security_group_id`, `sg_key`, `ssm_path` and
`event_source`; `env/main.hbs` merges them into `local.workload_roles`. A
capability module grants against those role names and returns environment
variables per workload, which the root collects into `local.workload_extra_env`
and passes back as `extra_env`. None of the contract outputs reads `extra_env`,
so there is no dependency cycle.

Workload names are unique across all kinds, and `backend`, `vault`, `task` and
`shared` are reserved (`app/workload_names.go`).

## Churn on upgrade

No resource is replaced. Every task definition gets one new revision for the new
variables, so `auto_deploy` services roll once, and `terraform init` is needed for
the new module.
