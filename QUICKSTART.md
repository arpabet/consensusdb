<!--
 Copyright (c) 2025-2026 Karagatan LLC.
 SPDX-License-Identifier: BUSL-1.1
-->

# ConsensusDB — Quick Start

Get a node running on your laptop, sign in to the dashboard, create the
identities you need, and grow to a cluster. Every command below was run against
the current build.

- [1. Run one node locally](#1-run-one-node-locally)
- [2. Open the dashboard](#2-open-the-dashboard)
- [3. Create identities and tokens](#3-create-identities-and-tokens)
- [4. Turn on authentication (optional)](#4-turn-on-authentication-optional)
- [5. Connect a client](#5-connect-a-client)
- [6. Add more nodes (a cluster)](#6-add-more-nodes-a-cluster)
- [Where things live](#where-things-live)
- [Troubleshooting](#troubleshooting)

---

## 1. Run one node locally

**Prerequisites:** Go 1.25+ (to build the server) and, for the dashboard, Node 20+.

```bash
# build the server binary
go build            # produces ./consensusdb

# start a single node
./consensusdb run
```

That's it — no configuration required. A fresh node comes up **single-node**
(no replication) and, on first run, writes a durable settings file you can edit
later. You'll see it listening on two ports:

| What            | Address                          | Purpose                                   |
|-----------------|----------------------------------|-------------------------------------------|
| Admin console   | `http://localhost:8441`          | dashboard + REST API                      |
| Data plane      | `tcp://localhost:8444`           | where client apps read/write key-values   |

Data is stored under `~/.consensusdb/data`, and settings are written to
`~/.consensusdb/consensusdb.yaml`. Authentication is **off** by default, so you
can explore immediately; [section 4](#4-turn-on-authentication-optional) turns it
on.

Stop the node with `Ctrl-C`.

> Relocate everything with `CONSENSUSDB_HOME=/path` (settings + data), or point
> just the data at a disk with `CONSENSUSDB_DATA_DIR=/path`.

---

## 2. Open the dashboard

The dashboard is a small web app the node serves at **`/console`**. It ships as
source, so build it once:

```bash
cd webapp
npm ci
npm run build       # produces webapp/dist, which the node serves
cd ..
```

(If you skip this, `/console` shows a short "run `npm run build`" placeholder — the
node still runs fine, you just don't get the UI.)

Now open:

```
http://localhost:8441/console
```

**First run** shows an onboarding wizard: create your admin account (a username and
a password of at least 8 characters), pick an auth method, and optionally download
a ledger CA. That admin is stored in the database.

**Signing in** uses an IAM **token**. Create one from the CLI (see the next
section) and paste it into the sign-in box. With authentication off the console is
open, so any token gets you in; once you enable auth the token must belong to an
admin (for the Nodes/Database tabs) or an auditor (read-only).

---

## 3. Create identities and tokens

Identities are managed with `consensusdb iam …`, which talks to the running node's
data plane on `tcp://127.0.0.1:8444` (so run these in another terminal while the
node is up).

```bash
# create the first admin (or use the dashboard wizard — either works)
./consensusdb iam bootstrap admin --password 'change-me-please'
#   admin user "admin" created

# create a service account and print its token (shown once — copy it now)
./consensusdb iam sa-add my-app
#   service account "my-app" created
#   token (shown once): my-app.f70659b6aa4e…2e0956
```

Use that `my-app.…` token as the dashboard sign-in token, or as a client
credential when auth is on.

Other identity commands (see `./consensusdb iam <cmd> --help`):

| Command | What it does |
|---|---|
| `iam user-add <name> [--admin]`        | a password user |
| `iam sa-add <name> [--cert-idents …]`  | a service account (token and/or mTLS login) |
| `iam role-add …`                       | a custom role (a set of permissions) |
| `iam binding-add …`                    | grant a role to a user/SA at a scope (instance / tenant / region) |

---

## 4. Turn on authentication (optional)

By default the data plane is open — fine for local development. To require
credentials, create your identities first (section 3), then restart with auth on:

```bash
# stop the node (Ctrl-C), then:
AUTH_ENABLED=true ./consensusdb run
```

(Or set `auth.enabled: true` in `~/.consensusdb/consensusdb.yaml`.) Now every data
plane connection — and the console — must present a valid credential:

```bash
# the iam CLI authenticates with a token from the environment
IAM_TOKEN='my-app.f70659…' ./consensusdb iam sa-add another-app
```

Clients pass the token the same way (see the next section).

---

## 5. Connect a client

Client apps talk to the data plane (`8444`) through the
`go.arpabet.com/store/providers/cdb` provider — a `store.DataStore`. The address
model is `major = tenant`, `region = logical table`:

```go
import (
    "go.arpabet.com/store"
    cdb "go.arpabet.com/store/providers/cdb"
)

// name, data-plane address, tenant (major), region (table)
ds, err := cdb.New("my-app", "tcp://localhost:8444", "acme", "ACCOUNTS")
defer ds.Destroy()

// putIfAbsent (version 0), then get
ok, err := ds.CompareAndSetRaw(ctx, []byte("balance"), []byte("100.00"), 0, 0)
val, err := ds.GetRaw(ctx, []byte("balance"), nil, nil, false)
```

When auth is on, attach the token with the provider's credential option (see the
`store/providers/cdb` README for tokens, TLS/mTLS/QUIC, and the multi-tenant
`Multi` client).

---

## 6. Add more nodes (a cluster)

A cluster replicates every write through Raft. Turn a node into a cluster member by
giving it a **raft** and **serf** bind address; the seed node bootstraps, and you
add the rest from the leader.

The easiest per-machine setup detects this host's address for you:

```bash
# on the first (seed) machine
./consensusdb init --cluster            # writes a cluster settings file, detects the host IP
./consensusdb run

# on each additional machine
./consensusdb init --cluster --seed=false --host <that-machine-ip>
./consensusdb run
```

Each node prints its **node id** on startup. From the **seed/leader**, add each
other node by its id and raft address:

```bash
./consensusdb raft join <node-id> <that-machine-ip>:8300
./consensusdb raft config          # lists the members and who is leader
```

### Two nodes on one laptop (for testing)

On a single machine each node needs its **own home, ports, and node id**. Use your
machine's real LAN IP (not `127.0.0.1` — see [Troubleshooting](#troubleshooting)).
With `IP` set to that address:

```bash
# node 0 — the seed (leader)
CONSENSUSDB_HOME=~/.cdb0 NODE_ID=1 CONSENSUSDB_MODE=cluster RAFT_BOOTSTRAP=true \
  RAFT_BIND_ADDRESS=$IP:8300 SERF_BIND_ADDRESS=$IP:8301 \
  VRPC_SERVER_BIND_ADDRESS=0.0.0.0:8444 HTTP_SERVER_BIND_ADDRESS=0.0.0.0:8441 \
  ./consensusdb run

# node 1 — a joiner (another terminal)
CONSENSUSDB_HOME=~/.cdb1 NODE_ID=2 CONSENSUSDB_MODE=cluster RAFT_BOOTSTRAP=false \
  RAFT_BIND_ADDRESS=$IP:8310 SERF_BIND_ADDRESS=$IP:8311 \
  VRPC_SERVER_BIND_ADDRESS=0.0.0.0:8454 HTTP_SERVER_BIND_ADDRESS=0.0.0.0:8451 \
  ./consensusdb run

# from the leader, add node 1, then confirm two voters
RAFT_VRPC_CLIENT_ADDRESS=tcp://127.0.0.1:8444 ./consensusdb raft join 2 $IP:8310
RAFT_VRPC_CLIENT_ADDRESS=tcp://127.0.0.1:8444 ./consensusdb raft config
#   server_list:{node_id:"1" … } server_list:{node_id:"2" … }
```

Keep the data-plane port exactly `raft port + 144` on every node (`8300→8444`,
`8310→8454`): the control plane derives each peer's address from that offset.

Once nodes are joined you can manage them from the dashboard's **Nodes** tab (an
admin token): live up/down health, per-node CPU/memory/storage, and add/remove.

---

## Where things live

| Path / variable                    | Default                          | What |
|------------------------------------|----------------------------------|------|
| `~/.consensusdb/consensusdb.yaml`  | written on first run             | settings file (edit + restart) |
| `~/.consensusdb/data`              | data directory                   | the badger store |
| `CONSENSUSDB_HOME`                 | `~/.consensusdb`                 | relocate settings + data |
| `CONSENSUSDB_CONFIG`               | `<home>/consensusdb.yaml`        | settings file path |
| `-c file.yaml`                     | —                                | load an extra settings file |

Precedence for any setting: command-line `-D key=value` > environment variable >
settings file > built-in default. Env names uppercase the property and turn `.`/`-`
into `_` (e.g. `vrpc-server.bind-address` → `VRPC_SERVER_BIND_ADDRESS`).

---

## Troubleshooting

**The dashboard shows a placeholder page.** The web app isn't built. Run
`cd webapp && npm ci && npm run build`, then reload `/console`.

**A cluster node fails to start with `too many colons in address`.** Raft is
advertising an address it can't parse. Bind raft to a concrete **IPv4** address
(your LAN IP), not `0.0.0.0` or `127.0.0.1` — `consensusdb init --cluster` detects
and uses the right one for you.

**`consensusdb run` panics or exits immediately.** A fresh single-node run should
just work. If you set `CONSENSUSDB_MODE=cluster` you must also provide
`RAFT_BIND_ADDRESS`/`SERF_BIND_ADDRESS` (or run `init --cluster`); otherwise the
node exits with a message telling you exactly that.

**`iam` commands report `connect …: missing address` or `connection refused`.**
The node isn't running, or it's on a non-default address. Start `consensusdb run`
first, or point the CLI with `IAM_ADDRESS=tcp://host:8444`.
