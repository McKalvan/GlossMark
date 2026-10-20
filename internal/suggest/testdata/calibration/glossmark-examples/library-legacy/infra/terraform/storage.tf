# Printed overdue notices are archived for seven years (records schedule LIB-12).
# @gloss provisions .NoticeArchive conf=0.64
resource "aws_s3_bucket" "notices" {
  bucket = "riverbend-circ-notices"
}

resource "aws_s3_bucket_lifecycle_configuration" "notices" {
  bucket = aws_s3_bucket.notices.id
  rule {
    id     = "records-schedule-lib-12"
    status = "Enabled"
    filter {}
    expiration {
      days = 2557
    }
  }
}
