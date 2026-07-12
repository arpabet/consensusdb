<script setup>
import { ref, onUnmounted } from 'vue'
import { api } from '../api.js'

// Form fields: the backup location, dump password, and the trust material
// (base64: CA public key, quorum certificate, node certs — newline-separated).
const form = ref({
  source: '',
  password: '',
  caCert: '',
  quorumCert: '',
  nodeCerts: '',
  threshold: 0,
})

const job = ref(null) // { state, progress, result, error }
const error = ref('')
const fetching = ref(false)
const fetched = ref(null) // /api/ledger/materials response
let timer = null

function stop() {
  if (timer) { clearInterval(timer); timer = null }
}
onUnmounted(stop)

// Prefill the trust-material fields from the live cluster: the aggregated
// quorum certificate of the current head, the signers' node certs, and the
// pinned CA public key. Every field stays editable — for an independent audit
// the CA public key (and ideally the certificate) come from records kept
// outside the cluster.
async function fetchFromCluster() {
  fetching.value = true
  error.value = ''
  try {
    const m = await api.ledgerMaterials()
    if (m.caPub) form.value.caCert = m.caPub
    if (m.quorumCert) form.value.quorumCert = m.quorumCert
    if (m.nodeCerts?.length) form.value.nodeCerts = m.nodeCerts.join('\n')
    fetched.value = m
  } catch (e) {
    error.value = e.message
  } finally {
    fetching.value = false
  }
}

async function start() {
  error.value = ''
  job.value = null
  stop()
  try {
    const payload = {
      source: form.value.source.trim(),
      password: form.value.password,
      caCert: form.value.caCert.trim(),
      quorumCert: form.value.quorumCert.trim(),
      nodeCerts: form.value.nodeCerts.split('\n').map((s) => s.trim()).filter(Boolean),
      threshold: Number(form.value.threshold) || 0,
    }
    const { id } = await api.startVerify(payload)
    poll(id)
  } catch (e) {
    error.value = e.message
  }
}

function poll(id) {
  timer = setInterval(async () => {
    try {
      const j = await api.verifyJob(id)
      job.value = j
      if (j.state !== 'running') stop()
    } catch (e) {
      error.value = e.message
      stop()
    }
  }, 400)
}

const running = () => job.value?.state === 'running'
</script>

<template>
  <p class="hint">
    Proves a backup is the state a majority of the cluster certified: the dump's
    stored chain head is checked against a quorum certificate — BLS co-signatures
    of CA-certified nodes — verifiable entirely offline. Paste trust material
    kept outside the cluster for an independent audit, or fetch the current
    materials from the live cluster for an operational consistency check.
  </p>

  <label>Backup source (path or s3://bucket/key)</label>
  <input v-model="form.source" placeholder="s3://backups/cdb/full.dump" />

  <label>Dump password (blank for a plain dump)</label>
  <input v-model="form.password" type="password" />

  <div style="margin:0.75rem 0">
    <button :disabled="fetching" @click="fetchFromCluster">
      {{ fetching ? 'Fetching…' : 'Fetch from cluster' }}
    </button>
  </div>
  <template v-if="fetched">
    <p class="hint">
      Fetched head height {{ fetched.height }} — {{ fetched.signers?.length || 0 }}/{{ fetched.members }}
      nodes signed. Cluster-supplied materials support a consistency check; an
      independent audit must take the CA public key from records kept outside
      the cluster.
    </p>
    <p v-for="wmsg in fetched.warnings || []" :key="wmsg" class="err-text">{{ wmsg }}</p>
  </template>

  <label>CA public key (base64)</label>
  <textarea v-model="form.caCert" />

  <label>Quorum certificate (base64)</label>
  <textarea v-model="form.quorumCert" />

  <label>Node certs (base64, one per line)</label>
  <textarea v-model="form.nodeCerts" />

  <label>Threshold (0 = certificate's signer count)</label>
  <input v-model="form.threshold" type="number" min="0" style="width:8rem" />

  <div>
    <button :disabled="running() || !form.source" @click="start">
      {{ running() ? 'Verifying…' : 'Verify backup' }}
    </button>
  </div>

  <p v-if="error" class="err-text">{{ error }}</p>

  <div v-if="job" style="margin-top:1rem">
    <div class="bar"><i :style="{ width: (job.progress || 0) + '%' }"></i></div>
    <div class="kv">
      <span>
        <span v-if="job.state === 'running'" class="badge run">running</span>
        <span v-else-if="job.result?.verified" class="badge ok">VERIFIED ✓</span>
        <span v-else class="badge err">NOT VERIFIED</span>
      </span>
      <span class="k">{{ job.progress || 0 }}%</span>
    </div>

    <template v-if="job.state !== 'running' && job.result">
      <div class="kv"><span class="k">Height</span><span>{{ job.result.height }}</span></div>
      <div class="kv"><span class="k">Signers</span><span>{{ job.result.signers }}</span></div>
      <div class="kv"><span class="k">Digest</span></div>
      <div class="mono">{{ job.result.digest }}</div>
      <p class="hint">{{ job.result.message }}</p>
    </template>
    <p v-if="job.error" class="err-text">{{ job.error }}</p>
  </div>
</template>
