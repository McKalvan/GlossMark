output "db_endpoint" {
  value = aws_db_instance.circ.address
}

output "cluster_arn" {
  value = aws_ecs_cluster.circ.arn
}
