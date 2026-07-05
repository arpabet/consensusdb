<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'
import Meter from './Meter.vue'

// Live cluster nodes: members with health and per-node CPU/Mem/Storage load
// (storage turns red over 80%) and overall load — read-only for everyone, so it
// lives on the dashboard. Add/remove controls appear only when canManage (admin).
const props = defineProps({ canManage: { type: Boolean, default: false } })
const nodes = ref([])
const replication = ref(false)
const error = ref('')
const busy = ref(false)
let timer = null

// add-node form
const showAdd = ref(false)
const addForm = ref({ nodeId: '', address: '' })
// remove confirmation
const confirmRemove = ref(null) // the node pending removal

async function refresh() {
  try {
    const r = await api.nodes()
    nodes.value = r.nodes || []
    replication.value = r.replication
  } catch (e) {
    error.value = e.message
  }
}

async function addNode() {
  error.value = ''
  busy.value = true
  try {
    await api.addNode(addForm.value.nodeId.trim(), addForm.value.address.trim())
    showAdd.value = false
    addForm.value = { nodeId: '', address: '' }
    await refresh()
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function removeNode() {
  error.value = ''
  busy.value = true
  const id = confirmRemove.value.id
  try {
    await api.removeNode(id)
    confirmRemove.value = null
    await refresh()
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

function fmtBytes(n) {
  if (!n) return '—'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(n) / Math.log(1024))
  return (n / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + u[i]
}

const overall = computed(() => {
  const up = nodes.value.filter((n) => n.up && n.metrics)
  if (!up.length) return null
  const avg = (sel) => Math.round(up.reduce((a, n) => a + sel(n.metrics), 0) / up.length)
  const maxDisk = Math.max(...up.map((n) => n.metrics.diskPercent))
  return { cpu: avg((m) => m.cpuPercent), mem: avg((m) => m.memPercent), disk: Math.round(maxDisk), upCount: up.length, total: nodes.value.length }
})

onMounted(() => { refresh(); timer = setInterval(refresh, 3000) })
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div v-if="overall" class="grid" style="margin-bottom:1rem">
    <div class="panel">
      <h2>Overall load ({{ overall.upCount }}/{{ overall.total }} up)</h2>
      <div class="meter-row"><span>CPU</span><Meter :pct="overall.cpu" /></div>
      <div class="meter-row"><span>Memory</span><Meter :pct="overall.mem" /></div>
      <div class="meter-row"><span>Storage (max)</span><Meter :pct="overall.disk" /></div>
    </div>
    <div class="panel">
      <h2>Cluster</h2>
      <div class="kv"><span class="k">Mode</span><span>{{ replication ? 'raft' : 'single-node' }}</span></div>
      <div class="kv"><span class="k">Nodes</span><span>{{ nodes.length }}</span></div>
      <button v-if="replication && canManage" @click="showAdd = !showAdd">+ Add node</button>
    </div>
  </div>

  <div v-if="showAdd" class="panel" style="margin-bottom:1rem">
    <h2>Introduce a node to the cluster</h2>
    <p class="hint">The node must already be running (e.g. a scaled-up pod). It joins as a raft voter.</p>
    <label>Node ID</label>
    <input v-model="addForm.nodeId" placeholder="the joiner's raft node id" />
    <label>Raft address</label>
    <input v-model="addForm.address" placeholder="consensusdb-3.consensusdb-headless…:8300" />
    <button :disabled="busy || !addForm.nodeId || !addForm.address" @click="addNode">Join to cluster</button>
    <button style="background:var(--panel-2);margin-left:0.5rem" @click="showAdd = false">Cancel</button>
  </div>

  <div class="panel">
    <h2>Nodes</h2>
    <table style="width:100%;border-collapse:collapse;font-size:0.85rem">
      <thead>
        <tr style="color:var(--muted);text-align:left">
          <th style="padding:0.3rem 0">Node</th><th>Address</th><th>Role</th><th>Status</th>
          <th style="width:9rem">CPU</th><th style="width:9rem">Mem</th><th style="width:9rem">Storage</th><th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="n in nodes" :key="n.id" style="border-top:1px solid var(--border)">
          <td style="padding:0.4rem 0">{{ n.id }} <span v-if="n.self" class="hint">(this)</span></td>
          <td class="mono" style="font-size:0.78rem">{{ n.address }}</td>
          <td>{{ n.leader ? 'leader' : (n.voter ? 'follower' : 'non-voter') }}</td>
          <td><span :class="'badge ' + (n.up ? 'ok' : 'err')">{{ n.up ? 'up' : 'down' }}</span></td>
          <td><Meter v-if="n.metrics" :pct="n.metrics.cpuPercent" /><span v-else class="hint">—</span></td>
          <td><Meter v-if="n.metrics" :pct="n.metrics.memPercent" /><span v-else class="hint">—</span></td>
          <td>
            <Meter v-if="n.metrics" :pct="n.metrics.diskPercent" :label="fmtBytes(n.metrics.diskUsedBytes)" />
            <span v-else class="hint">—</span>
          </td>
          <td style="text-align:right">
            <button v-if="replication && !n.leader && canManage" style="background:var(--err);padding:0.3rem 0.6rem"
              @click="confirmRemove = n">Remove</button>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="error" class="err-text">{{ error }}</p>
  </div>

  <!-- remove confirmation dialog -->
  <div v-if="confirmRemove" class="modal-backdrop" @click.self="confirmRemove = null">
    <div class="panel" style="max-width:26rem">
      <h2>Remove node</h2>
      <p>Remove <strong>{{ confirmRemove.id }}</strong> ({{ confirmRemove.address }}) from the cluster?
        It stops being a raft voter. Removing a node from a 3-node cluster leaves no
        fault tolerance until you add one back.</p>
      <div style="display:flex;gap:0.5rem;justify-content:flex-end">
        <button style="background:var(--panel-2)" @click="confirmRemove = null">Cancel</button>
        <button :disabled="busy" style="background:var(--err)" @click="removeNode">Remove node</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.meter-row { display: flex; align-items: center; gap: 0.75rem; padding: 0.35rem 0; }
.meter-row > span:first-child { width: 8rem; color: var(--muted); }
.modal-backdrop {
  position: fixed; inset: 0; background: rgba(0, 0, 0, 0.6);
  display: flex; align-items: center; justify-content: center; padding: 1rem;
}
</style>
