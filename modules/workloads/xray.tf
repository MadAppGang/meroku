
locals {
  xray_container = var.xray_enabled ? [{
    name  = "xray-daemon"
    image = "amazon/aws-xray-daemon"
    portMappings = [{
      containerPort = 2000
      hostPort      = 2000
      protocol      = "udp"
    }]
    cpu               = 32
    memoryReservation = 256
    essential         = false
  }] : []

  app_container_environment = var.xray_enabled ? [
    {
      name  = "AWS_XRAY_DAEMON_ADDRESS"
      value = "xray-daemon:2000"
    }
  ] : []
}
