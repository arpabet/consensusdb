cos     = "prod"
project = "consensusdb"
# Shared, multi-tenant instance — runs in its own dedicated namespace. Per-tenant
# isolation is handled inside consensusdb (the cdb client's tenant → major key).
namespace  = "consensusdb"
deployment = "consensusdb"

# Single-node (raft disabled). Raising this needs raft/serf config — see README.
num_replicas = 1

# Persistent data volume for the badger store.
storage_size = "10Gi"
# storage_class = "fast-ssd"   # empty uses the cluster default StorageClass
