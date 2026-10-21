# Meroku V4: the decisions we made for you

Setting up a backend on AWS means choosing between something like 150 options.
Fargate or EC2. API Gateway or a load balancer. NAT Gateway, NAT instance, VPC
endpoints or public IPs. Each choice changes the bill, the security and the
amount of work somebody does every month. Most teams choose without calculating
the cost, and see the result later on their AWS bill.

Meroku makes those choices for you.

It's Heroku or Vercel, but on top of your own AWS account. For each concern you
get about three ready-made choices instead of 150 options.

Meroku is built for small and mid-size projects. That matters, because AWS
writes its best practices for large companies. For them, maximum isolation
comes first and cost comes second. We follow AWS advice where it fits our size,
and we say so openly where we don't. Every choice follows the same five goals:

- **Secure by default.** Nothing is open to the internet unless it has to be.
- **The smallest possible bill.** An idle project costs a few dollars, not
  US$73 (A$133 in Sydney).
- **The least possible maintenance.** Nobody patches or restarts servers by
  hand.
- **The full power of AWS.** It's plain Terraform in your account. If a choice
  doesn't fit, you add your own Terraform through the `custom_pre` and
  `custom_post` modules, or override the AMI, user data or IAM policies on a
  pool.
- **Everything automatic.** If meroku can set something up by itself, it does.
  A decision is one click in the UI or one line in YAML.

This document lists the decisions behind V4, why we made each one, and where V4
doesn't meet these goals yet. The fixes are in `ARCHITECTURE_V5_PLAN.md`.

**About the prices.** Every price is shown twice: us-east-1 in US dollars
(US$) and Sydney (ap-southeast-2) in Australian dollars (A$). Both come from
the AWS Price List API on 2026-09-30. AWS lists Sydney prices in USD, so we
convert them at **1 USD = 1.4277 AUD**, the European Central Bank reference
rate of 2026-09-29. AWS bills in USD. If you pay in AUD, AWS uses its own daily
rate, so your invoice will differ a little. A month is 730 hours. Numbers
marked "approximate" aren't checked against AWS pricing yet.

## The choices you actually make

| Concern | Your choices | Default |
|---|---|---|
| Where a service runs | `fargate` or `ec2` (a compute pool) | `fargate` |
| How an EC2 pool buys machines | `on_demand`, `spot_with_base`, `spot` | set per pool |
| How tasks reach the internet | `public_ip`, `nat_gateway`, `nat_gateway_ha` | `public_ip` |
| How requests come in | API Gateway, or a load balancer (ALB) | API Gateway |

Meroku decides everything else. You never pick a subnet layout, a service
discovery tool or a security group rule.

---

## 1. Fargate by default, EC2 when you need it

**The choices.** `fargate` for almost everything. `ec2` when a service needs
GPUs, runs at a steady load, or needs local disk.

**Why we chose it.** Fargate needs no maintenance. There are no servers, and
you pay per running task. But Fargate has no GPUs. And a service that runs 24/7
pays the Fargate premium every hour.

GPUs were the main reason for adding EC2. We want to run our own AI models.
Model serving runs all the time and needs a GPU machine that is always on and
scales with traffic. Training runs on demand. It needs GPU machines that start
when a job arrives and stop when the job ends.

So we added EC2 **compute pools** to the same cluster. The setup is one line:
`backend_runtime: ec2` creates a pool with default settings. The pool uses
three similar instance types, so AWS rarely runs out of capacity. ECS adds and
removes machines between `min_size` and `max_size`. A pool can scale to zero,
so an idle GPU pool costs nothing. For a lower bill, a pool can use spot
machines. AWS can take a spot machine back with two minutes' notice
(approximate), so always-on services keep at least one on-demand machine.

**The analysis.**

