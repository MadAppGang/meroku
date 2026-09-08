# aws_lambda_invocation.{backend,services}_revision — the edge that tells the CI
# Lambda a new ECS task-definition revision exists. Without it a configuration
# change reaches no container: every aws_ecs_service here ignores
# task_definition, so Terraform registers the revision and deliberately does not
# deploy it, and the Lambda cannot register that revision itself
# (RegisterRevisionWithImage clones the latest ACTIVE revision and copies
# container_definitions wholesale). backend.tf carries the full account.
#
# The contract, in the order the contract matters.
#
#   1. An auto-deployable target gets exactly one invocation, and the module
#      still PLANS with these resources in it.
#      (auto_deployable_targets_are_notified)
#   2. An opted-out target gets none. This is the whole reason the gate sits at
#      the resource: nothing downstream will stop the invocation.
#      (backend_opt_out_creates_no_invocation, and "billing" in run 1)
#   3. Opting out withholds the NOTIFICATION, never the target.
#      (opting_out_leaves_the_target_in_every_map)
#   4. The payload decodes to the keys handler.manualDetail decodes.
#      (the_payload_is_the_one_the_lambda_decodes)
#   5. The trigger is the task-definition REVISION.
#      (the_payload_is_the_one_the_lambda_decodes)
#
# Why this exists on top of the Go guard. The only other cover for these
# resources is TestEveryAutoDeployableServiceNotifiesOnANewRevision in
# ci_lambda/internal/boundary/lambdatf_guard_test.go, which parses backend.tf and
# services.tf as TEXT. Text proves the resource is written down. It cannot prove
# any of:
#
#   * that modules/workloads still plans with these resources present.
#     `terraform validate` does not evaluate count, for_each, jsonencode or a
#     precondition's error_message, so a resource that is syntactically perfect
#     and blows up at plan time passes every other gate in CI and fails in a
#     user's terminal — exactly how the v4.2.0 compute-pool null shipped through
#     four releases (modules/compute_pool_check/tests/messages.tftest.hcl).
#   * that the gate EVALUATES. Text alone cannot tell
#     `count = var.backend_auto_deploy ? 1 : 0` from
#     `count = var.backend_auto_deploy != null ? 1 : 0`; both contain
#     "auto_deploy" and only one of them gates anything. Planning both settings
#     of the flag is what separates them.
#   * that jsonencode PRODUCES the keys the guard read off the HCL. The guard's
#     detailKeys() regex matches `^\s*key =` in the source, and its character
#     class is lower-case only — so a camelCase key that encoding/json drops on
#     the floor is invisible to it. Here the JSON is decoded, so the assertion is
#     on the bytes the Lambda actually receives.
#
# Why mock_provider works when the CI comments say modules/workloads "can never
# be planned without AWS credentials". That is still true of `terraform plan`.
# `terraform test` with mock_provider never calls the provider at all: every
# remote data source returns generated values, so the graph walks with no
# account, no credentials and no network. What the mocks below pin is the
# handful of places where the AWS provider validates a value CLIENT-side, before
# it would ever reach the network, and a generated eight-character word is not a
# policy document, a CIDR block or an ARN.
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  # A generated mock string is not a policy document, and the provider rejects
  # it: seven resources fail with `"policy" contains an invalid JSON policy: not
  # a JSON object` before any assertion runs. The content is irrelevant to
  # everything asserted here — it only has to parse — so every
  # aws_iam_policy_document in the module gets the same empty-but-valid one.
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }

  # Same shape, one resource: aws_security_group.backend puts
  # data.aws_vpc.selected.cidr_block straight into cidr_blocks, and the provider
  # validates it as a CIDR block.
  mock_data "aws_vpc" {
    defaults = {
      cidr_block = "10.0.0.0/16"
    }
  }

  # ARNs, for the one run below that APPLIES rather than plans. The provider
  # validates an ARN wherever one resource's is an argument of another —
  # aws_ecs_task_definition's task_role_arn and execution_role_arn,
  # aws_iam_role_policy_attachment's policy_arn, aws_ecs_service's
  # service_registries.registry_arn, aws_cloudwatch_event_target's arn,
  # aws_lambda_permission's source_arn, aws_apigatewayv2_stage's
  # access_log_settings.destination_arn.
  #
  # Keyed on the PRODUCING type, so this is six entries rather than one per
  # consuming attribute, and adding a seventh consumer of an ARN already listed
  # here costs nothing. When a NEW producer's ARN starts being consumed the apply
  # run fails with `"<attribute>" (<junk>) is an invalid ARN`, naming the
  # attribute; follow it to the resource it reads and add that type here.
  #
  # Account 000000000000 is synthetic. This repository is public and no real
  # identifier belongs in it.
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::000000000000:role/mock"
    }
  }
  mock_resource "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::000000000000:policy/mock"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock"
    }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = {
      arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock"
    }
  }
  mock_resource "aws_lambda_function" {
    defaults = {
      arn = "arn:aws:lambda:us-east-1:000000000000:function:mock"
    }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:rule/mock"
    }
  }
}

