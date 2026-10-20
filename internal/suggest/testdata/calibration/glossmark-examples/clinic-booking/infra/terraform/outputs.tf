output "database_endpoint" {
  value = aws_db_instance.booking.address
}

output "reminder_queue_url" {
  value = aws_sqs_queue.reminder_events.url
}

output "cluster_name" {
  value = module.eks.cluster_name
}
