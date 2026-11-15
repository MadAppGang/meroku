# KMS signing keys

A KMS signing key is an asymmetric AWS KMS key that never leaves KMS. The YAML
declares the keys of an environment; each workload connected to a key may sign
with it, verify with it, or both, and its container gets the key's ARN and its
public key as environment variables. Built by `modules/kms_keys`; the YAML is
normalised in `app/kms_keys.go`.

Use it for tokens, webhooks or documents that other parties verify: the private
key cannot be read or copied, and anyone holding the public key can verify a
signature without calling AWS.

## YAML

```yaml
kms_keys:
  - name: auth                 # required
    key_spec: ECC_NIST_P256    # optional; the default
  - name: webhooks
    key_spec: RSA_2048
  - name: old_auth
    retired: true              # still deployed; nothing may connect to it

workload:                      # the backend
  kms_keys:
    - { name: auth, access: both }

services:
  - name: api
    kms_keys:
      - { name: auth, access: sign }
      - { name: webhooks, access: verify }

scheduled_tasks:
  - name: nightly
    kms_keys: [{ name: auth, access: verify }]

event_processor_tasks:
  - name: ingest
    kms_keys: [{ name: webhooks, access: sign }]
```

- `kms_keys[].name`: 1 to 31 lower-case letters, digits and underscores,
  starting with a letter (`^[a-z][a-z0-9_]{0,30}$`). Unique. The web UI names a
  new key `key<n>`, n one more than the highest `key<n>` already in the list.
