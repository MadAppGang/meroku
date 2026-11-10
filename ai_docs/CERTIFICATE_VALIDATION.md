# Certificate validation and DNS checks

ACM validates a certificate by looking up a CNAME through AWS's own resolvers.
Nothing outside AWS can query those resolvers or their caches, so meroku reads
the two things it can: ACM's verdict, and the lookup a resolver with an empty
cache makes.

## `meroku cert check <env>`

Code: `app/acm_validation_check.go`, `app/cert_cli.go`.

1. Lists the account's `PENDING_VALIDATION` certificates for the environment's
   zone, in the environment's region and in `us-east-1` (CloudFront, Amplify).
2. Reads each domain's `ValidationStatus` and validation record from
   `DescribeCertificate`. `SUCCESS` and `FAILED` are ACM's final word.
3. For a pending domain, asks the registry which nameservers the zone is
   delegated to, then asks those nameservers directly for the CNAME.

| Verdict | Meaning |
|---|---|
| issued | ACM validated it |
| failed | ACM gave up; only a new certificate request validates again |
| not delegated | the registry delegates the zone nowhere, or its nameservers do not answer |
| record missing | the delegated nameservers have no validation record: ACM reads a different zone than the one meroku wrote to |
| record wrong | the record exists with another value |
| waiting on ACM | the record is correct at the authoritative servers; ACM has not looked again |
| unchecked | port 53 is intercepted on this network (see below) |

When pending, the check also asks the public resolvers over HTTPS. Resolvers
that hold an older delegation of the zone fail the lookup until the TLD's TTL
on that delegation expires, and the resolvers ACM uses may do the same.

During an apply the same check runs every 30 s while an
`aws_acm_certificate_validation` resource is in flight
(`app/acm_apply_watch.go`). Its one-line summary goes on that resource's status
line and into the log when it changes.

## Intercepted DNS

Some networks answer every query to port 53 themselves, whatever server it was
addressed to, over UDP and TCP. An answer from such a proxy is a cached
recursive answer, not the server's. Registries and Route53 nameservers never
offer recursion, so `delegationAtParent` and `authoritativeCNAME` reject any
answer with the RA flag (and the CNAME lookup also requires AA) as
`errPort53Intercepted`.

Then:

- The delegation of a domain directly under a TLD is read from the registry
  over RDAP (`app/dns_rdap.go`, bootstrap from IANA). A delegated subdomain
  stays unchecked.
- `meroku cert check` reads the validation CNAME through the DNS-over-HTTPS
  resolvers and compares its value with ACM's.

## The canary: measuring propagation on the DNS screen

Matching nameservers say a resolver has the delegation, not that it can read
the zone. The DNS screen writes a wildcard TXT record,
`*._meroku-check.<zone>`, with a random token, and waits until Route53 reports
it applied (`app/dns_canary.go`). Each check asks the DoH resolvers for a new
name under it. A resolver counts once it returns the token. Two are enough to
continue.

A new name each round matters. A resolver that asked before the record
existed, or while it still followed an old delegation, caches "no such name"
for up to the SOA minimum (900 s on Route53), for that name only.

The record is removed when the screen closes, whatever the outcome. If it
cannot be written, the screen falls back to comparing nameservers.

Live check against a real zone:
`MEROKU_LIVE_CANARY=<profile>:<zoneID>:<zone> go test -run TestCanaryLive ./app`.

## Which zone the pre-flight checks

With `create_domain_zone: true`, the zone the apply writes every record into
is the one terraform state holds at `module.domain.aws_route53_zone.domain[0]`.
Route53 allows several public zones with one name, so the pre-flight takes the
zone from state, not from a lookup by name (`app/dns_zone_ownership.go`):

| Terraform state | Public zones with the name | Pre-flight |
|---|---|---|
| holds zone X | X | checks delegation to X |
| holds nothing | none | phase 1 creates the zone |
| holds nothing | Y | imports Y, then checks |
| holds X | X and Y, delegation points at Y | `state rm` X, imports Y, then checks |
| holds nothing | several, delegation points at none | stops and names them |
| unreadable (not initialised) | any | checks the delegated zone, imports nothing |

The delegation is read from the registry or parent, not a resolver cache.
A zone that keeps the delegation wins over the one in state because moving a
delegation means waiting for caches again; moving records is one apply. The
deploy imports before it continues (`adoptDomainZone`), then runs the
pre-flight again. Other same-name zones are listed with the command that
deletes them once the apply has moved their records out.

Without this, a zone left in AWS after state was cleared passed the
pre-flight, and the apply created a second zone and wrote the validation
record into it, where ACM could never read it.

## Root domains: the registrar

A root domain is delegated by the nameserver list at its registrar
(`app/dns_registrar.go`). The DNS screen asks Route 53 Domains in every local
profile for the registration. When found, it sets the nameservers without a
keypress if the zone the registrar points at holds only NS and SOA, and asks
first when that zone serves other records. The old zone is left alone:
resolvers that cached the old nameservers fail the lookup either way, with "no
such record" from the zone or a refusal once it is deleted.

## State lock held by another run

A destroy that fails with "Error acquiring the state lock" shows who holds the
lock and whether that run is still alive (`app/state_lock.go`). When the lock
was taken on this machine and a terraform process still runs in `env/<env>`,
`[s]` interrupts it (terraform cancels cleanly and releases the lock) and the
destroy runs again. When no such process is left, `[u]` runs
`terraform force-unlock` and the destroy runs again. A lock taken on another
machine is only explained.