# null_resource.build_ci_lambda (lambda.tf) and the random_* resources. Nothing
# here is asserted on; they are mocked only so the suite needs no provider that
# reaches out.
mock_provider "null" {}
mock_provider "random" {}

# The smallest input set modules/workloads accepts. Every value is synthetic.
variables {
  # project and env travel verbatim into the invocation payload, where manual.go
  # compares them against the Lambda's own config and IGNORES an event whose
  # project or env does not match. They are asserted, so they are stated.
  project = "acme"
  env     = "dev"

  # Required by the module and never dereferenced by anything under test — they
  # are handed to resources the mocked provider accepts without looking.
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"

  # Required with no default ON PURPOSE (variables.tf:96): an omitted value must
  # fail rather than fall back to a wildcard that lets any repository assume the
  # deploy role. So a test has to state one; it is never read here.
  github_subjects = ["repo:acme/acme:ref:refs/heads/main"]

  # The three cases the services gate has to tell apart, and all three matter:
  #
  #   orders  — auto_deploy explicitly true.
  #   billing — auto_deploy explicitly false. The opt-out.
  #   search  — auto_deploy ABSENT. Absent is true (variables.tf:381), because a
  #             project that predates the setting auto-deployed everything and
  #             must keep doing so. A gate written as `service.auto_deploy ==
  #             true` against a null would silently stop deploying every service
  #             in every project that has not been re-rendered since.
  services = [
    { name = "orders", auto_deploy = true },
    { name = "billing", auto_deploy = false },
    { name = "search" },
  ]
}

# ---------------------------------------------------------------------------
# The gate. Four runs, all `command = plan`.
#
# They come FIRST deliberately. A failed ASSERTION lets the remaining runs
# proceed, but an ERROR — a plan or apply that does not complete — SKIPS every
# run after it. The payload run at the bottom is the one that applies against
# mocks, which makes it by far the likeliest source of such an error from an
# unrelated change elsewhere in this large module: a new resource consuming a
# validated ARN is enough. Ordered the other way round, that change would take
# the auto-deploy gate's cover down with it, and the four runs it silenced would
# be reported as "skip" rather than as anything anyone reads.
# ---------------------------------------------------------------------------

