variable "region" {
  type    = string
  default = "us-east-2"
}

variable "environment" {
  type    = string
  default = "prod"
}

variable "web_image" {
  type        = string
  description = "circ-web image tag pushed by the Jenkins build"
}

variable "kiosk_image" {
  type        = string
  description = "kiosk-gateway image tag"
}

variable "db_instance_class" {
  type    = string
  default = "db.t4g.medium"
}
