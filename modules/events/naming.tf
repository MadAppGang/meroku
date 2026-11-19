# Every capped AWS name this module builds, through the shared cascade. See
# modules/naming. All of them are new resources, so there is no legacy form.
#
# Queues cap at 80 and rules at 64. A rule carries two user-supplied fields, the
# consumer and the subscription key, so it is the one most likely to reach form 3.
# app/events.go computes the queue names with the same cascade (awsName) to
# refuse an extensions.sqs_queues entry of the same name.
#
# One module call per AWS namespace, because collisions are checked within one
# call and names in different namespaces never clash: an SQS queue and an
# EventBridge subscriber may share a text. module.naming holds the SQS queues
# and the rules, module.naming_subscribers the enhanced-bus subscribers and
# module.naming_roles the delivery roles. A request exists only for a resource
# that exists, so a classic environment asks for exactly the names it always
# did. app/events_enhanced.go mirrors the subscriber and role names.

module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = merge(
    { for c in keys(local.consumers) : "queue/${c}" => {
      parts = ["events", c]
      limit = 80
    } },
    { for c in keys(local.consumers) : "dlq/${c}" => {
      parts = ["events", c, "dlq"]
      limit = 80
    } },
    { for c in keys(local.consumers) : "delivery_dlq/${c}" => {
      parts = ["events", c, "delivery-dlq"]
      limit = 80
    } },
    { for k, s in local.rule_subscriptions : "rule/${k}" => {
      parts = ["events", s.consumer, s.key]
      limit = 64
    } },
    { for c in local.fifo_consumers : "fifo_queue/${c}" => {
      parts  = ["events", c]
      limit  = 80
      suffix = ".fifo"
    } },
    { for c in local.fifo_consumers : "fifo_dlq/${c}" => {
      parts  = ["events", c, "dlq"]
      limit  = 80
      suffix = ".fifo"
    } },
    { for c in local.fifo_consumers : "fifo_delivery_dlq/${c}" => {
      parts  = ["events", c, "delivery-dlq"]
      limit  = 80
      suffix = ".fifo"
    } },
  )
}

# A standard subscriber's name joins consumer and key with a ".", which neither
# can contain, so the name splits back into one subscription. A FIFO
# subscriber's name has no "." and ends in "-fifo", so it never equals a
# standard one.
module "naming_subscribers" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = merge(
    { for k, s in local.subscriber_subscriptions : "subscriber/${k}" => {
      parts = ["events", "${s.consumer}.${s.key}"]
      limit = 64
    } },
    { for c in local.ordered_consumers : "fifo_subscriber/${c}" => {
      parts = ["events", c, "fifo"]
      limit = 64
    } },
  )
}

module "naming_roles" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = { for c in local.delivery_consumers : "delivery_role/${c}" => {
    parts     = ["events", c, "delivery"]
    limit     = 64
    separator = "_"
  } }
}