| What Fargate can't do | Why it matters |
|---|---|
| Cost at steady load | A 24/7 service pays the Fargate premium every hour, roughly 20 to 40 percent over the same EC2 capacity (approximate). |
| Hardware | No GPU, limited memory-to-CPU ratios, about 16 vCPU and 120 GB per task at most (approximate). |
| State | No EBS volumes, no host mounts, no one-per-host daemons. |
| Cold start | Every task attaches its own network card at start. EC2 machines that are already running start tasks faster. |

| AI workload | Load | What the pool needs |
|---|---|---|
| Model serving | Always on, varies with traffic | `min_size` of 1 or more, autoscaling, `spot_with_base` with at least one on-demand machine |
| Training and fine-tuning | On demand, hours per run | `min_size: 0`, `spot`, and jobs that save checkpoints to S3 or EFS |

| `capacity_type` | What the pool buys | Fits |
|---|---|---|
| `on_demand` | Only on-demand machines | Services that can't take any interruption |
| `spot_with_base` | `on_demand_base` on-demand machines, spot above that | Always-on services: the base stays up, spot handles peaks |
| `spot` | Only spot machines | Jobs that can restart from a checkpoint |

**The downside.** AWS patches Fargate hosts. With EC2, patching is our job.

**What doesn't work yet.**

- A pool gets a new AMI only when someone runs `terraform apply`. A pool that
  nobody touches stays unpatched. That goes against "least maintenance".
- No task can reserve a GPU yet, so two tasks can share one GPU.
- Training jobs can't use a pool, because scheduled and event tasks run only on
  Fargate.

---

## 2. Every task needs a public IP, so EC2 shares one

The user never sees this decision, but it saves the most money per task.

A task can't start without internet access. It pulls its image from ECR, sends
logs to CloudWatch and reads secrets from SSM. In the default setup there is no
NAT Gateway, so the only way out is the Internet Gateway. The Internet Gateway
only forwards traffic from an address with a public IP. So every Fargate task
gets a public IP.

Since February 2024, AWS charges US$0.005 an hour for each public IPv4
address, in every region. That is US$3.65 a month, or A$5.21 in Sydney. Every
Fargate task pays it.

On EC2, the network mode decides how many addresses you pay for (A$ at
1 USD = 1.4277 AUD):

| 8 small tasks on... | Public IPs | us-east-1, per month | Sydney, per month |
|---|---|---|---|
| Fargate | 8, one per task | US$29.20 | A$41.69 |
| One EC2 machine, `bridge` mode | 1, shared | US$3.65 | A$5.21 |
| One EC2 machine, `awsvpc` mode | 0, they can't get one | nothing works without a NAT at US$36.50 | nothing works without a NAT at A$66.70 |

AWS recommends `awsvpc`, and our first plan used it. It didn't work in our
setup. In `awsvpc` mode on EC2, each task gets its own network card, and AWS
never gives that card a public IP. Without a NAT, the task's own traffic can't
reach the internet. The machine still pulls the image and sends the logs
through its own network card. So the task starts and passes its health check.
Only the app's own calls, to S3 or Stripe, time out. Every dashboard says
"healthy", and the app doesn't work.

`bridge` mode doesn't have this problem. Tasks use the machine's network card
and its one public IP. It also fits about 8 small tasks on an `m7i-flex.large`,
where `awsvpc` fits 2.

So pools use `bridge` by default, and there is no setting for it. Meroku
refuses `awsvpc` unless you confirm the subnets have a NAT.

**The analysis.** How the three ways to run a task compare:

| | `bridge` on EC2 | `awsvpc` on EC2 | `awsvpc` on Fargate |
|---|---|---|---|
| Network cards (interfaces) | None per task. All tasks share the machine's card. | One per task, created by ECS | One per task |
| Public IP | One per machine, shared by its tasks | **None.** AWS never gives a task card on EC2 a public IP. | One per task |
| Public IPv4 cost for 8 tasks | US$3.65 (A$5.21) | US$0, but no internet without a NAT | US$29.20 (A$41.69) |
| Internet access without a NAT | Yes, through the machine | **No.** The app's calls time out without an error. | Yes |
| Task address | The machine's IP plus a random port | Its own private IP | Its own private IP |
| Ports | Random host port, 32768 and up | The container port | The container port |
| Security group | One per machine, shared | One per task | One per task |
| Tasks per `m7i-flex.large` | About 8 small tasks, limited by CPU and memory | 2, or 4 with ENI trunking | Not applicable |
| Load balancer target | Machine plus port | Task IP | Task IP |
| Cloud Map record | SRV only, because the port is random | A and SRV | A and SRV |
| Sidecars over `localhost` | Don't work | Work | Work |

