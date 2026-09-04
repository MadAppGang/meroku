# EC2 compute pools — ASG-backed ECS capacity providers.
#
# THE RULE FOR THIS FILE: every resource carries a guard, and a resource without
# one does not belong here. The `for_each`-driven resources guard themselves
# because `local.pools` is empty when there are no pools; the shared ones beside
# them do not, and a resource with neither `count` nor `for_each` is created
# unconditionally. That is how a file whose `for_each` resources all behave
# correctly still plans "5 to add" in an environment that will never run an EC2
# task. Traced against an environment with no `compute:` block, every resource
# below contributes nothing: `Plan: 0 to add, 0 to change, 0 to destroy`.
#
# Network mode is `bridge` by default (D-6): the task shares the instance's
# primary ENI, which carries a public IP because `map_public_ip_on_launch` is
# true on this VPC's subnets, so egress works with no NAT gateway and no VPC
# endpoint. `awsvpc` stays available as a per-pool override and is refused at
# generate time unless the pool asserts an egress path (`assume_egress`).

locals {
  # var.compute_pools appears EXACTLY ONCE in this module — on the next line.
  # Every other expression keys off local.pools.
  # `grep -c 'var\.compute_pools' modules/workloads/*.tf` must return 1; anything
  # reading the unfiltered variable reintroduces the all-disabled-pools diff.
  #
  # p.enabled is read directly and that is safe: variables.tf declares it
  # optional(bool, true), so Terraform substitutes true during type conversion
  # for every pool that omits the key, and the attribute is never null.
  pools = { for p in var.compute_pools : p.name => p if p.enabled }

  # The one switch behind every shared resource below. Derived from the
  # enabled-filtered map, never from the variable — `length(var.compute_pools)`
  # would create a cluster capacity-provider association for a pool set that is
  # entirely disabled.
  pool_count = length(local.pools) > 0 ? 1 : 0

  # D-6. Bridge is the default and the vast majority; awsvpc is a per-pool
  # override. Only bridge pools need the dynamic-host-port ingress on the
  # instance security group, because only they put tasks on the host's ports.
  bridge_pools      = { for k, p in local.pools : k => p if p.network_mode == "bridge" }
  bridge_pool_count = length(local.bridge_pools) > 0 ? 1 : 0

  # Per-pool inline IAM statements collapse onto the one shared instance role.
  instance_policy_statements = flatten([
    for k, p in local.pools : [
      for s in p.instance_policies : { actions = s.actions, resources = s.resources }
    ]
  ])
  instance_policy_count = length(local.instance_policy_statements) > 0 ? 1 : 0

  pool_ami_ssm = {
    al2023       = "/aws/service/ecs/optimized-ami/amazon-linux-2023/recommended/image_id"
    al2023_arm64 = "/aws/service/ecs/optimized-ami/amazon-linux-2023/arm64/recommended/image_id"
    al2023_gpu   = "/aws/service/ecs/optimized-ami/amazon-linux-2023/gpu/recommended/image_id"
  }

  # instances_distribution, derived from capacity_type so the invalid
  # combinations are unrepresentable:
  #   on_demand       -> (0, 100)   all on-demand
  #   spot            -> (0, 0)     all spot
  #   spot_with_base  -> (base, 0)  `base` on-demand INSTANCES, spot above it
  # on_demand_base_capacity counts INSTANCES, not tasks. The recommender's
  # effectiveHourly blend depends on that unit.
  pool_on_demand_base = { for k, p in local.pools :
    k => p.capacity_type == "spot_with_base" ? p.on_demand_base : 0
  }
  pool_on_demand_percentage = { for k, p in local.pools :
    k => p.capacity_type == "on_demand" ? 100 : 0
  }

  # Which pool, if any, each workload runs on. Consumed by backend.tf and
  # services.tf. Either may only ever name a key that exists in local.pools:
  # generate-time validation refuses anything else, and the preconditions on the
  # ECS services turn a hand-edited main.tf into a sentence rather than
  # `Error: Invalid index`.
  backend_pool = var.backend_runtime == "ec2" ? var.backend_compute_pool : null
  service_pools = { for s in var.services :
    s.name => try(s.runtime, "fargate") == "ec2" ? s.compute_pool : null
  }

  # Whether either of these names a usable pool, and what to say when it does
  # not, is decided in pool_check.tf. That work sits in a module with no
  # provider so it can be tested; see the comment there.

  # THE NETWORK MODE OF EACH WORKLOAD. Fargate is awsvpc and always will be — it
  # is the only mode Fargate supports. An EC2 workload takes its pool's.
  #
  # These reads are TOTAL — lookup()/try() with a null-safe default, never a
  # direct index into local.pools. A local is evaluated where it is first
  # referenced and Terraform gives no ordering guarantee between resource nodes,
  # so a direct `local.pools[local.backend_pool]` here can raise
  # `Error: Invalid index` from the task definition, from a target group's
  # count/for_each, from the Cloud Map `dynamic "dns_records"` or from the port
  # mappings — none of which sits behind the precondition, which lives only in
  # the ECS service's lifecycle block. Made total, every one of those sites
  # renders the Fargate value for a pool name that is not in the map, and the
  # precondition is the only thing that can fail. That is the whole point of
  # having a precondition.
  backend_pool_obj     = local.backend_pool == null ? null : lookup(local.pools, local.backend_pool, null)
  backend_network_mode = local.backend_pool_obj == null ? "awsvpc" : local.backend_pool_obj.network_mode
  service_network_modes = { for k, v in local.service_pools :
    k => v == null ? "awsvpc" : try(local.pools[v].network_mode, "awsvpc")
  }

  # Same treatment, same reason — the arm64 task-definition branch is the other
  # place a workload reads its pool's attributes.
  backend_arm64 = try(local.backend_pool_obj.ami_family, "") == "al2023_arm64"
  service_arm64 = { for k, v in local.service_pools :
    k => v == null ? false : try(local.pools[v].ami_family, "") == "al2023_arm64"
  }

  # Convenience booleans, so backend.tf/services.tf read as English rather than
  # as three nested conditionals per line.
  backend_bridge = local.backend_network_mode == "bridge"
  service_bridge = { for k, m in local.service_network_modes : k => m == "bridge" }

  # hostPort = 0 asks Docker for an ephemeral port. Two ranges are in play: the
  # Docker default for published ports is 49153-65535, and the Linux ephemeral
  # range on AL2023 is 32768-60999. The security group opens the union, which is
  # also the range AWS's own dynamic-port-mapping guidance names. Narrowing it to
  # one of the two would work until an AMI or agent change moved the allocation,
  # and the failure — a target that never becomes healthy — does not look like a
  # port-range problem.
  ecs_dynamic_port_from = 32768
  ecs_dynamic_port_to   = 65535
}

