# Scheduled off-site backups (plan S4). A CronJob runs `consensusdb backup` inside
# the cluster: it dials a node's admin surface, receives the dump stream, encrypts
# it client-side (argon2id + AES-256-GCM) and uploads it to S3-compatible object
# storage (AWS S3 / MinIO / GCS) with object-lock retention (WORM) — so even a
# cluster admin cannot alter or delete a backup until it expires.
#
# Everything is opt-in via terraform.tfvars: leave backup_schedule empty to skip.

variable "backup_schedule" {
  description = "Cron schedule for backups (e.g. \"0 2 * * *\"); empty disables the CronJob"
  type        = string
  default     = ""
}

variable "backup_dest_prefix" {
  description = "s3://bucket/prefix the dated dump object is written under"
  type        = string
  default     = ""
}

variable "backup_s3_endpoint" {
  description = "S3 endpoint host[:port] (empty = AWS); MinIO/GCS set their own"
  type        = string
  default     = ""
}

variable "backup_s3_region" {
  description = "S3 region"
  type        = string
  default     = ""
}

variable "backup_retain_days" {
  description = "Object-lock (WORM) retention in days for each backup object"
  type        = number
  default     = 30
}

# Backup credentials: the dump password (argon2id) and the S3 keys. Empty
# backup_password writes a plain (unencrypted) dump.
variable "backup_password" {
  description = "Password to encrypt dumps (empty = plain dump)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "backup_s3_access_key" {
  type      = string
  sensitive = true
  default   = ""
}

variable "backup_s3_secret_key" {
  type      = string
  sensitive = true
  default   = ""
}

# IAM credentials the backup job authenticates with when auth is enabled (a
# service account bound to roles/cdb.admin, or with cdb.backups.create).
variable "backup_iam_token" {
  description = "IAM token for the backup service account (empty when auth is disabled)"
  type        = string
  sensitive   = true
  default     = ""
}

resource "kubernetes_secret_v1" "backup" {
  count = var.backup_schedule != "" ? 1 : 0
  metadata {
    name      = "${var.deployment}-backup"
    namespace = var.namespace
  }
  data = {
    "backup-password" = var.backup_password
    "s3-access-key"   = var.backup_s3_access_key
    "s3-secret-key"   = var.backup_s3_secret_key
    "iam-token"       = var.backup_iam_token
  }
}

resource "kubernetes_cron_job_v1" "backup" {
  count = var.backup_schedule != "" ? 1 : 0
  metadata {
    name      = "${var.deployment}-backup"
    namespace = var.namespace
  }
  spec {
    schedule                      = var.backup_schedule
    concurrency_policy            = "Forbid"
    successful_jobs_history_limit = 3
    failed_jobs_history_limit     = 3
    job_template {
      metadata {}
      spec {
        backoff_limit = 2
        template {
          metadata {}
          spec {
            restart_policy = "Never"
            container {
              name  = "backup"
              image = "${var.registry_hostname}/${var.project}/${var.deployment}:${var.image_tag}"

              # Dump object: <prefix>/<node>-<timestamp>.dump
              command = ["/bin/sh", "-c"]
              args = [<<-EOT
                ts="$(date -u +%Y%m%dT%H%M%SZ)"
                exec /app/consensusdb backup "${var.backup_dest_prefix}/${var.deployment}-$ts.dump"
              EOT
              ]

              env {
                name  = "ADMIN_ADDRESS"
                value = "tcp://${var.deployment}.${var.namespace}.svc.cluster.local:${var.vrpc_port}"
              }
              env {
                name  = "BACKUP_S3_ENDPOINT"
                value = var.backup_s3_endpoint
              }
              env {
                name  = "BACKUP_S3_REGION"
                value = var.backup_s3_region
              }
              env {
                name  = "BACKUP_S3_USE_SSL"
                value = "true"
              }
              env {
                name  = "BACKUP_S3_RETAIN_DAYS"
                value = tostring(var.backup_retain_days)
              }
              env {
                name = "BACKUP_PASSWORD"
                value_from {
                  secret_key_ref {
                    name = kubernetes_secret_v1.backup[0].metadata[0].name
                    key  = "backup-password"
                  }
                }
              }
              env {
                name = "BACKUP_S3_ACCESS_KEY"
                value_from {
                  secret_key_ref {
                    name = kubernetes_secret_v1.backup[0].metadata[0].name
                    key  = "s3-access-key"
                  }
                }
              }
              env {
                name = "BACKUP_S3_SECRET_KEY"
                value_from {
                  secret_key_ref {
                    name = kubernetes_secret_v1.backup[0].metadata[0].name
                    key  = "s3-secret-key"
                  }
                }
              }
              env {
                name = "IAM_TOKEN"
                value_from {
                  secret_key_ref {
                    name = kubernetes_secret_v1.backup[0].metadata[0].name
                    key  = "iam-token"
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