Why the two modes exist:

| Mode | Where it came from | What it was built for |
|---|---|---|
| `bridge` | Docker's default network. ECS took it over at launch. | Many containers on one machine, sharing its network card. |
| `awsvpc` | Added by AWS in November 2017. | Each task as a full VPC member, with its own security group, flow logs and IP. The price is one network card per task, and machines have few. |
| ENI trunking | Added by AWS in 2019. | Raising the card limit for `awsvpc`. It needs an account setting, and t2, t3 and t4g machines don't support it. |

**The downside.** All tasks on one machine share one security group. A
separate firewall rule set for each task is the main thing `awsvpc` gives you.
We don't need it. In meroku, services trust each other inside the VPC. The
real protection is at the entry point, the database and the secrets. A
firewall per task would add work and cost, and it protects against a risk our
projects don't have. For our projects, `bridge` is the better option.

Yes, AWS recommends `awsvpc`. AWS writes its recommendations for large
companies, where isolation comes first and the bill comes second. Meroku is
built for small and mid-size projects, where the bill matters more. Following
the AWS advice here would give a small team a US$36.50 (A$66.70) NAT bill
instead of one US$3.65 (A$5.21) IP, and a quarter of the tasks per machine.

One thing to know: anything inside the VPC can reach the random ports (32768
to 65535) on a pool machine. The tracing sidecar also can't run in this mode
(decision 6).

**What doesn't work yet.** We wrote the `awsvpc` refusal one day before NAT
support existed. It still refuses `awsvpc` in NAT mode, where `awsvpc` works.
Its error message also says the VPC has no NAT, which is wrong in NAT mode.

---

## 3. Public IPs first, a NAT Gateway later

**The choices.** `public_ip`, `nat_gateway`, `nat_gateway_ha`. Three options,
each the cheapest at a different size.

**Why we chose it.** A public IP costs US$3.65 (A$5.21) per task and nothing
per GB. A NAT Gateway costs US$36.50 (A$66.70) a month, including the public IP that
each NAT needs for itself, plus US$0.045 (A$0.0842) per GB, and nothing per
task. More services make the NAT cheaper. More traffic
makes public IPs cheaper. With 2 tasks per service, one NAT becomes cheaper at
about 6 services in us-east-1. At 5 services it still costs US$40.90 against
US$36.50 for public IPs. In Sydney a NAT costs more and a public IP doesn't,
so the NAT becomes cheaper later, at about 8 services.

**The analysis.** Unit prices (A$ at 1 USD = 1.4277 AUD):

| Item | us-east-1 rate | us-east-1 per month | Sydney rate | Sydney per month |
|---|---|---|---|---|
| Public IPv4 address | US$0.005 per hour | US$3.65 | A$0.0071 per hour | A$5.21 |
| NAT Gateway | US$0.045 per hour, plus US$0.045 per GB | US$32.85 plus traffic | A$0.0842 per hour, plus A$0.0842 per GB | A$61.49 plus traffic |
| Public IP of the NAT Gateway, one per NAT | US$0.005 per hour | US$3.65 | A$0.0071 per hour | A$5.21 |
| NAT instance (fck-nat or similar): a `t4g.nano`, its public IP, an 8 GB disk | US$0.0042 per hour for the machine | about US$7.36 | A$0.0076 per hour for the machine | about A$11.83 |
| Interface VPC endpoint, per zone | US$0.01 per hour, plus US$0.01 per GB | US$7.30 | A$0.0186 per hour, plus A$0.0143 per GB | A$13.55 |
| Traffic between zones | US$0.01 per GB each way | none | A$0.0143 per GB each way | none |
| S3 gateway endpoint | free | US$0 | free | A$0 |
| IPv6 with an egress-only gateway | free | US$0 | free | A$0 |

