variable "registry_hostname" {
  description = "The hostname for the private registry"
  sensitive   = true
}

variable "registry_username" {
  description = "The username for the private registry"
  sensitive   = true
}

variable "registry_password" {
  description = "The password for the private registry"
  sensitive   = true
}

variable "encryption_key" {
  description = "Base64 AES-256 master key for badger encryption at rest. Empty means unencrypted."
  type        = string
  sensitive   = true
  default     = ""
}
