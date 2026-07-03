# Terraform state lives in a Kubernetes secret in the deployment namespace. This
# is a shared, multi-tenant instance, so it runs in its own `consensusdb`
# namespace. The namespace must already exist. variables are not allowed here.
terraform {
  backend "kubernetes" {
    secret_suffix = "consensusdb"
    config_path   = "~/.kube/config"
    namespace     = "consensusdb"
  }
}