Each option gets more expensive with only one thing:

| Option | Grows with tasks | Grows with traffic | Fixed cost |
|---|---|---|---|
| Public IP per task | yes, US$3.65 or A$5.21 each | no | none |
| NAT Gateway | no | yes, US$0.045 or A$0.0842 per GB | US$36.50 or A$66.70 per NAT, with its public IP |
| NAT instance | no | in steps | about US$7.36 or A$11.83 |
| VPC endpoints | no | a little | US$7.30 or A$13.55 per endpoint per zone |
| IPv6 | no | no | none |

How the cost grows, with 2 tasks per service across 2 zones. One NAT also pays
for the half of the traffic that comes from the other zone. A NAT per zone
doesn't.

us-east-1, US$ per month:

| Services | Tasks | Traffic | Public IPs | One NAT | NAT per zone |
|---|---|---|---|---|---|
| 1 | 2 | 20 GB | **US$7.30** | US$37.60 | US$73.90 |
| 3 | 6 | 50 GB | **US$21.90** | US$39.25 | US$75.25 |
| 5 | 10 | 80 GB | **US$36.50** | US$40.90 | US$76.60 |
| 8 | 16 | 120 GB | US$58.40 | **US$43.10** | US$78.40 |
| 12 | 24 | 180 GB | US$87.60 | **US$46.40** | US$81.10 |
| 20 | 40 | 300 GB | US$146.00 | **US$53.00** | US$86.50 |

Sydney, A$ per month (1 USD = 1.4277 AUD):

| Services | Tasks | Traffic | Public IPs | One NAT | NAT per zone |
|---|---|---|---|---|---|
| 1 | 2 | 20 GB | **A$10.42** | A$68.67 | A$135.09 |
| 3 | 6 | 50 GB | **A$31.27** | A$71.63 | A$137.62 |
| 5 | 10 | 80 GB | **A$52.11** | A$74.58 | A$140.14 |
| 8 | 16 | 120 GB | A$83.38 | **A$78.52** | A$143.51 |
| 12 | 24 | 180 GB | A$125.07 | **A$84.43** | A$148.57 |
| 20 | 40 | 300 GB | A$208.44 | **A$96.26** | A$158.67 |

When a NAT becomes cheaper than public IPs:

| | us-east-1 | Sydney |
|---|---|---|
| One NAT | from about 6 services | from about 8 services |
| A NAT per zone | from about 11 services | from about 15 services |

At 20 services, one NAT saves about US$93 a month per environment in
us-east-1, and about A$112 in Sydney. Meroku always creates the S3 gateway
endpoint, which is free. ECR stores image layers in S3. So the layers, the
biggest part of the traffic, never pass through the NAT. Only the small ECR
API calls do.

The resulting choices:

| Size | Choice | us-east-1, per month | Sydney, per month |
|---|---|---|---|
| 1 to 5 services | `public_ip` | US$7 to US$37 | A$10 to A$52 |
| 6+ services, non-production | `nat_gateway` | about US$42 to US$53 | about A$76 to A$96 |
| 11+ services, production | `nat_gateway_ha` | about US$80 to US$87 | about A$147 to A$159 |

The service counts in this table come from us-east-1 prices. The advisor uses
the same counts in every region.

Production gets one NAT per availability zone. With a single NAT, all outbound
traffic stops when its zone goes down.

"Why not just use a NAT from day one, Jack?" you'll ask me. It's a fair
question. A project with one service would pay US$38 a month (A$69 in Sydney)
to reach the internet, before it has a single user. That goes against
"smallest bill".

We rejected three other options:

- **A NAT instance** is the cheapest at every size, about US$7 (A$12) a month.
  But it's a server, and someone has to patch it. That goes against "least
  maintenance".