# 1. AMI. A public SSM parameter, so `terraform validate -backend=false` never
#    needs to resolve it. Skipped entirely for a pool that pins an explicit
#    ami_id.
data "aws_ssm_parameter" "ecs_ami" {
  for_each = { for k, p in local.pools : k => p if p.ami_id == "" }

  name = local.pool_ami_ssm[each.value.ami_family]
}

# 2. Launch template. No `instance_type` here — the ASG's mixed_instances_policy
#    overrides supply them.
resource "aws_launch_template" "pool" {
  for_each = local.pools

  name        = "${var.project}_${each.key}_${var.env}"
  description = "ECS container instances for compute pool ${each.key} (${var.project}/${var.env})"
  image_id    = each.value.ami_id != "" ? each.value.ami_id : try(data.aws_ssm_parameter.ecs_ami[each.key].value, null)

  update_default_version = true

  # The [0] is not decoration: these resources are count-gated, so an un-indexed
  # reference is a type error at plan time.
  iam_instance_profile {
    arn = aws_iam_instance_profile.ecs_instance[0].arn
  }

  vpc_security_group_ids = [aws_security_group.ecs_instance[0].id]

  # 3. User data: join the cluster, then whatever the pool appended, and nothing
  #    else.
  #
  #    ECS_ENABLE_HIGH_DENSITY_ENI is deliberately NOT emitted. Trunking is
  #    enabled solely by the awsvpcTrunking account setting; the agent ignores
  #    unknown keys, so emitting it would fail silently and leave a config file
  #    that reads as though trunking were configured.
  #
  #    ECS_ENABLE_SPOT_INSTANCE_DRAINING is also omitted: AWS documents it as
  #    redundant when managed draining is on, and it is (capacity provider,
  #    below).
  user_data = base64encode(join("\n", compact([
    "#!/bin/bash",
    "echo \"ECS_CLUSTER=${aws_ecs_cluster.main.name}\" >> /etc/ecs/ecs.config",
    each.value.user_data_extra,
  ])))

  # IMDSv2 required. Hop limit 2 so the ECS agent and host-networked helpers,
  # which sit one hop further out, still reach IMDS.
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size           = each.value.root_volume_gb
      volume_type           = "gp3"
      encrypted             = "true"
      delete_on_termination = "true"
    }
  }

  dynamic "block_device_mappings" {
    for_each = each.value.extra_volumes

    content {
      device_name = block_device_mappings.value.device_name

      ebs {
        volume_size           = block_device_mappings.value.size_gb
        volume_type           = block_device_mappings.value.type
        encrypted             = "true"
        delete_on_termination = "true"
      }
    }
  }

  # An awsvpcTrunking prerequisite, harmless without it.
  private_dns_name_options {
    enable_resource_name_dns_a_record = false
  }

  tag_specifications {
    resource_type = "instance"

    tags = {
      Name        = "${var.project}_${each.key}_${var.env}"
      Environment = var.env
      Project     = var.project
      ManagedBy   = "meroku"
      terraform   = "true"
      Application = "${var.project}-${var.env}"
      ComputePool = each.key
    }
  }

  tag_specifications {
    resource_type = "volume"

    tags = {
      Name        = "${var.project}_${each.key}_${var.env}"
      Environment = var.env
      Project     = var.project
      ManagedBy   = "meroku"
      terraform   = "true"
      Application = "${var.project}-${var.env}"
      ComputePool = each.key
    }
  }

  tags = {
    Name        = "${var.project}_${each.key}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }

  # An instance profile is usable before its role's policy attachments have
  # propagated, and the first instance then comes up without
  # ecs:RegisterContainerInstance — a failure that looks like a broken AMI.
  depends_on = [
    aws_iam_role_policy_attachment.ecs_instance_ecs,
    aws_iam_role_policy_attachment.ecs_instance_ssm,
  ]
}

