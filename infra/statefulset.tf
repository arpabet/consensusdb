# Optional at-rest encryption key, mounted into the pod as a secret when set.
resource "kubernetes_secret_v1" "encryption" {
  count = var.encryption_key != "" ? 1 : 0
  metadata {
    name      = "${var.deployment}-encryption"
    namespace = var.namespace
  }
  data = {
    "encryption-key" = var.encryption_key
  }
}

# Force a rolling restart when the image tag is unchanged (SHA-tagged deploys roll
# automatically; this covers re-deploying the same tag).
resource "null_resource" "image_change_trigger" {
  triggers = {
    image_version = var.image_tag
  }
  provisioner "local-exec" {
    command = "kubectl rollout restart statefulset ${var.deployment} -n ${var.namespace} || exit 0"
  }
}

resource "kubernetes_stateful_set_v1" "consensusdb" {
  metadata {
    name      = var.deployment
    namespace = var.namespace
    labels = {
      app     = var.deployment
      cos     = var.cos
      project = var.project
    }
  }

  spec {
    replicas     = var.num_replicas
    service_name = kubernetes_service_v1.headless.metadata[0].name

    selector {
      match_labels = {
        app = var.deployment
      }
    }

    template {
      metadata {
        labels = {
          app = var.deployment
        }
      }

      spec {
        container {
          name              = var.deployment
          image             = "${var.registry_hostname}/${var.project}/${var.deployment}:${var.image_tag}"
          image_pull_policy = "IfNotPresent"

          args = ["run"]

          port {
            name           = "http"
            container_port = var.http_port
          }
          port {
            name           = "grpc"
            container_port = var.grpc_port
          }
          port {
            name           = "vrpc"
            container_port = var.vrpc_port
          }

          # Persist badger under the mounted volume, and enable the value-rpc data
          # plane the store/providers/cdb client (e.g. staphi) connects to.
          env {
            name  = "CONSENSUSDB_DATA_DIR"
            value = var.data_dir
          }
          env {
            name  = "VRPC_SERVER_BIND_ADDRESS"
            value = "tcp://0.0.0.0:${var.vrpc_port}"
          }
          env {
            name  = "COS"
            value = var.cos
          }
          dynamic "env" {
            for_each = var.encryption_key != "" ? [1] : []
            content {
              name = "CONSENSUSDB_ENCRYPTION_KEY"
              value_from {
                secret_key_ref {
                  name = kubernetes_secret_v1.encryption[0].metadata[0].name
                  key  = "encryption-key"
                }
              }
            }
          }

          volume_mount {
            name       = "data"
            mount_path = var.data_dir
          }

          readiness_probe {
            tcp_socket {
              port = var.vrpc_port
            }
            initial_delay_seconds = 5
            period_seconds        = 10
          }
          liveness_probe {
            tcp_socket {
              port = var.http_port
            }
            initial_delay_seconds = 10
            period_seconds        = 20
          }
        }

        image_pull_secrets {
          name = kubernetes_secret_v1.docker_registry.metadata[0].name
        }
      }
    }

    volume_claim_template {
      metadata {
        name = "data"
      }
      spec {
        access_modes       = ["ReadWriteOnce"]
        storage_class_name = var.storage_class != "" ? var.storage_class : null
        resources {
          requests = {
            storage = var.storage_size
          }
        }
      }
    }
  }

  depends_on = [null_resource.image_change_trigger]
}

# Headless service gives each StatefulSet pod a stable DNS name
# (<name>-<ordinal>.<headless>.<ns>.svc) — the basis for raft peer identity.
resource "kubernetes_service_v1" "headless" {
  metadata {
    name      = "${var.deployment}-headless"
    namespace = var.namespace
  }
  spec {
    cluster_ip                  = "None"
    publish_not_ready_addresses = true
    selector = {
      app = var.deployment
    }
    port {
      name        = "http"
      port        = var.http_port
      target_port = var.http_port
    }
    port {
      name        = "grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
    }
    port {
      name        = "vrpc"
      port        = var.vrpc_port
      target_port = var.vrpc_port
    }
  }
}

# Stable in-cluster endpoint clients use, e.g. the cdb provider dials
# tcp://<deployment>.<namespace>.svc.cluster.local:<vrpc_port>.
resource "kubernetes_service_v1" "consensusdb" {
  metadata {
    name      = var.deployment
    namespace = var.namespace
  }
  spec {
    type = "ClusterIP"
    selector = {
      app = var.deployment
    }
    port {
      name        = "http"
      port        = var.http_port
      target_port = var.http_port
    }
    port {
      name        = "grpc"
      port        = var.grpc_port
      target_port = var.grpc_port
    }
    port {
      name        = "vrpc"
      port        = var.vrpc_port
      target_port = var.vrpc_port
    }
  }
}
