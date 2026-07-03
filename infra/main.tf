terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}

provider "kubernetes" {
  # kubeconfig from the local environment (the deploy workflow writes it from the
  # KUBE_CONFIG secret). The state backend (state.tf) uses the same kubeconfig.
  config_path = "~/.kube/config"
}

# Pull secret for the private registry, referenced by the StatefulSet so the
# cluster can pull the consensusdb image.
resource "kubernetes_secret_v1" "docker_registry" {
  metadata {
    name      = "docker-registry-secret"
    namespace = var.namespace
  }

  data = {
    ".dockerconfigjson" = jsonencode({
      auths = {
        "https://${var.registry_hostname}" = {
          username = var.registry_username
          password = var.registry_password
          email    = "ops@karagatan.com"
        }
      }
    })
  }

  type = "kubernetes.io/dockerconfigjson"
}
