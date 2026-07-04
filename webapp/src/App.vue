<script setup>
import { ref, onMounted } from 'vue'
import { api, setToken, hasCredential } from './api.js'
import VerifyBackup from './components/VerifyBackup.vue'

const token = ref('')
const authed = ref(false)
const cluster = ref(null)
const ledger = ref(null)
const error = ref('')

async function refresh() {
  error.value = ''
  try {
    cluster.value = await api.cluster()
    ledger.value = await api.ledgerStatus()
    authed.value = true
  } catch (e) {
    authed.value = false
    error.value = e.message
  }
}

function connect() {
  setToken(token.value.trim())
  refresh()
}

onMounted(() => {
  if (hasCredential()) refresh()
})
</script>

<template>
  <div class="wrap">
    <header>
      <h1>ConsensusDB</h1>
      <span class="sub">admin console</span>
    </header>

    <div v-if="!authed" class="panel" style="max-width:28rem">
      <h2>Connect</h2>
      <label>IAM token (service account)</label>
      <input v-model="token" type="password" placeholder="serviceAccount.xxxxx" @keyup.enter="connect" />
      <p class="hint">Needs a token with <code>cdb.proofs.read</code> (e.g. the auditor role).</p>
      <button @click="connect">Connect</button>
      <p v-if="error" class="err-text">{{ error }}</p>
    </div>

    <template v-else>
      <div class="grid">
        <div class="panel">
          <h2>Cluster</h2>
          <div class="kv"><span class="k">Replication</span><span>{{ cluster.replication ? 'raft' : 'single-node' }}</span></div>
          <template v-if="cluster.replication">
            <div class="kv"><span class="k">State</span><span>{{ cluster.state }}</span></div>
            <div class="kv"><span class="k">Term</span><span>{{ cluster.term }}</span></div>
            <div class="kv"><span class="k">Applied</span><span>{{ cluster.appliedIndex }}</span></div>
            <div class="kv"><span class="k">Peers</span><span>{{ cluster.numPeers }}</span></div>
          </template>
        </div>

        <div class="panel">
          <h2>Ledger head</h2>
          <template v-if="ledger.available">
            <div class="kv"><span class="k">Height</span><span>{{ ledger.height }}</span></div>
            <div class="kv"><span class="k">Digest</span></div>
            <div class="mono">{{ ledger.digest }}</div>
          </template>
          <p v-else class="hint">Ledger digest unavailable on this node.</p>
        </div>
      </div>

      <div class="panel" style="margin-top:1rem">
        <h2>Verify a backup against a quorum certificate</h2>
        <VerifyBackup />
      </div>
    </template>
  </div>
</template>