# 4. The ASG behind the capacity provider.
#
#    No create_before_destroy and no name_prefix: the capacity provider depends
#    on this ASG's ARN and is itself not CBD (its `name` is Required + ForceNew
#    with no name_prefix, so CBD would collide on the name). A CBD resource under
#    a non-CBD dependent is exactly where `Error: Cycle` comes from.
resource "aws_autoscaling_group" "pool" {
  for_each = local.pools

  name                = "${var.project}_${each.key}_${var.env}"
  vpc_zone_identifier = var.subnet_ids
  min_size            = each.value.min_size
  max_size            = each.value.max_size
  health_check_type   = "EC2"

  # MUST match the capacity provider's managed_termination_protection =
  # "ENABLED" below. Enabling one without the other leaves instances stuck in
  # DRAINING forever.
  protect_from_scale_in = true

  # desired_capacity is deliberately unset and ignored — ECS managed scaling
  # owns it, and Terraform reasserting a stale value would fight the capacity
  # provider on every apply.

  mixed_instances_policy {
    instances_distribution {
      on_demand_base_capacity                  = local.pool_on_demand_base[each.key]
      on_demand_percentage_above_base_capacity = local.pool_on_demand_percentage[each.key]
      spot_allocation_strategy                 = "price-capacity-optimized"
    }

    launch_template {
      # NOT "$Latest". A literal never changes, so a launch template edit
      # produces no diff on this resource, and instance_refresh below — which
      # the provider starts only when launch_template/mixed_instances_policy
      # actually change — would never fire. latest_version is computed from the
      # template, so an ami_family/user_data_extra/root_volume_gb/extra_volumes
      # edit moves this attribute and the refresh triggers. New instances still
      # get the newest version either way; only the diff differs.
      launch_template_specification {
        launch_template_id = aws_launch_template.pool[each.key].id
        version            = aws_launch_template.pool[each.key].latest_version
      }

      # No instance weighting: an ASG backing an ECS capacity provider cannot
      # have instance weighting settings.
      dynamic "override" {
        for_each = each.value.instance_types

        content {
          instance_type = override.value
        }
      }
    }
  }

  # Omit this and the second apply strips the tag AWS adds on the first, after
  # which ECS can no longer manage the scaling — a failure that appears one
  # apply after the mistake.
  tag {
    key                 = "AmazonECSManaged"
    value               = "true"
    propagate_at_launch = true
  }

  tag {
    key                 = "Name"
    value               = "${var.project}_${each.key}_${var.env}"
    propagate_at_launch = true
  }

  tag {
    key                 = "Environment"
    value               = var.env
    propagate_at_launch = true
  }

  tag {
    key                 = "Project"
    value               = var.project
    propagate_at_launch = true
  }

  tag {
    key                 = "ManagedBy"
    value               = "meroku"
    propagate_at_launch = true
  }

  tag {
    key                 = "terraform"
    value               = "true"
    propagate_at_launch = true
  }

  tag {
    key                 = "Application"
    value               = "${var.project}-${var.env}"
    propagate_at_launch = true
  }

  # THE ONLY PLACE A SUCCESSFUL APPLY WOULD NOT MEAN THE CHANGE TOOK EFFECT.
  # Editing ami_family, user_data_extra, root_volume_gb or extra_volumes writes
  # a new launch template version, which by itself only affects instances
  # launched AFTER it. Without this block terraform reports success, every
  # running instance keeps the old configuration, and at min_size steady state
  # managed scaling may never cycle them — so the pool runs a configuration that
  # exists nowhere in the repository, indefinitely, with nothing to show for it.
  instance_refresh {
    strategy = "Rolling"

    preferences {
      # 0, not a comfortable 50. min_size is 1 by default, and a one-instance
      # group cannot hold any instance in service while replacing its only one:
      # any value above 0 leaves the refresh unable to start a batch. That stall
      # happens AFTER `terraform apply` has returned — StartInstanceRefresh is
      # asynchronous — so a refresh that cannot proceed is exactly as silent as
      # having no refresh at all, which is the failure this block exists to fix.
      # 0 always makes progress. The cost is a capacity gap on a pool small
      # enough to have one instance; managed_draining on the capacity provider
      # stops that instance's tasks gracefully first and ECS reschedules them on
      # the replacement. A pool large enough to care can raise this — knowing
      # that anything above 0 opts a min_size 1 pool out of refreshing at all.
      min_healthy_percentage = 0

      # MANDATORY under managed_termination_protection, not a preference. That
      # setting protects every instance in this group from scale-in, and the
      # default here is "Ignore" — which skips every protected instance and
      # reports the refresh SUCCESSFUL having replaced nothing. AWS states it
      # directly: "Make sure you select Replace for Scale-in protection when
      # calling StartInstanceRefresh if you're using managed termination
      # protection." "Refresh" is that value's name in this API.
      scale_in_protected_instances = "Refresh"
    }
  }

  lifecycle {
    ignore_changes = [desired_capacity]
  }

  # Same instance-profile propagation window as the launch template.
  depends_on = [aws_iam_instance_profile.ecs_instance]
}

