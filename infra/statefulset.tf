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

# Pre-shared cluster bootstrap token: every ordinal gets the same secret; fresh
# joiners (RAFT_BOOTSTRAP=false) enroll with it on first start and the leader
# adopts it as a reusable join record, so `terraform apply` alone forms the
# cluster — no manual token minting. A node's identity persists on its data
# volume, so the token is only read on a node's very first start; rotate with
# `terraform apply -replace=random_password.bootstrap_token`.
resource "random_password" "bootstrap_token" {
  length  = 48
  special = false
}

resource "kubernetes_secret_v1" "bootstrap_token" {
  metadata {
    name      = "${var.deployment}-bootstrap-token"
    namespace = var.namespace
  }
  data = {
    "bootstrap-token" = random_password.bootstrap_token.result
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

    # Start all ordinals at once instead of gating each on its predecessor's
    # readiness: joiners that come up before the seed leads simply retry via the
    # container restart backoff and enroll when it does. With the default
    # OrderedReady, a joiner waiting on the seed would block pod creation (and
    # rolling updates) behind its own not-ready state. Updates still roll one pod
    # at a time — this only affects creation/scaling.
    pod_management_policy = "Parallel"

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
        # Spread raft voters across nodes so one node failure costs one voter,
        # not quorum. Preferred (not required) so small clusters still schedule.
        affinity {
          pod_anti_affinity {
            preferred_during_scheduling_ignored_during_execution {
              weight = 100
              pod_affinity_term {
                topology_key = "kubernetes.io/hostname"
                label_selector {
                  match_labels = {
                    app = var.deployment
                  }
                }
              }
            }
          }
        }

        container {
          name              = var.deployment
          image             = "${var.registry_hostname}/${var.project}/${var.deployment}:${var.image_tag}"
          image_pull_policy = "IfNotPresent"

          # Ordinal 0 is the raft seed: it bootstraps a single-voter cluster on
          # first start. The other ordinals enroll themselves with the shared
          # bootstrap token (CONSENSUSDB_BOOTSTRAP_TOKEN below) and are added as
          # voters by the leader — formation is automatic (see the README
          # runbook). Peers record each node under its stable headless DNS name
          # (CONSENSUSDB_ADVERTISE_ADDRESS), so a reschedule doesn't strand the
          # membership on a dead pod IP.
          command = ["/bin/sh", "-c"]
          args = [<<-EOT
            ordinal="$${HOSTNAME##*-}"
            if [ "$ordinal" = "0" ]; then export RAFT_BOOTSTRAP=true; else export RAFT_BOOTSTRAP=false; fi
            export CONSENSUSDB_ADVERTISE_ADDRESS="$${HOSTNAME}.${var.deployment}-headless.${var.namespace}.svc.cluster.local:${var.raft_port}"
            exec /app/consensusdb run
          EOT
          ]

          port {
            name           = "http"
            container_port = var.http_port
          }
          port {
            name           = "vrpc"
            container_port = var.vrpc_port
          }
          port {
            name           = "raft"
            container_port = var.raft_port
          }
          port {
            name           = "serf"
            container_port = var.serf_port
          }

          # Persist badger under the mounted volume, and enable the value-rpc data
          # plane the store/providers/cdb client (e.g. staphi) connects to.
          env {
            name  = "CONSENSUSDB_DATA_DIR"
            value = var.data_dir
          }
          env {
            name = "VRPC_SERVER_BIND_ADDRESS"
            # Bare host:port (no scheme): the raft control-plane client pool derives
            # its port offset from this via net.SplitHostPort, which rejects a
            # "tcp://" prefix.
            value = "0.0.0.0:${var.vrpc_port}"
          }
          # Enable raft replication (RaftHost requires both bind addresses). The
          # advertised peer address is derived from the pod's private IP.
          env {
            name  = "RAFT_BIND_ADDRESS"
            value = "0.0.0.0:${var.raft_port}"
          }
          env {
            name  = "SERF_BIND_ADDRESS"
            value = "0.0.0.0:${var.serf_port}"
          }
          # Raft snapshots and serf artifacts live on the data volume too.
          env {
            name  = "APPLICATION_DATA_DIR"
            value = var.data_dir
          }
          # Lets `kubectl exec <pod> -- /app/consensusdb raft …` dial this node.
          env {
            name  = "RAFT_VRPC_CLIENT_ADDRESS"
            value = "tcp://127.0.0.1:${var.vrpc_port}"
          }
          # Cluster formation: a fresh joiner redeems the deployment-wide
          # bootstrap token against the ClusterIP service — it routes to ready
          # nodes only (the seed, during formation) and the console forwards
          # enrollment to the raft leader. Both are ignored once the node's
          # identity exists on its data volume.
          env {
            name = "CONSENSUSDB_BOOTSTRAP_TOKEN"

            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.bootstrap_token.metadata[0].name
                key  = "bootstrap-token"
              }
            }
          }
          env {
            name  = "CONSENSUSDB_JOIN_PEER"
            value = "http://${var.deployment}.${var.namespace}.svc.cluster.local:${var.http_port}"
          }
          # Data-plane authentication. Flip to true only after `iam bootstrap`
          # has created the admin (see the README auth runbook).
          env {
            name  = "AUTH_ENABLED"
            value = tostring(var.auth_enabled)
          }
          env {
            name  = "COS"
            value = var.cos
          }
          env {
            name = "CONSENSUSDB_ENCRYPTION_KEY"

            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.encryption[0].metadata[0].name
                key  = "encryption-key"
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
            http_get {
              path = "/healthz"
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
      name        = "vrpc"
      port        = var.vrpc_port
      target_port = var.vrpc_port
    }
    port {
      name        = "raft"
      port        = var.raft_port
      target_port = var.raft_port
    }
    port {
      name        = "serf"
      port        = var.serf_port
      target_port = var.serf_port
    }
  }
}

# Voluntary disruptions (node drains, upgrades) may take at most one voter at a
# time, so a 3-node cluster never loses raft quorum to maintenance.
resource "kubernetes_pod_disruption_budget_v1" "consensusdb" {
  metadata {
    name      = var.deployment
    namespace = var.namespace
  }
  spec {
    max_unavailable = "1"
    selector {
      match_labels = {
        app = var.deployment
      }
    }
  }
}

# Optional exposure outside the cluster (README "External access"). The admin
# console/dashboard/metrics (http) are follower-safe behind any entry point —
# every node serves them and forwards admin actions to the raft leader
# server-side. The data plane (vrpc, opt-in) is NOT: any node serves reads, but
# a write landing on a follower is answered with a redirect to the leader's
# in-cluster endpoint, which external clients cannot reach. Enable auth before
# exposing either port.
resource "kubernetes_service_v1" "external" {
  count = var.external_access != "" ? 1 : 0
  metadata {
    name      = "${var.deployment}-external"
    namespace = var.namespace
  }
  spec {
    type = var.external_access
    selector = {
      app = var.deployment
    }
    port {
      name        = "http"
      port        = var.http_port
      target_port = var.http_port
    }
    dynamic "port" {
      for_each = var.external_expose_data_plane ? [1] : []
      content {
        name        = "vrpc"
        port        = var.vrpc_port
        target_port = var.vrpc_port
      }
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
      name        = "vrpc"
      port        = var.vrpc_port
      target_port = var.vrpc_port
    }
  }
}
