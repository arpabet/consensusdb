<!--
  mTLS certificate management for one principal (a user or a service account),
  backed by the single built-in CA. Issue a cert (the console mints the key, the CA
  signs it, and the key is offered for download once) or register an identity from
  a certificate the owner already holds. Used by Access.vue (service accounts) and
  Users.vue (users).
-->
<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="panel" style="max-width:34rem;width:100%">
      <h2>mTLS certificates — {{ label }}</h2>
      <p class="hint">
        A client certificate authenticates as <span class="mono">{{ principal }}</span>.
        Issue one from the built-in CA (download the private key once) or register an
        identity (SAN URI or CN) from a certificate the owner already holds.
      </p>

      <p v-if="error" class="hint" style="color:var(--err)">{{ error }}</p>

      <div
        v-for="c in certs"
        :key="c.identity"
        style="display:flex;align-items:center;gap:0.5rem;padding:0.25rem 0;border-top:1px solid var(--border)"
      >
        <span class="mono" style="font-size:0.8rem;word-break:break-all">{{ c.identity }}</span>
        <span class="hint" style="font-size:0.7rem">{{ c.issued ? 'issued' : 'registered' }}</span>
        <button
          style="background:var(--err);padding:0.1rem 0.45rem;font-size:0.75rem;margin-left:auto"
          @click="revoke(c.identity)"
        >revoke</button>
      </div>
      <p v-if="!certs.length" class="hint">No certificates yet.</p>

      <!-- issued material: shown once, for download -->
      <div v-if="issued" class="panel" style="background:var(--panel-2);margin-top:0.75rem">
        <p class="hint">
          Issued <span class="mono">{{ issued.identity }}</span>. The private key is shown
          once — download all three now.
        </p>
        <div style="display:flex;gap:0.75rem;flex-wrap:wrap">
          <a :href="dl(issued.keyPem)" :download="fileName('key')">▼ private key</a>
          <a :href="dl(issued.certPem)" :download="fileName('cert')">▼ certificate</a>
          <a :href="dl(issued.caPem)" download="ca.pem">▼ CA cert</a>
        </div>
      </div>

      <div style="display:flex;gap:0.5rem;align-items:flex-end;margin-top:0.75rem">
        <div>
          <label>Valid for (days)</label>
          <input type="number" v-model.number="ttlDays" min="1" style="width:7rem" />
        </div>
        <button :disabled="busy" @click="issue">Issue &amp; download</button>
      </div>

      <label style="margin-top:0.5rem">Register an existing identity (SAN URI or CN)</label>
      <input v-model="identity" placeholder="cdb://user/alice   or   CN=alice-laptop" @keyup.enter="register" />

      <div style="display:flex;gap:0.5rem;justify-content:flex-end;margin-top:0.75rem">
        <button :disabled="busy || !identity.trim()" @click="register">Register</button>
        <button style="background:var(--panel-2)" @click="$emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'

const props = defineProps({ principal: { type: String, required: true }, label: String })
defineEmits(['close'])

const certs = ref([])
const issued = ref(null)
const ttlDays = ref(365)
const identity = ref('')
const busy = ref(false)
const error = ref('')

async function load() {
  try {
    certs.value = (await api.certs(props.principal)).certs || []
  } catch (e) {
    error.value = e.message
  }
}

async function run(fn) {
  busy.value = true
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

function issue() {
  run(async () => {
    issued.value = await api.issueCert(props.principal, ttlDays.value)
    await load()
  })
}

function register() {
  const id = identity.value.trim()
  if (!id) return
  run(async () => {
    await api.registerCert(props.principal, id)
    identity.value = ''
    await load()
  })
}

function revoke(id) {
  run(async () => {
    await api.revokeCert(id)
    await load()
  })
}

// Offer PEM text as a client-side download (no round trip; the key never persists
// server-side).
function dl(text) {
  return 'data:application/x-pem-file;charset=utf-8,' + encodeURIComponent(text)
}
function fileName(kind) {
  return props.principal.replace(/[^a-zA-Z0-9]+/g, '-') + '-' + kind + '.pem'
}

onMounted(load)
</script>