# Runs 1 and 2 differ in exactly one variable. That is the point of the pair: the
# gate is a boolean, and a suite that only ever plans one setting of it cannot
# tell a real gate from one that is always true.
run "auto_deployable_targets_are_notified" {
  command = plan

  variables {
    backend_auto_deploy = true
  }

  assert {
    condition     = length(aws_lambda_invocation.backend_revision) == 1
    error_message = "backend_auto_deploy is true, so exactly one aws_lambda_invocation.backend_revision must be planned. Without it Terraform registers a task-definition revision that no component ever deploys: aws_ecs_service.backend ignores task_definition, and the Lambda's RegisterRevisionWithImage clones the latest ACTIVE revision — so a configuration change reaches no container while every component reports success."
  }

  # The whole key set, compared as a set. A `contains` check would be satisfied
  # by a gate that notified everything, which is one of the mutations this file
  # exists to catch.
  assert {
    condition     = sort(keys(aws_lambda_invocation.services_revision)) == tolist(["orders", "search"])
    error_message = "Exactly the services with auto_deploy on must get an invocation: \"orders\" (explicitly true) and \"search\" (absent, which means true). \"billing\" is explicitly false and must get none — manual.go deliberately does not consult auto_deploy, so an ungated apply deploys a service whose operator opted out. Got: ${jsonencode(sort(keys(aws_lambda_invocation.services_revision)))}"
  }

  # The flip side of the same rule, stated separately because it is a different
  # failure: the gate belongs on the NOTIFICATION, not on the service. All three
  # services keep their task definition; only two of them are announced.
  assert {
    condition     = length(aws_ecs_task_definition.services) == 3
    error_message = "auto_deploy is a flag, never a filter. Terraform still owns the shape of an opted-out service and still registers its revisions; only the CI notification is withheld. Got ${length(aws_ecs_task_definition.services)} task definitions for 3 services."
  }

  # The ordering edge these resources deliberately do NOT state with a
  # depends_on: naming the function through the resource is what puts the
  # invocation after aws_lambda_function.lambda_deploy and, through that
  # function's ECS_SERVICE_MAP, after the ECS service it is about to deploy.
  # boundary/lambdatf_guard_test.go asserts the reference exists in the text;
  # this asserts it still resolves to the same function once evaluated.
  assert {
    condition     = aws_lambda_invocation.backend_revision[0].function_name == aws_lambda_function.lambda_deploy.function_name
    error_message = "The invocation must name aws_lambda_function.lambda_deploy through the resource. It is the only thing ordering the invocation after the ECS service on the first apply of a new environment; without it the Lambda answers ServiceNotFoundException, which retry.go classifies non-retryable and the handler reports as \"ignored\" with a nil error — a green apply that deployed nothing."
  }
}

# The opt-out. This is the assertion the resource-level `count` exists for: the
# Lambda's manual path (handler/manual.go) deliberately does NOT consult
# auto_deploy, because a DEPLOY event is somebody asking for that exact
# deployment and turning off automatic deploys in prod must not also take away
# the button that deploys prod. Nothing downstream will stop this invocation, so
# the only place the policy can be applied is by not creating the resource.
run "backend_opt_out_creates_no_invocation" {
  command = plan

  variables {
    backend_auto_deploy = false
  }

  assert {
    condition     = length(aws_lambda_invocation.backend_revision) == 0
    error_message = "backend_auto_deploy is false, so NO aws_lambda_invocation.backend_revision may be planned. handler/manual.go does not consult auto_deploy — it treats a SERVICE_DEPLOY event as an explicit request — so an invocation created here rolls a backend whose operator switched automatic deploys off, on a plain terraform apply. Got ${length(aws_lambda_invocation.backend_revision)}."
  }
}

