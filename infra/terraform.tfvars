cos     = "prod"
project = "consensusdb"
# Shared, multi-tenant instance — runs in its own dedicated namespace. Per-tenant
# isolation is handled inside consensusdb (the cdb client's tenant → major key).
namespace  = "consensusdb"
deployment = "consensusdb"

# 3-node raft cluster: ordinal 0 bootstraps; the other ordinals enroll with the
# generated bootstrap-token Secret and are added as voters automatically — one
# `terraform apply` forms the cluster (see the README runbook). Quorum = 2, so
# one voter can fail (or drain — the PDB enforces max one) without losing writes.
num_replicas = 3

# Persistent data volume for the badger store.
storage_size = "100Gi"
# storage_class = "fast-ssd"   # empty uses the cluster default StorageClass

# Data-plane authentication: bootstrap the admin first (README auth runbook),
# then flip to true and re-apply.
# auth_enabled = true

# Expose the admin console/dashboard/metrics (8441) outside the cluster — only
# with auth_enabled = true; see the README "External access" section.
# external_access = "LoadBalancer"      # or "NodePort"
# external_expose_data_plane = true     # 8444 too — read the README caveat first
