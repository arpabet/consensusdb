<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'

// Live dashboard: cluster status, ledger head, per-region footprint, and
// store-wide read/write throughput. Reads/writes per second are derived from the
// delta between two /stats polls.
const cluster = ref(null)
const ledger = ref(null)
const regions = ref([])
const rate = ref({ reads: 0, writes: 0 })
const disk = ref(0)
let prev = null
let timer = null

function fmtBytes(n) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(n) / Math.log(1024))
  return (n / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + u[i]
}

async function tick() {
  try {
    const [c, l, s, rg] = await Promise.all([api.cluster(), api.ledgerStatus(), api.stats(), api.regions()])
    cluster.value = c
    ledger.value = l
    regions.value = rg.regions || []
    disk.value = s.diskBytes || 0
    if (prev) {
      const dt = (s.unix - prev.unix) / 1000
      if (dt > 0) {
        rate.value = {
          reads: Math.max(0, Math.round((s.reads - prev.reads) / dt)),
          writes: Math.max(0, Math.round((s.writes - prev.writes) / dt)),
        }
      }
    }
    prev = s
  } catch (e) {
    /* transient poll error; keep last values */
  }
}

onMounted(() => { tick(); timer = setInterval(tick, 2000) })
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="grid">
    <div class="panel">
      <h2>Cluster</h2>
      <div class="kv"><span class="k">Mode</span><span>{{ cluster?.replication ? 'raft' : 'single-node' }}</span></div>
      <template v-if="cluster?.replication">
        <div class="kv"><span class="k">State</span><span>{{ cluster.state }}</span></div>
        <div class="kv"><span class="k">Term</span><span>{{ cluster.term }}</span></div>
        <div class="kv"><span class="k">Applied</span><span>{{ cluster.appliedIndex }}</span></div>
        <div class="kv"><span class="k">Peers</span><span>{{ cluster.numPeers }}</span></div>
      </template>
      <!-- The transport-CA fingerprint: members of one cluster share it, so two
           clusters on one network are distinguishable at a glance. -->
      <template v-if="cluster?.clusterId">
        <div class="kv"><span class="k">Identity</span></div>
        <div class="mono" :title="cluster.clusterId">{{ cluster.clusterId }}</div>
      </template>
    </div>

    <div class="panel">
      <h2>Throughput</h2>
      <div class="kv"><span class="k">Reads / s</span><span>{{ rate.reads }}</span></div>
      <div class="kv"><span class="k">Writes / s</span><span>{{ rate.writes }}</span></div>
      <div class="kv"><span class="k">On disk (at rest)</span><span>{{ fmtBytes(disk) }}</span></div>
    </div>
  </div>

  <div class="panel" style="margin-top:1rem">
    <h2>Ledger head</h2>
    <template v-if="ledger?.available">
      <div class="kv"><span class="k">Height</span><span>{{ ledger.height }}</span></div>
      <div class="mono">{{ ledger.digest }}</div>
    </template>
    <p v-else class="hint">Ledger digest unavailable on this node.</p>
  </div>

  <div class="panel" style="margin-top:1rem">
    <h2>Regions</h2>
    <table style="width:100%;border-collapse:collapse;font-size:0.85rem">
      <thead>
        <tr style="color:var(--muted);text-align:left">
          <th style="padding:0.3rem 0">Tenant</th><th>Region</th><th style="text-align:right">Keys</th>
          <th style="text-align:right">On transfer</th><th style="text-align:right">On rest</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in regions" :key="r.tenant + '/' + r.region" style="border-top:1px solid var(--border)">
          <td style="padding:0.3rem 0">{{ r.tenant || '—' }}</td>
          <td>{{ r.region || '—' }}</td>
          <td style="text-align:right">{{ r.keys }}</td>
          <td style="text-align:right">{{ fmtBytes(r.transferBytes) }}</td>
          <td style="text-align:right">{{ fmtBytes(r.restBytes) }}</td>
        </tr>
        <tr v-if="!regions.length"><td colspan="5" class="hint" style="padding:0.5rem 0">No regions yet.</td></tr>
      </tbody>
    </table>
    <p class="hint">“On transfer” is the logical (uncompressed) size; “on rest” is on-disk (compressed/encrypted).</p>
  </div>
</template>