# auto_deploy is a flag, never a filter (lambda.tf:865-882). Every target stays
# in every map and every repository stays in the ECR event rule, so a push to a
# disabled target still invokes the Lambda and still writes a line naming the
# reason: "auto_deploy is disabled for billing", not "no target uses repository
# acme_service_billing", which would be a lie that reads like a naming bug.
#
# Without this run the two above could be satisfied by dropping opted-out targets
# from the module altogether — which would pass, and would turn every disabled
# service into an unexplained silence.
run "opting_out_leaves_the_target_in_every_map" {
  command = plan

  variables {
    backend_auto_deploy = false
  }

  # The identifier is read from the same contract file the Lambda embeds rather
  # than written out as "backend". Terraform once wrote "backend" while Go
  # expected "", and nothing compared them; a literal here would be a third place
  # for that to happen again.
  assert {
    condition     = contains(keys(jsondecode(aws_lambda_function.lambda_deploy.environment[0].variables["ECS_SERVICE_MAP"])), jsondecode(file("${path.module}/ci_lambda/contract/contract.json")).backend_id)
    error_message = "backend_auto_deploy = false must not remove the backend from ECS_SERVICE_MAP. The flag travels to the Lambda as data so it can say why it did nothing; removing the entry makes it say the target is unknown instead. Got: ${aws_lambda_function.lambda_deploy.environment[0].variables["ECS_SERVICE_MAP"]}"
  }

  assert {
    condition     = jsondecode(aws_lambda_function.lambda_deploy.environment[0].variables["AUTO_DEPLOY_MAP"])[jsondecode(file("${path.module}/ci_lambda/contract/contract.json")).backend_id] == false
    error_message = "AUTO_DEPLOY_MAP must carry the backend's policy as false, not omit the backend. config.SelfCheck asserts this map's key set against ECS_SERVICE_MAP and SCHEDULED_TASK_MAP, so an omission fails at runtime instead of quietly defaulting. Got: ${aws_lambda_function.lambda_deploy.environment[0].variables["AUTO_DEPLOY_MAP"]}"
  }

  # And the same for the service that opted out, which is still in var.services
  # from the file-level variables block.
  assert {
    condition     = jsondecode(aws_lambda_function.lambda_deploy.environment[0].variables["AUTO_DEPLOY_MAP"])["billing"] == false
    error_message = "AUTO_DEPLOY_MAP must carry billing => false. Got: ${aws_lambda_function.lambda_deploy.environment[0].variables["AUTO_DEPLOY_MAP"]}"
  }

  assert {
    condition     = contains(keys(jsondecode(aws_lambda_function.lambda_deploy.environment[0].variables["ECS_SERVICE_MAP"])), "billing")
    error_message = "billing has auto_deploy = false and must still be in ECS_SERVICE_MAP, so a push to its repository is logged as \"auto_deploy is disabled\" rather than as an unmapped repository. Got: ${aws_lambda_function.lambda_deploy.environment[0].variables["ECS_SERVICE_MAP"]}"
  }
}

# An environment with no services at all — the shape of every project before it
# grows a second container, and the one where a for_each over an empty map is
# most likely to be written as something that fails to plan rather than as
# something that produces nothing.
run "no_services_plans_and_notifies_nothing" {
  command = plan

  variables {
    services = []
  }

  assert {
    condition     = length(aws_lambda_invocation.services_revision) == 0
    error_message = "With no services there must be no services_revision invocations. Got ${length(aws_lambda_invocation.services_revision)}."
  }

  assert {
    condition     = length(aws_lambda_invocation.backend_revision) == 1
    error_message = "The backend is still notified in an environment with no services. Got ${length(aws_lambda_invocation.backend_revision)}."
  }
}

