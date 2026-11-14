# PutEvents exists only for declared producers and matches their contract source.
mock_provider "aws" {}
override_data {
  target = data.aws_region.current
  values = { region = "us-east-1", name = "us-east-1" }
}
override_data {
  target = data.aws_caller_identity.current
  values = { account_id = "000000000000" }
}
override_resource {
  target = aws_cloudwatch_event_bus.project[0]
  values = { arn = "arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev" }
}

variables {
  project     = "myapp"
  env         = "dev"
  bus_enabled = true
  workloads = {
    orders = { task_role_name = "orders-task", event_source = "myapp.orders" }
    worker = { task_role_name = "worker-task", event_source = "myapp.worker" }
  }
}

# Exact own-source matching may use StringEquals, or the all-values form with
# a Null=false guard so an absent source cannot satisfy it.
run "typed_and_any_producers_are_bus_and_source_scoped" {
  command = apply

  variables {
    producers = [{
      name   = "orders"
      source = "myapp.orders"
      entries = [
        { bus = "project", detail_types = ["order.created", "order.cancelled"] },
        { bus = "default", any = true }
      ]
    }]
  }

  assert {
    condition = (
      toset(keys(output.policies)) == toset(["orders"]) &&
      aws_iam_role_policy.workload["orders"].role == "orders-task" &&
      length(jsondecode(output.policies["orders"]).Statement) == 2 &&
      alltrue([
        for st in jsondecode(output.policies["orders"]).Statement :
        st.Effect == "Allow" &&
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["events:PutEvents"]) &&
        (
          try(toset(try(tolist(st.Condition.StringEquals["events:source"]), [tostring(st.Condition.StringEquals["events:source"])])) == toset(["myapp.orders"]), false) ||
          (
            try(toset(try(tolist(st.Condition["ForAllValues:StringEquals"]["events:source"]), [tostring(st.Condition["ForAllValues:StringEquals"]["events:source"])])) == toset(["myapp.orders"]), false) &&
            try(tostring(st.Condition.Null["events:source"]), "") == "false"
          )
        )
      ])
    )
    error_message = "Only orders may publish, on its task role, with no infrastructure actions and own-source restrictions on both entries."
  }

  assert {
    condition = (
      toset(try(
        tolist(one([for st in jsondecode(output.policies["orders"]).Statement : st if contains(try(tolist(st.Resource), [tostring(st.Resource)]), "arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev")]).Condition["ForAllValues:StringEquals"]["events:detail-type"]),
        []
      )) == toset(["order.created", "order.cancelled"]) &&
      try(tostring(one([
        for st in jsondecode(output.policies["orders"]).Statement : st
        if contains(try(tolist(st.Resource), [tostring(st.Resource)]), "arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev")
      ]).Condition.Null["events:detail-type"]), "") == "false"
    )
    error_message = "Typed publishing needs the complete allowed detail-type set and Null=false so a missing detail-type cannot bypass the batch restriction."
  }

  assert {
    condition = (
      toset(flatten([for st in jsondecode(output.policies["orders"]).Statement : try(tolist(st.Resource), [tostring(st.Resource)])])) ==
      toset([
        "arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev",
        "arn:aws:events:us-east-1:000000000000:event-bus/default"
      ]) &&
      alltrue([
        for st in jsondecode(output.policies["orders"]).Statement :
        alltrue([for test, vars in st.Condition : !contains(keys(vars), "events:detail-type")])
        if contains(try(tolist(st.Resource), [tostring(st.Resource)]), "arn:aws:events:us-east-1:000000000000:event-bus/default")
      ])
    )
    error_message = "any on the explicit default bus must omit every detail-type condition, without broadening the bus resources."
  }
}

run "any_with_foreign_sources_has_no_conditions" {
  command = apply

  variables {
    producers = [{
      name    = "orders"
      source  = "myapp.orders"
      entries = [{ bus = "project", any = true, own_source_only = false }]
    }]
  }

  assert {
    condition = (
      toset(keys(output.policies)) == toset(["orders"]) &&
      length(jsondecode(output.policies["orders"]).Statement) == 1 &&
      alltrue([
        for st in jsondecode(output.policies["orders"]).Statement :
        st.Effect == "Allow" &&
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["events:PutEvents"]) &&
        toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset(["arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev"]) &&
        !contains(keys(st), "Condition")
      ]) &&
      length(aws_sns_topic.alerts) == 0
    )
    error_message = "any with own_source_only=false must have no detail-type or source condition, must stay on its selected bus, and must not grant an unlisted workload or create consumer alerts."
  }
}

run "producer_sources_match_the_shared_vectors" {
  command = apply

  variables {
    workloads = {
      for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors :
      v.workload => { task_role_name = "myapp_${v.workload}_task_dev", event_source = v.want }
    }
    producers = [
      for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : {
        name    = v.workload
        source  = v.want
        entries = [{ bus = "project", detail_types = ["synthetic.completed"] }]
      }
    ]
  }

  assert {
    condition = alltrue([
      for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors :
      length(jsondecode(output.policies[v.workload]).Statement) == 1 &&
      alltrue([
        for st in jsondecode(output.policies[v.workload]).Statement :
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["events:PutEvents"]) &&
        (
          try(toset(try(tolist(st.Condition.StringEquals["events:source"]), [tostring(st.Condition.StringEquals["events:source"])])) == toset([v.want]), false) ||
          (
            try(toset(try(tolist(st.Condition["ForAllValues:StringEquals"]["events:source"]), [tostring(st.Condition["ForAllValues:StringEquals"]["events:source"])])) == toset([v.want]), false) &&
            try(tostring(st.Condition.Null["events:source"]), "") == "false"
          )
        )
      ])
    ])
    error_message = "Every backend, service, scheduled-task and event-task vector must use its exact EVENT_SOURCE in the producer source condition."
  }
}
