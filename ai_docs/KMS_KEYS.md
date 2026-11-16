# KMS signing and HMAC keys

A KMS key here is either a **signing key**, an asymmetric AWS KMS key, or an
**HMAC key**; neither ever leaves KMS. The YAML declares the keys of an
environment; each workload connected to a key may sign with it, verify with it,
or both, and its container gets the key's ARN, plus the public key of a signing
key, as environment variables. Built by `modules/kms_keys`, which reads the
public keys through its child module `modules/kms_keys/public_keys`; the YAML is
normalised in `app/kms_keys.go`.

Use a signing key for tokens, webhooks or documents that other parties verify:
the private key cannot be read or copied, and anyone holding the public key can
verify a signature without calling AWS. Use an HMAC key for requests between
workloads that all run in AWS; see [HMAC keys](#hmac-keys).

## YAML

```yaml
kms_keys:
  - name: auth                 # required
    key_spec: ECC_NIST_P256    # optional; the default
  - name: webhooks
    key_spec: RSA_2048
  - name: requests
    key_spec: HMAC_256         # an HMAC key
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
      - { name: requests, access: sign }    # creates MACs

scheduled_tasks:
  - name: nightly
    kms_keys:
      - { name: auth, access: verify }
      - { name: requests, access: verify }  # checks MACs

event_processor_tasks:
  - name: ingest
    kms_keys: [{ name: webhooks, access: sign }]
```

- `kms_keys[].name`: 1 to 31 lower-case letters, digits and underscores,
  starting with a letter (`^[a-z][a-z0-9_]{0,30}$`). Unique. The web UI names a
  new key `key<n>`, n one more than the highest `key<n>` already in the list.
- `kms_keys[].key_spec`: one of `ECC_NIST_P256` (the default), `ECC_NIST_P384`,
  `ECC_NIST_P521`, `ECC_SECG_P256K1`, `RSA_2048`, `RSA_3072`, `RSA_4096` (signing
  keys, key usage `SIGN_VERIFY`), or `HMAC_224`, `HMAC_256`, `HMAC_384`,
  `HMAC_512` (HMAC keys, key usage `GENERATE_VERIFY_MAC`). There is no separate
  usage field: the usage follows from the type. Once an apply has created the
  key, a changed value is ignored; see
  [Changing the key type](#changing-the-key-type).
- `kms_keys[].retired`: optional, default `false`. See [Removing a key](#removing-a-key).
- A connection's `name` must be a key of the top-level list that is not retired,
  and a workload connects to a key at most once.
- `access`: required, `sign`, `verify` or `both`, for every key type. On an
  HMAC key `sign` means create a MAC and `verify` check one.

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
| `KMS_<NAME>_KEY_ARN` | The key's ARN, for `kms:Sign` and `kms:Verify` calls (`kms:GenerateMac` and `kms:VerifyMac` on an HMAC key). |
| `KMS_<NAME>_PUBLIC_KEY` | Signing keys only: the key's public key, PEM-encoded (`-----BEGIN PUBLIC KEY-----` …). |

`KMS_AUTH_PUBLIC_KEY` holds a **multi-line** PEM: the header line, the base64
body over several lines, the footer line. ECS passes container variables as JSON
strings, so the newlines reach the process unchanged; read the variable as is
and hand it to the PEM parser. Do not expect a single line.

Every workload connected to a signing key gets both variables whatever its
access; a workload connected to an HMAC key gets only `KMS_<NAME>_KEY_ARN`.
Variables are sorted by key name. A workload's YAML variable with the name of
one it gets (`backend_env_variables`, `env_vars`, `environment_variables`) is
refused at generate, and so is a vault secret that would become the same name on
that workload. `KMS_<NAME>_PUBLIC_KEY` is reserved only for a signing key: on a
workload connected to the HMAC key `<name>`, a YAML variable of that name is
allowed.

The values are known only once the key exists: the first apply that creates a
key registers the task definitions that name it in the same run.

Adding, retiring or deleting one key leaves the other keys' variables known at
plan, so a workload connected only to other keys keeps its task definition.
This holds because the public keys are read in a child module,
`modules/kms_keys/public_keys`, whose only input is a map of key name to ARN.
Terraform records a data source's dependency on a managed resource for the
whole `aws_kms_key.key` block, not per key: a read placed beside the keys is
deferred to apply for every key whenever any key is created or destroyed, every
`KMS_<NAME>_PUBLIC_KEY` becomes unknown, and ECS replaces the task definition of
every connected workload. Do not move the read back into `modules/kms_keys`;
`modules/kms_keys/tests/public_key_known_at_plan.tftest.hcl` fails if it is.

A new grant can take a few seconds to reach every KMS endpoint after the apply
that creates it. An application that calls KMS at startup should retry an
`AccessDeniedException` a few times over several seconds before it gives up,
so a task started right after the apply does not crash on its first call.

## Grants

One inline policy on each connected workload's **task role**,
`<project>_kms_keys_<workload>_<env>`, scoped to the ARNs of its own keys:

| Access | Signing key | HMAC key |
|---|---|---|
| `sign` | `kms:Sign`, `kms:GetPublicKey`, `kms:DescribeKey` | `kms:GenerateMac`, `kms:DescribeKey` |
| `verify` | `kms:Verify`, `kms:GetPublicKey`, `kms:DescribeKey` | `kms:VerifyMac`, `kms:DescribeKey` |
| `both` | `kms:Sign`, `kms:Verify`, `kms:GetPublicKey`, `kms:DescribeKey` | `kms:GenerateMac`, `kms:VerifyMac`, `kms:DescribeKey` |

The policy holds one statement per action group, each only when it has a key, in
this order: `KmsSign`, `KmsVerify`, `KmsGenerateMac`, `KmsVerifyMac`,
`KmsPublicKey` (`kms:GetPublicKey` and `kms:DescribeKey` on the signing keys),
`KmsDescribeMacKey` (`kms:DescribeKey` on the HMAC keys). A workload connected
only to signing keys therefore has the same policy, byte for byte, as before
HMAC keys existed; `modules/kms_keys/tests/signing_keys.tftest.hcl` pins it.

Each key keeps the default key policy, which delegates to IAM in the account; no
other principal can use a key unless an IAM policy in the account grants it.

## The keys

| Setting | Value |
|---|---|
| Key usage | `SIGN_VERIFY`; `GENERATE_VERIFY_MAC` for an `HMAC_*` type |
| Key type | `key_spec` (`customer_master_key_spec`) |
| Description | `<project> <env> signing key <name> (meroku)`; `… HMAC key …` for an HMAC key |
| Alias | `alias/<project>-<env>-<name>` |
| Deletion window | 7 days |
| Rotation | none: AWS does not rotate asymmetric or HMAC keys automatically |

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

## HMAC keys

An HMAC key is for machine-to-machine requests between workloads that run in
AWS. The caller asks KMS for a MAC over the request (`GenerateMac`) and sends it
along; the receiver asks KMS to check it (`VerifyMac`). Both sides call KMS: the
secret never leaves KMS, so there is no public key and no
`KMS_<NAME>_PUBLIC_KEY`. That is also the limit: a partner outside AWS, or
outside this account's IAM, cannot check the MAC. For anything a third party
verifies, use a signing key.

```
aws kms generate-mac \
  --key-id "$KMS_REQUESTS_KEY_ARN" \
  --message fileb://request.bin \
  --mac-algorithm HMAC_SHA_256

aws kms verify-mac \
  --key-id "$KMS_REQUESTS_KEY_ARN" \
  --message fileb://request.bin \
  --mac-algorithm HMAC_SHA_256 \
  --mac fileb://mac.bin
```

The MAC algorithm matches the type: `HMAC_SHA_224` / `_256` / `_384` / `_512`
for `HMAC_224` / `_256` / `_384` / `_512`. A message is at most 4096 bytes; MAC a
hash of a larger body. Give the caller `sign` and the receiver `verify`; a
workload that does both gets `both`.

- No public key variable, and no `kms:GetPublicKey` grant: KMS has no public key
  to return, and `modules/kms_keys/public_keys` is passed only signing keys.
- No rotation: AWS does not rotate HMAC keys automatically. Move to a new key the
  same way as [changing the key type](#changing-the-key-type).
- The usage cannot change after creation: a signing key cannot become an HMAC
  key or the other way round (next section).
- Cost: about $1 per key per month, like any KMS key, plus per-request charges
  for `GenerateMac` and `VerifyMac`.

## Changing the key type

AWS cannot change the type or the usage of an existing key. Replacing the key
would make every signature or MAC made with it stop verifying, so
`modules/kms_keys` never does: `aws_kms_key` has
`lifecycle { ignore_changes = [customer_master_key_spec, key_usage] }`. The usage
follows from the type, so the type check below also catches a switch between a
signing key and an HMAC key. Grants and variables follow the type the key has in
AWS, not the changed `key_spec`, so the workloads keep working.
Once an apply has created a key, a changed `key_spec` in the YAML changes
nothing in AWS, and the plan shows a warning from the check
`key_spec_unchanged`:

```
KMS key "auth" is ECC_NIST_P256 in AWS, but kms_keys sets RSA_2048. AWS cannot
change a key's type: add a new key with the type you want, move the workloads
to it, then retire this one. Until then the key keeps ECC_NIST_P256.
```

The warning never fails the plan or the apply. Until an apply creates the key,
`key_spec` is free to change.

The web UI offers the type only for a key added since the panel was opened; a
key already in the YAML when the panel opened shows its type read-only.

To move to another type:

1. Add a new key with the type you want.
2. Move the workloads' connections to it and apply.
3. Retire the old key once nothing verifies with it any more (see
   [Removing a key](#removing-a-key)).

## Removing a key

Removing a key takes two applies, so nothing loses a key it still uses:

1. **Retire**: set `retired: true` and remove every connection to the key (the
   web UI's "Retire" does both). Apply. The variables and the IAM grants go; the
   key still exists, so a signature it made can still be verified with its
   public key, and anything outside meroku that uses it keeps working.

   The apply removes the workloads' grants at once, while the tasks it
   redeploys start without the variables. A task still running the old task
   definition loses `kms:Sign` and `kms:Verify` on the key as soon as the policy
   changes. Stop the application using the key, deploy that, then retire it.
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

A key's Terraform address is its name,
`module.kms_keys.aws_kms_key.key["<name>"]`. Deleting a key's entry and adding
a new entry with the same name before the next apply is, to Terraform, the same
key: the old key stays, its deletion is not scheduled, and with a different
`key_spec` the plan warns as in [Changing the key type](#changing-the-key-type).
To get a fresh key under a name, apply the deletion first, or use a new name.

## Destroy

`meroku destroy` destroys every key, retired ones included, which schedules each
for deletion with the 7-day window above. Cancel within the window to keep a
key.

## Cost

About $1 per key per month (AWS bills asymmetric and HMAC keys the same as
symmetric ones), retired keys included, plus per-request charges for `Sign`,
`Verify`, `GetPublicKey`, `GenerateMac` and `VerifyMac`, a few cents per 10,000
requests depending on the key type (see AWS KMS pricing). Verifying a signature
locally with the PEM makes no request; checking a MAC always does.

## Custom Terraform

A project's root-level custom files may read `module.kms_keys.key_arns`,
`module.kms_keys.environment` and `local.bridge.kms_key_arns`. A custom `.tf`
that declares its own `module "kms_keys"` is refused at generate; rename it.
