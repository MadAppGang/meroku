# Upgrading Old Deployments

Steps for an environment deployed by an old meroku version that the current
modules cannot upgrade on their own. Each section names the error the apply
stops with, why, and what to run.

The YAML migrations (`ai_docs/MIGRATIONS.md`) change config files. These steps
change live infrastructure or Terraform state, so meroku does not run them
itself.

## Cloud Map: `ServiceAlreadyExists` on `aws_service_discovery_service.backend[0]`

```
creating Service Discovery Service (<project>_service_<env>): ... ServiceAlreadyExists
```

**Affects:** environments whose backend was deployed before `9830d0b` (October
2025).

**Why:** the backend ECS service used to run with ECS Service Connect, and
Service Connect created a Cloud Map service named `<project>_service_<env>` in
the `<project>_<env>.private` namespace. That service was never in Terraform
state. The module now creates a Cloud Map service with the same name itself
(`modules/workloads/backend.tf`) and registers the ECS service with it through
`service_registries`, so the create collides with the one Service Connect made.

The failed apply leaves the environment working: the ECS service still has
Service Connect, and the API Gateway integration still points at the old Cloud
Map service.

**Fix:** turn Service Connect off on the ECS service. When that deployment
completes, ECS deletes the Cloud Map service it created. Then apply again.

```bash
export AWS_PROFILE=<profile> AWS_REGION=<region>

# 1. Confirm the collision is the Service Connect one: no DnsConfig, and the
#    ECS service has Service Connect enabled.
aws servicediscovery list-services \
  --query "Services[?Name=='<project>_service_<env>'].[Id,Name]" --output text
aws ecs describe-services --cluster <project>_cluster_<env> \
  --services <project>_service_<env> \
  --query 'services[0].deployments[0].serviceConnectConfiguration.enabled'

# 2. Turn Service Connect off.
aws ecs update-service --cluster <project>_cluster_<env> \
  --service <project>_service_<env> \
  --service-connect-configuration enabled=false

# 3. Wait for the deployment to complete and the Cloud Map service to disappear.
aws ecs wait services-stable --cluster <project>_cluster_<env> \
  --services <project>_service_<env>
aws servicediscovery list-services \
  --query "Services[?Name=='<project>_service_<env>']" --output text   # empty

# 4. Apply. The plan creates aws_service_discovery_service.backend[0], adds
#    service_registries to the ECS service and repoints the API Gateway
#    integration.
```

**Downtime:** from the moment ECS deletes the old Cloud Map service until the
apply repoints API Gateway, the API returns errors. On passflow dev the
deployment took about five minutes, then the apply.

If the service still exists after step 3 (ECS did not delete it), check it has
no instances (`aws servicediscovery list-instances --service-id <id>`) and
delete it with `aws servicediscovery delete-service --id <id>`.

## EventBridge: `aws_cloudwatch_event_rule.ecr_event` destruction times out

```
module.workloads.aws_cloudwatch_event_rule.ecr_event: Destruction errored after 5m0s
```

**Affects:** environments created by a version whose rule was named
`ecr_events_cicd` with no project or environment in it, where a second
environment was deployed into the same AWS account.

**Why:** both environments' states manage the one rule, and each attached its
own deploy Lambda as a target. The current module replaces the rule with the
`ci_*` rules and destroys it. AWS refuses to delete a rule that still has
targets, and the other environment's target is still on it, so the provider
retries until it times out.

**Fix:** check the remaining targets belong to another environment, then remove
the rule from this environment's state only. Nothing in AWS is deleted.

```bash
aws events list-targets-by-rule --rule ecr_events_cicd
terraform -chdir=env/<env> state rm module.workloads.aws_cloudwatch_event_rule.ecr_event
```

The rule stays in AWS with the other environment's target, and that
environment's upgrade or cleanup removes it.

## GitHub Actions role renamed from `GithubActionsRole`

**Affects:** environments deployed before December 2025, when the role was
named `GithubActionsRole` in every project.

The apply replaces it with `<project>-<env>-github-actions-role`. Since v4.8.3
the subject overlap check recognises the old role as this environment's own,
using the Terraform state. After the apply, update every workflow that assumes
`role/GithubActionsRole`:

```yaml
role-to-assume: arn:aws:iam::<account>:role/<project>-<env>-github-actions-role
```

Until that change is merged, deploys from GitHub Actions fail at the credentials
step.