- **VPC endpoints** cost US$58 (A$108) a month for the minimum set, and they
  still don't give access to the internet.
- **IPv6 only** is free, but it fails as soon as an external API has no IPv6
  address.

The switch is one line. Terraform adds the private subnets, the NAT and the
routes, and moves the tasks. A built-in advisor tells you when the switch
saves money. Tasks behind a NAT also have no public address, so the cheaper
option at scale is also the safer one.

**What doesn't work yet.**

- The advisor only recommends. You still have to notice the advice and make
  the switch yourself.
- The advisor calculates with us-east-1 prices. In Sydney the NAT only becomes
  cheaper at about 8 and 15 services, so the advisor recommends the switch
  about two services too early there. Switching at 6 services costs A$13.37 a
  month more than staying on public IPs. Switching production at 11 costs
  A$32.66 more.

---

## 4. API Gateway by default, a load balancer when you need it

**The choices.** API Gateway, or the ALB. (A$ at 1 USD = 1.4277 AUD.)

| | API Gateway (HTTP API) | ALB |
|---|---|---|
| Cost when idle | nothing | US$16.43 a month in us-east-1, A$26.26 in Sydney, plus usage |
| Billing | per request: US$1.00 per million in us-east-1, A$1.84 in Sydney | per hour, plus US$0.008 (A$0.0114) per capacity-unit hour |
| Free tier | about 1 million requests a month, first 12 months (approximate) | none |
| Request limit | 30 seconds, fixed | configurable |
| SSE, WebSocket | no | yes |
| Sticky sessions | no | yes, by cookie |
| Rate limiting | built in | no |
| How it finds tasks | VPC link, then Cloud Map | target groups with health checks |

**Why we chose it.** With API Gateway, a new project pays nothing when there
are no requests. When a service needs streaming or long requests, you turn on
the ALB for that environment. The public API address stays the same, so
clients don't need any change.

What exists in each mode:

| | API Gateway mode | ALB mode |
|---|---|---|
| API Gateway, its stage and VPC links | yes | yes, kept |
| Backend route and custom domain on API Gateway | yes | removed |
| ALB, listeners, target groups, path rules | no | yes |
| DNS for the API address | points to API Gateway | points to the ALB |

When the ALB is off, meroku creates no ALB. Only its security group stays, and
security groups are free.

**What doesn't work yet.** The switch is one line, but it isn't safe yet.

- API Gateway removes the `/service-name` prefix from the URL, and the ALB
  keeps it. A service that works today returns 404 after the switch.
- Named services keep their API Gateway routes after the switch, so they are
  reachable in two ways.
- SNS webhooks still point at API Gateway, so they stop working.
- There is no WAF. The ALB is open to the whole internet, even when CloudFront
  is in front of it.

---

## 5. Cloud Map connects services automatically

Nobody picks a service discovery tool in meroku. Every environment gets a
private DNS zone through AWS Cloud Map, and ECS keeps it up to date. When a
task starts, ECS adds its address. When the task stops, ECS removes it. It
works like a phone book: a caller asks for `orders`, gets up to 8 healthy
addresses and picks one.

API Gateway uses the same phone book to reach tasks, through a VPC link that
costs nothing. Internal calls use Cloud Map directly. We don't send internal
traffic through the ALB:

| | Cloud Map (chosen) | Through the ALB |
|---|---|---|
| Extra hop | none | one |
| Extra cost | none | a charge per request |
| When the ALB fails | internal calls still work | internal calls fail too |
| Spreading load | the caller picks, weak | round robin with health checks |
| During a deploy | up to 10 seconds of stale addresses | clean connection draining |

Cloud Map stays on even in ALB mode. AWS refuses to remove it from a running
service, so turning it off would break the switch.

**The downside.** Cloud Map only checks if a task is healthy. It doesn't check
how busy the task is. After a deploy, a stopped task can still get calls for up
to 10 seconds.

