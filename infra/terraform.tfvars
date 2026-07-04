cos     = "prod"
project = "consensusdb"
# Shared, multi-tenant instance — runs in its own dedicated namespace. Per-tenant
# isolation is handled inside consensusdb (the cdb client's tenant → major key).
namespace  = "consensusdb"
deployment = "consensusdb"

# 3-node raft cluster: ordinal 0 bootstraps, 1 and 2 are joined by the leader
# (one-time `raft join`, see the README runbook). Quorum = 2, so one voter can
# fail (or drain — the PDB enforces max one) without losing writes.
num_replicas = 3

# Persistent data volume for the badger store.
storage_size = "100Gi"
# storage_class = "fast-ssd"   # empty uses the cluster default StorageClass
