# @gloss provisions .CircCluster conf=0.9
resource "aws_ecs_cluster" "circ" {
  name = "circ-${var.environment}"
}

resource "aws_ecs_task_definition" "web" {
  family                   = "circ-web"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 1024
  memory                   = 2048
  container_definitions = jsonencode([{
    name         = "circ-web"
    image        = var.web_image
    portMappings = [{ containerPort = 8080 }]
    environment  = [{ name = "CIRC_CONFIG_DIR", value = "/etc/circ" }]
  }])
}

# @gloss provisions .CircWebApp conf=0.8
resource "aws_ecs_service" "web" {
  name            = "circ-web"
  cluster         = aws_ecs_cluster.circ.id
  task_definition = aws_ecs_task_definition.web.arn
  desired_count   = 2
  launch_type     = "FARGATE"
  network_configuration {
    subnets = data.aws_subnets.private.ids
  }
}

resource "aws_ecs_task_definition" "kiosk" {
  family                   = "kiosk-gateway"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  container_definitions = jsonencode([{
    name         = "kiosk-gateway"
    image        = var.kiosk_image
    portMappings = [{ containerPort = 9443 }]
  }])
}

# @gloss provisions .KioskGateway conf=0.8
resource "aws_ecs_service" "kiosk" {
  name            = "kiosk-gateway"
  cluster         = aws_ecs_cluster.circ.id
  task_definition = aws_ecs_task_definition.kiosk.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  network_configuration {
    subnets = data.aws_subnets.private.ids
  }
}

# @gloss provisions .NightlyBatch conf=0.8
resource "aws_cloudwatch_event_rule" "nightly" {
  name                = "circ-nightly"
  schedule_expression = "cron(30 6 * * ? *)"
}

resource "aws_cloudwatch_event_target" "nightly" {
  rule     = aws_cloudwatch_event_rule.nightly.name
  arn      = aws_ecs_cluster.circ.arn
  role_arn = aws_iam_role.events.arn
  ecs_target {
    task_definition_arn = aws_ecs_task_definition.web.arn
    launch_type         = "FARGATE"
    network_configuration {
      subnets = data.aws_subnets.private.ids
    }
  }
  input = jsonencode({
    containerOverrides = [{ name = "circ-web", command = ["/opt/circ/batch/bin/run_nightly.sh"] }]
  })
}

resource "aws_iam_role" "events" {
  name = "circ-nightly-events"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "events.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}