# ---------------------------------------------------------------------------
# The payload and the trigger. One run, and the only one that applies.
#
# `revision` is Computed, so at plan time it is unknown — and an unknown anywhere
# in a condition aborts the RUN with "Condition expression could not be evaluated
# at this time" instead of failing an assertion. The payload and the trigger
# therefore cannot be read at plan at all.
#
# Terraform >= 1.11 offers `override_during = plan` for exactly this. It is not
# used: CI pins 1.9.8 (.github/workflows/ci.yml), which rejects the argument
# outright — "An argument named override_during is not expected here" — so the
# whole file would fail to parse in the one place it has to run. Applying against
# mocks needs nothing newer than the 1.7 that introduced them.
#
# No AWS is reached. Every provider is mocked, so this writes to Terraform's
# in-memory test state and the teardown destroy is mocked too.
# ---------------------------------------------------------------------------
run "the_payload_is_the_one_the_lambda_decodes" {
  command = apply

  variables {
    backend_auto_deploy = true
  }

  # The revisions, pinned. Not merely plumbing to make the values known: this is
  # what gives the trigger and reason assertions their teeth. 42 and 7 only reach
  # `triggers.revision` and the `reason` string if those expressions really read
  # aws_ecs_task_definition.<x>.revision — point either at .arn, .id or a literal
  # and a different value arrives.
  override_resource {
    target = aws_ecs_task_definition.backend
    values = {
      revision = 42
    }
  }

  override_resource {
    target = aws_ecs_task_definition.services["orders"]
    values = {
      revision = 7
    }
  }

  # An override, not a mock, and the difference is the point: an overridden
  # resource is never created, so its PROVISIONER never runs.
  # null_resource.build_ci_lambda shells out to `go build` (lambda.tf). A plan
  # would never reach it — provisioners do not run at plan — but an apply would,
  # putting a Go toolchain and a cross-compile on the critical path of a test
  # that is about a jsonencode.
  override_resource {
    target = null_resource.build_ci_lambda
  }

  # ---- the envelope ------------------------------------------------------

  # handler.Handle routes a deploy on the DETAIL-TYPE, not the source, because
  # the set of sources the workflow generators emit has changed over time.
  assert {
    condition     = jsondecode(aws_lambda_invocation.backend_revision[0].input)["detail-type"] == "SERVICE_DEPLOY"
    error_message = "The invocation must send detail-type SERVICE_DEPLOY; handler.Handle routes on it and answers anything else with \"ignored\" and a nil error, which is a GREEN apply that deployed nothing. Got: ${aws_lambda_invocation.backend_revision[0].input}"
  }

  # The source is not routing — it is a DISCRIMINATOR, asserted here on the
  # RENDERED value because that is the string handler/manual.go compares.
  #
  # "terraform.{env}" is the only source promoted to deploy.SourceTerraform, and
  # deploy.SourceTerraform is the only source allowed to poll through an IAM
  # permission-propagation race rather than reporting AccessDenied as permanent.
  # That race is real and measured: on a first apply, the policy attachment
  # completed at 06:01:17.0126 and these invocations ran at 06:01:23.38 — 6.4s
  # later, before IAM had caught up — and all three answered
  # "not authorized to perform: ecs:UpdateService" against a correct policy,
  # which the old classification reported as a GREEN apply that deployed nothing.
  # Rewriting this to "action.${var.env}" to match the other emitters breaks
  # nothing visible and silently restores that.
  assert {
    condition     = jsondecode(aws_lambda_invocation.backend_revision[0].input).source == "terraform.${var.env}"
    error_message = "The invocation must send source \"terraform.${var.env}\". handler/manual.go compares it against handler.TerraformInvocationSource(env) to promote the request to deploy.SourceTerraform — the only source permitted to wait out an IAM propagation race, and the only one whose failure fails the apply. Got: ${jsondecode(aws_lambda_invocation.backend_revision[0].input).source}"
  }

  assert {
    condition     = jsondecode(aws_lambda_invocation.services_revision["orders"].input).source == "terraform.${var.env}"
    error_message = "Every services_revision invocation must send source \"terraform.${var.env}\" too; a service that loses it races IAM on the first apply exactly as the backend did. Got: ${jsondecode(aws_lambda_invocation.services_revision["orders"].input).source}"
  }

  # ---- the detail: exactly the keys manualDetail decodes -------------------

  # The exact key set, not a subset. handler.manualDetail decodes service,
  # project, env, task_definition, image_uri and reason; encoding/json drops any
  # other key without a word, so a renamed or camelCased one disables the
  # notification while every text-level check still passes. An EXTRA key is as
  # much of a failure as a missing one — if a key is added here it has to be
  # added to manualDetail's json tags in the same change, which the AST-reading
  # Go guard (boundary/lambdatf_guard_test.go) then keeps honest.
  assert {
    condition     = sort(keys(jsondecode(aws_lambda_invocation.backend_revision[0].input).detail)) == tolist(["env", "project", "reason", "service"])
    error_message = "The detail object must carry exactly service, project, env and reason. Anything else is dropped by encoding/json in handler.manualDetail and the deploy silently does not happen. Got: ${jsonencode(sort(keys(jsondecode(aws_lambda_invocation.backend_revision[0].input).detail)))}"
  }

  assert {
    condition     = jsondecode(aws_lambda_invocation.backend_revision[0].input).detail.service == jsondecode(file("${path.module}/ci_lambda/contract/contract.json")).backend_id
    error_message = "detail.service must be the identifier ECS_SERVICE_MAP is keyed with, which is ci_lambda/contract/contract.json's backend_id. Anything else and the Lambda answers \"unknown target\" and ignores the event. Got: ${jsondecode(aws_lambda_invocation.backend_revision[0].input).detail.service}"
  }

  # manual.go ignores an event whose project or env names a different project.
  # This is what stops a second meroku project in the same AWS account being
  # deployed by this one's invocation, so both fields have to be the module's
  # own — not a neighbour's, and not empty.
  assert {
    condition     = jsondecode(aws_lambda_invocation.backend_revision[0].input).detail.project == var.project && jsondecode(aws_lambda_invocation.backend_revision[0].input).detail.env == var.env
    error_message = "detail.project and detail.env must match this module's project and env; manual.go compares them against the Lambda's config and returns \"ignored\" on a mismatch. Got: ${aws_lambda_invocation.backend_revision[0].input}"
  }

  # ---- the trigger is the revision, not the ARN ---------------------------

  # The revision is the number that changes exactly when the rendered content
  # did, so an apply that re-renders identical content invokes nothing. The ARN
  # would work too — and would also re-fire on every ARN-level change — but the
  # cost of getting this wrong is asymmetric: a trigger that stops changing stops
  # deploying configuration changes silently, which is the original defect.
  assert {
    condition     = aws_lambda_invocation.backend_revision[0].triggers["revision"] == "42"
    error_message = "triggers.revision must be aws_ecs_task_definition.backend.revision. The override above sets that attribute to 42, so any other expression here yields a different value. Got: ${jsonencode(aws_lambda_invocation.backend_revision[0].triggers)}"
  }

  # The reason is what a human reads in the Lambda's log line when they ask why a
  # deploy happened, so it has to name the revision that caused it.
  assert {
    condition     = strcontains(jsondecode(aws_lambda_invocation.backend_revision[0].input).detail.reason, "revision 42")
    error_message = "detail.reason must name the revision that triggered the deploy; it is the only explanation in the Lambda's log. Got: ${jsondecode(aws_lambda_invocation.backend_revision[0].input).detail.reason}"
  }

  # ---- and all of it again for a service ----------------------------------

  # Proved on a service too, not on the backend alone: these are two separate
  # resources in two separate files that happen to agree today. services.tf
  # builds detail.service from module.ci_identifiers.service_ids[each.key] rather
  # than from each.key, because it has to be the key ECS_SERVICE_MAP was built
  # with; the two are the same string today and nothing should depend on that
  # staying true.
  assert {
    condition     = jsondecode(aws_lambda_invocation.services_revision["orders"].input).detail.service == "orders"
    error_message = "detail.service for a service must be its identifier in ECS_SERVICE_MAP. Got: ${jsondecode(aws_lambda_invocation.services_revision["orders"].input).detail.service}"
  }

  assert {
    condition     = sort(keys(jsondecode(aws_lambda_invocation.services_revision["orders"].input).detail)) == tolist(["env", "project", "reason", "service"])
    error_message = "The service detail object must carry exactly service, project, env and reason — see the backend assertion above for what an extra key costs. Got: ${jsonencode(sort(keys(jsondecode(aws_lambda_invocation.services_revision["orders"].input).detail)))}"
  }

  assert {
    condition     = aws_lambda_invocation.services_revision["orders"].triggers["revision"] == "7"
    error_message = "triggers.revision must be aws_ecs_task_definition.services[each.key].revision. Got: ${jsonencode(aws_lambda_invocation.services_revision["orders"].triggers)}"
  }

  assert {
    condition     = strcontains(jsondecode(aws_lambda_invocation.services_revision["orders"].input).detail.reason, "revision 7")
    error_message = "detail.reason must name the revision that triggered the deploy. Got: ${jsondecode(aws_lambda_invocation.services_revision["orders"].input).detail.reason}"
  }
}
