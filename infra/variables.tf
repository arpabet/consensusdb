variable "namespace" {
  description = "Kubernetes namespace (must already exist; state is stored here)"
  type        = string
}

variable "cos" {
  description = "Class of Service"
  type        = string
}

variable "project" {
  description = "Project name (registry image namespace)"
  type        = string
}

variable "deployment" {
  description = "StatefulSet / service name and registry image name"
  type        = string
}

variable "image_tag" {
  description = "Container image tag to deploy (e.g. the git SHA). Defaults to latest for local runs."
  type        = string
  default     = "latest"
}

variable "num_replicas" {
  description = "StatefulSet replicas. 1 = single-node (raft disabled). Multi-node raft needs additional serf/raft config — see the README."
  type        = number
  default     = 1
}

variable "storage_size" {
  description = "Per-replica persistent volume size for the badger data directory"
  type        = string
  default     = "10Gi"
}

variable "storage_class" {
  description = "StorageClass for the data volume; empty uses the cluster default"
  type        = string
  default     = ""
}

variable "data_dir" {
  description = "Mount path for the persistent data directory (consensusdb.data-dir)"
  type        = string
  default     = "/data"
}

variable "http_port" {
  description = "HTTP server port (health, metrics, welcome)"
  type        = number
  default     = 8441
}

variable "vrpc_port" {
  description = "value-rpc data-plane port that the store/providers/cdb client connects to"
  type        = number
  default     = 8444
}