- `kms_keys[].key_spec`: one of `ECC_NIST_P256` (the default), `ECC_NIST_P384`,
  `ECC_NIST_P521`, `ECC_SECG_P256K1`, `RSA_2048`, `RSA_3072`, `RSA_4096`. It
  cannot change on an existing key; see [Changing the key type](#changing-the-key-type).
- `kms_keys[].retired`: optional, default `false`. See [Removing a key](#removing-a-key).
- A connection's `name` must be a key of the top-level list that is not retired,
  and a workload connects to a key at most once.
- `access`: required, `sign`, `verify` or `both`.

`meroku generate` refuses anything else and names the field, for example:

```
kms_keys:
  workload.kms_keys[0]: kms key "auth" is retired: remove the connection, or set retired: false
  services[0].kms_keys ('api')[0] connects kms key "billing", which is not in the top-level kms_keys list; add it under kms_keys first
```

No schema migration: an absent `kms_keys` block creates no key, and the struct
fields use `omitempty` ([MIGRATIONS.md](./MIGRATIONS.md)).

## Environment variables

For each key a workload connects to, with `<NAME>` the key name upper-cased:

| Variable | Value |
|---|---|
| `KMS_<NAME>_KEY_ARN` | The key's ARN, for `kms:Sign` and `kms:Verify` calls. |
| `KMS_<NAME>_PUBLIC_KEY` | The key's public key, PEM-encoded (`-----BEGIN PUBLIC KEY-----` …). |

`KMS_AUTH_PUBLIC_KEY` holds a **multi-line** PEM: the header line, the base64
body over several lines, the footer line. ECS passes container variables as JSON
strings, so the newlines reach the process unchanged; read the variable as is
and hand it to the PEM parser. Do not expect a single line.

Every connected workload gets both variables whatever its access, sorted by key
name. A workload's YAML variable with the same name (`backend_env_variables`,
`env_vars`, `environment_variables`) is refused at generate, and so is a vault
secret that would become the same name on that workload.

The values are known only once the key exists: the first apply that creates a
key registers the task definitions that name it in the same run.

## Grants

One inline policy on each connected workload's **task role**,
`<project>_kms_keys_<workload>_<env>`, scoped to the ARNs of its own keys:

| Access | Actions |
|---|---|
| `sign` | `kms:Sign`, `kms:GetPublicKey`, `kms:DescribeKey` |
| `verify` | `kms:Verify`, `kms:GetPublicKey`, `kms:DescribeKey` |
| `both` | `kms:Sign`, `kms:Verify`, `kms:GetPublicKey`, `kms:DescribeKey` |

Each key keeps the default key policy, which delegates to IAM in the account; no
other principal can use a key unless an IAM policy in the account grants it.

## The keys

| Setting | Value |
|---|---|
| Key usage | `SIGN_VERIFY` |
| Key type | `key_spec` (`customer_master_key_spec`) |
| Alias | `alias/<project>-<env>-<name>` |
| Deletion window | 7 days |
| Rotation | none: AWS does not rotate asymmetric keys |

`module.kms_keys.key_arns` (and `local.bridge.kms_key_arns`, `{}` without keys)
lists every key's ARN by name, retired keys included.

## Signing and verifying in an application

Sign with KMS, using the ARN from `KMS_<NAME>_KEY_ARN`:

```
aws kms sign \
  --key-id "$KMS_AUTH_KEY_ARN" \
  --message fileb://payload.bin \
  --message-type RAW \
  --signing-algorithm ECDSA_SHA_256
```

From an SDK, call `Sign` with `KeyId`, `Message`, `MessageType` and
`SigningAlgorithm`:

- `MessageType: RAW` sends the message itself (up to 4096 bytes) and KMS hashes
  it. `DIGEST` sends a hash you computed, which suits large payloads; the hash
  must match the algorithm (32 bytes for `*_SHA_256`).
- `SigningAlgorithm` must suit the key type:

  | key_spec | Algorithm |
  |---|---|
  | `ECC_NIST_P256` | `ECDSA_SHA_256` |
  | `ECC_NIST_P384` | `ECDSA_SHA_384` |
  | `ECC_NIST_P521` | `ECDSA_SHA_512` |
  | `ECC_SECG_P256K1` | `ECDSA_SHA_256` |
  | `RSA_*` | `RSASSA_PSS_SHA_256` / `_384` / `_512`, or `RSASSA_PKCS1_V1_5_SHA_256` / `_384` / `_512` |

An ECDSA signature comes back DER-encoded. A JWT (`ES256`) wants the raw 64-byte
`r || s` form, so convert it before encoding.

Verify locally with the PEM from `KMS_<NAME>_PUBLIC_KEY`: parse it as a
SubjectPublicKeyInfo public key with the language's crypto library and check the
signature with the same algorithm. That needs no AWS call and no `kms:Verify`.
`kms:Verify` (access `verify` or `both`) is for an application that prefers KMS
to check the signature, or has no crypto library for the key type.

## Changing the key type

`key_spec` cannot change on an existing key: Terraform would destroy the key and
create a new one, every signature made with the old key would stop verifying,
and the old key would be scheduled for deletion. The web UI fixes the type once
a key is saved. To move to another type, add a new key with the new type, move
the connections to it, deploy, and retire the old key once nothing verifies with
it any more.

## Removing a key

Removing a key takes two applies, so nothing loses a key it still uses:

1. **Retire**: set `retired: true` and remove every connection to the key (the
   web UI's "Retire" does both). Apply. The variables and the IAM grants go; the
   key still exists, so a signature it made can still be verified with its
   public key, and anything outside meroku that uses it keeps working.
2. **Delete**: remove the entry from `kms_keys`. Apply. Terraform destroys the
   key, which AWS turns into a **scheduled deletion 7 days ahead**. During those
   7 days the key is disabled (no Sign, no Verify, no GetPublicKey) but not gone:

   ```
   aws kms cancel-key-deletion --key-id <key ARN>
   aws kms enable-key --key-id <key ARN>
   ```

   brings it back. Put the entry back in the YAML and import the key into
   `module.kms_keys.aws_kms_key.key["<name>"]` (and its alias) before the next
   apply; without the import Terraform creates a new key instead. After 7 days
   the key is gone for good; a saved copy of its public key still verifies the
   signatures it made.

Setting `retired: false` again restores a retired key: connect it and apply.

## Destroy

`meroku destroy` destroys every key, retired ones included, which schedules each
for deletion with the 7-day window above. Cancel within the window to keep a
key.

## Cost

About $1 per key per month (AWS bills asymmetric keys the same as symmetric
ones), retired keys included, plus per-request charges for `Sign`, `Verify` and
`GetPublicKey`, a few cents per 10,000 requests depending on the key type (see
AWS KMS pricing). Verifying locally with the PEM makes no request.

## Custom Terraform

A project's root-level custom files may read `module.kms_keys.key_arns`,
`module.kms_keys.environment` and `local.bridge.kms_key_arns`. A custom `.tf`
that declares its own `module "kms_keys"` is refused at generate; rename it.
