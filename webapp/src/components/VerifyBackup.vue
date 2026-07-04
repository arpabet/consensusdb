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
let timer = null

function stop() {
  if (timer) { clearInterval(timer); timer = null }
}
onUnmounted(stop)

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
  <label>Backup source (path or s3://bucket/key)</label>
  <input v-model="form.source" placeholder="s3://backups/cdb/full.dump" />

  <label>Dump password (blank for a plain dump)</label>
  <input v-model="form.password" type="password" />

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
