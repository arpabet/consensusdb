<script setup>
import { ref } from 'vue'
import { api } from '../api.js'

const emit = defineEmits(['done'])

// First-run wizard: create the admin, optionally generate the ledger CA, finish.
const step = ref(1)
const totalSteps = 3
const error = ref('')

const admin = ref({ username: '', password: '', confirm: '' })
const ca = ref(null) // { caKey, caPub } once generated

async function createAdmin() {
  error.value = ''
  if (admin.value.password.length < 8) { error.value = 'Password must be at least 8 characters.'; return }
  if (admin.value.password !== admin.value.confirm) { error.value = 'Passwords do not match.'; return }
  try {
    await api.bootstrap(admin.value.username.trim(), admin.value.password)
    step.value = 2
  } catch (e) { error.value = e.message }
}

async function generateCA() {
  error.value = ''
  try { ca.value = await api.generateCA() } catch (e) { error.value = e.message }
}

function download(name, b64) {
  const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
  const url = URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' }))
  const a = document.createElement('a')
  a.href = url; a.download = name; a.click()
  URL.revokeObjectURL(url)
}

function finish() {
  emit('done')
}
</script>

<template>
  <div class="panel" style="max-width:34rem;margin:2rem auto">
    <h2>Set up ConsensusDB — step {{ step }} of {{ totalSteps }}</h2>
    <div class="bar"><i :style="{ width: (step / totalSteps * 100) + '%' }"></i></div>

    <!-- Step 1: create the first admin -->
    <template v-if="step === 1">
      <p class="hint">Create the first administrator. This is only possible once.</p>
      <label>Admin username</label>
      <input v-model="admin.username" placeholder="root" />
      <label>Password (min 8 chars)</label>
      <input v-model="admin.password" type="password" />
      <label>Confirm password</label>
      <input v-model="admin.confirm" type="password" />
      <button :disabled="!admin.username" @click="createAdmin">Create admin →</button>
    </template>

    <!-- Step 2: ledger CA (optional) -->
    <template v-else-if="step === 2">
      <p class="hint">
        The verifiable ledger uses a certificate authority to certify node signing
        keys. Generate one now (keep the private key offline), or skip and use the
        <code>consensusdb ledger ca-init</code> CLI later.
      </p>
      <button @click="generateCA">Generate ledger CA</button>
      <template v-if="ca">
        <div class="kv" style="margin-top:0.75rem"><span class="k">CA generated</span><span class="badge ok">✓</span></div>
        <button @click="download('ca.key', ca.caKey)">Download ca.key (private — keep offline)</button>
        <button @click="download('ca.pub', ca.caPub)">Download ca.pub (public)</button>
      </template>
      <div style="margin-top:0.75rem">
        <button @click="step = 3">{{ ca ? 'Next →' : 'Skip →' }}</button>
      </div>
    </template>

    <!-- Step 3: done -->
    <template v-else>
      <p>Setup complete. Sign in with the <strong>username and password</strong> you just created.</p>
      <p class="hint">
        Authentication is not enforced until you set <code>AUTH_ENABLED=true</code> and restart the nodes.
        Clients can authenticate with a password (over TLS), an application token (Access page),
        or a client certificate (mutual TLS) — all are accepted once enabled.
      </p>
      <button @click="finish">Go to admin console →</button>
    </template>

    <p v-if="error" class="err-text">{{ error }}</p>
  </div>
</template>