# 5. The capacity provider.
#
#    `name` is stable and deliberately not derived from the ASG: it is what every
#    capacity_provider_strategy in backend.tf and services.tf references.
#
#    No lifecycle { create_before_destroy = true }: `name` is Required + ForceNew
#    and there is no name_prefix, so CBD would create the replacement first under
#    the same name and AWS answers
#    `ClientException: The specified capacity provider already exists`.
resource "aws_ecs_capacity_provider" "pool" {
  for_each = local.pools

  name = "${var.project}_${each.key}_${var.env}"

  auto_scaling_group_provider {
    auto_scaling_group_arn = aws_autoscaling_group.pool[each.key].arn

    # managed_draining needs provider >= 5.34.0 — see versions.tf.
    managed_draining = "ENABLED"

    # Derived from the ASG's protect_from_scale_in, never independently
    # configurable, so the invalid combination is unrepresentable in our YAML.
    managed_termination_protection = "ENABLED"

    managed_scaling {
      status                    = "ENABLED"
      target_capacity           = each.value.target_capacity
      minimum_scaling_step_size = 1
      maximum_scaling_step_size = 10
      instance_warmup_period    = 300
    }
  }

  tags = {
    Name        = "${var.project}_${each.key}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# 7. Cluster association. Absent — not empty — when there are no pools, so an
#    environment without a `compute:` block issues no PutClusterCapacityProviders
#    against a live cluster.
#
#    No default_capacity_provider_strategy: setting one changes placement for
#    services that still use `launch_type` and breaks the zero-diff contract.
resource "aws_ecs_cluster_capacity_providers" "main" {
  count = local.pool_count

  cluster_name = aws_ecs_cluster.main.name

  capacity_providers = concat(
    [for k, _ in local.pools : aws_ecs_capacity_provider.pool[k].name],
    ["FARGATE", "FARGATE_SPOT"],
  )

  # AWS refuses to delete a capacity provider that a cluster's provider list
  # still references, and Terraform's default ordering deletes the provider
  # first, so the apply hangs until the resource timeout. Replacing the
  # association whenever a capacity provider changes is the documented
  # workaround.
  lifecycle {
    replace_triggered_by = [aws_ecs_capacity_provider.pool]
  }
}

# 8. The shared instance role. One role across all pools — but shared is not the
#    same as unconditional, hence the count.
resource "aws_iam_role" "ecs_instance" {
  count = local.pool_count

  name = module.naming.names["ecs_instance_role"]

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = "sts:AssumeRole"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Name        = module.naming.names["ecs_instance_role"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# 9. The managed policy that makes an instance an ECS container instance.
#
#    It carries ecs:StartTelemetrySession. A hand-rolled minimal role without it
#    produces NO CloudWatch metrics at all, indistinguishable from "the service
#    never ran" — and the compute recommender reads exactly those metrics.
resource "aws_iam_role_policy_attachment" "ecs_instance_ecs" {
  count = local.pool_count

  role       = aws_iam_role.ecs_instance[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

# 10. Session Manager onto the HOST — shell access with no bastion and no SSH
#     key, and nothing more. It does NOT enable ECS Exec: that needs ssmmessages
#     on the TASK role, which backend.tf already grants.
resource "aws_iam_role_policy_attachment" "ecs_instance_ssm" {
  count = local.pool_count

  role       = aws_iam_role.ecs_instance[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# 11. Per-pool instance_policies, flattened onto the one shared role. Separately
#     guarded, because a pool set that defines no policies must not produce an
#     empty policy document.
resource "aws_iam_role_policy" "ecs_instance_extra" {
  count = local.instance_policy_count

  name = "${var.project}_ecs_instance_extra_${var.env}"
  role = aws_iam_role.ecs_instance[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      for s in local.instance_policy_statements : {
        Effect   = "Allow"
        Action   = s.actions
        Resource = s.resources
      }
    ]
  })
}

# 12. The instance profile the launch template references as [0].
resource "aws_iam_instance_profile" "ecs_instance" {
  count = local.pool_count

  name = module.naming.names["ecs_instance_role"]
  role = aws_iam_role.ecs_instance[0].name

  tags = {
    Name        = module.naming.names["ecs_instance_role"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# 13. The container-instance security group — where D-6 lands.
#
#     Under bridge there is no per-task ENI and therefore no per-task security
#     group, so THIS is what the ALB and the API Gateway VPC Links actually
#     reach. Under an awsvpc-only pool set the group keeps no ingress at all,
#     exactly as before, because the task ENI carries aws_security_group.backend
#     / .services[*] instead.
#
#     The VPC rule is a CIDR rather than a security-group reference on purpose:
#     naming aws_security_group.services[*].id would be tighter, but the source
#     set changes with every service added or removed, so the rule count churns
#     and each change is an in-place SG update on a live instance group. The CIDR
#     form matches the idiom one file away (backend.tf's own VPC-CIDR ingress for
#     the API Gateway VPC Link).
#
#     This is a real widening over awsvpc and it is stated rather than buried:
#     anything in the VPC can reach any ephemeral port on any pool instance. A
#     pool whose workloads need per-task isolation sets network_mode: awsvpc and
#     asserts an egress path.
resource "aws_security_group" "ecs_instance" {
  count = local.pool_count

  name        = module.naming.names["ecs_instance_role"]
  description = "ECS container instances for ${var.project} ${var.env} compute pools"
  vpc_id      = var.vpc_id

  dynamic "ingress" {
    for_each = local.bridge_pool_count > 0 && var.enable_alb && var.alb_security_group_id != "" ? [1] : []

    content {
      protocol        = "tcp"
      from_port       = local.ecs_dynamic_port_from
      to_port         = local.ecs_dynamic_port_to
      security_groups = [var.alb_security_group_id]
      description     = "Allow the ALB to reach bridge-mode tasks on their dynamic host ports"
    }
  }

  dynamic "ingress" {
    for_each = local.bridge_pool_count > 0 ? [1] : []

    content {
      protocol    = "tcp"
      from_port   = local.ecs_dynamic_port_from
      to_port     = local.ecs_dynamic_port_to
      cidr_blocks = [data.aws_vpc.selected.cidr_block]
      description = "Allow traffic from VPC (API Gateway VPC Link, Cloud Map SRV service-to-service)"
    }
  }

  # The path D-6 buys: the instance's primary ENI has a public IP, so a bridge
  # task egresses through it with no NAT gateway.
  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound"
  }

  tags = {
    Name        = module.naming.names["ecs_instance_role"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}