**What doesn't work yet.** Each task gets the backend's address and its own.
To call another service, a developer builds its DNS name by hand.

---

## 6. Tracing is one setting

`xray_enabled: true` adds a tracing collector next to your app, on Fargate and
on EC2. It's off by default, so nobody pays for traces they don't read.

**What doesn't work yet.**

- Tracing doesn't work in `bridge` mode, the default for EC2. The app looks for
  the collector at `127.0.0.1`, which in `bridge` mode is the app's own
  container. The traces would be lost without an error, so meroku refuses this
  combination.
- CloudWatch holds logs only. There are no alarms, so nobody hears about a
  failure until a user reports it.

---

## 7. Amplify for frontends

S3 serves static files only. A Next.js app with server-side rendering would
need us to run its server part ourselves. That goes against "least
maintenance". Amplify builds from Git, deploys each branch, manages the domain
and can run that server part.

**What doesn't work yet.** Meroku hard-codes the Amplify platform to `WEB`,
which is static only. So server-side rendering doesn't work yet, and that was
the reason we chose Amplify. Frontend build variables are also stored in plain
text.

---

## 8. One event bus, no setup

Apps publish events to the default EventBridge bus. Rules on it start
event-driven tasks. The deploy pipeline listens on it for new images and
config changes. We chose it for serious technical reasons: it was already
there. It also needs no setup at all.

**What doesn't work yet.**

- All environments in an account share one bus, and IAM lets one environment
  publish into another's events.
- Events aren't stored. A missed event is gone, and a retried publish runs the
  task twice.
- Editing an `.env` file should redeploy the service, but that trigger needs a
  CloudTrail trail that meroku doesn't create. Without it, nothing happens and
  nothing warns you. Meroku could create this trail automatically, and it
  doesn't.

---

## Security by default

These hold for every project without any setting:

- The database isn't reachable from the internet, and its storage is encrypted.
- Secrets live in SSM Parameter Store as encrypted values, never in YAML.
- Tasks accept traffic only from the entry point and from inside the VPC.
- GitHub Actions deploys through a short-lived OIDC role, so no AWS keys are
  stored in GitHub.

Two permissions are still too broad: the CI role can publish to any event bus,
and the scheduler role has full EventBridge access.

---

## Scorecard

How well each V4 decision meets the goals today:

| Decision | Smallest bill | Least maintenance | Secure | Automatic |
|---|---|---|---|---|
| 1. Fargate and EC2 pools | yes | **no**, patching is manual | yes | partly, no GPU reservation |
| 2. `bridge` and shared IPs | yes | yes | yes, per-task firewall rules not needed | partly, outdated refusal |
| 3. Public IPs, then NAT | yes | yes | yes | partly, advice only |
| 4. API Gateway or ALB | yes | yes | **no WAF** | **no**, the switch breaks URLs |
| 5. Cloud Map | yes | yes | yes | partly, manual service names |
| 6. Tracing | yes | yes | yes | partly, no alarms, none on `bridge` |
| 7. Amplify | yes | yes | partly, plaintext build values | **no**, no SSR |
| 8. Default event bus | yes | yes | partly, no isolation | **no**, missing CloudTrail trail |

V4 is strongest on the bill. It's weakest on automation.

---

## Where the details live

- `ai_docs/EGRESS_COST_MODEL.md`: the full public IP and NAT cost model.
- `ai_docs/EC2_COMPUTE_POOLS.md`: how pools work, and the live test of `bridge`.
- `ai_docs/EC2_FARGATE_RUNTIME_PLAN.md`: the original EC2 plan.
- `ai_docs/ARCHITECTURE_V5_PLAN.md`: what we fix next.
- Prices: AWS Price List API, us-east-1 and ap-southeast-2 offer files for
  EC2, VPC, ELB and API Gateway, fetched 2026-09-30.
- Exchange rate: European Central Bank reference rate via
  [Frankfurter](https://frankfurter.dev), 1 USD = 1.4277 AUD on 2026-09-29.
