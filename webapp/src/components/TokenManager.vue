<!--
  Personal access token (PAT) management for one user: mint an expiring "pat-…"
  bearer token (shown once), list existing PATs, and revoke them. Used by Users.vue.
-->
<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="panel" style="max-width:34rem;width:100%">
      <h2>Personal access tokens — {{ user }}</h2>
      <p class="hint">
        Expiring bearer tokens the user authenticates with (as
        <span class="mono">user:{{ user }}</span>). Shown once at creation.
      </p>

      <p v-if="error" class="hint" style="color:var(--err)">{{ error }}</p>

      <!-- freshly minted token: shown once -->
      <div v-if="minted" class="panel" style="background:var(--panel-2);margin-top:0.5rem">
        <p class="hint">New token — copy it now, it will not be shown again:</p>
        <div style="display:flex;gap:0.5rem;align-items:center">
          <input class="mono" :value="minted.token" readonly style="font-size:0.8rem" @focus="$event.target.select()" />
          <button @click="copy(minted.token)">Copy</button>
        </div>
      </div>

      <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin-top:0.5rem">
        <thead>
          <tr style="color:var(--muted);text-align:left">
            <th style="padding:0.3rem 0">Label</th><th>Created</th><th>Expires</th><th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="tk in tokens" :key="tk.id" style="border-top:1px solid var(--border)">
            <td style="padding:0.35rem 0">{{ tk.label || '—' }}</td>
            <td class="hint">{{ fmt(tk.createdAt) }}</td>
            <td class="hint" :style="expired(tk.expiresAt) ? 'color:var(--err)' : ''">{{ fmt(tk.expiresAt) }}{{ expired(tk.expiresAt) ? ' (expired)' : '' }}</td>
            <td style="text-align:right">
              <button style="background:var(--err);padding:0.1rem 0.45rem;font-size:0.75rem" @click="revoke(tk.id)">revoke</button>
            </td>
          </tr>
          <tr v-if="!tokens.length"><td colspan="4" class="hint" style="padding:0.4rem 0">No tokens.</td></tr>
        </tbody>
      </table>

      <div style="display:flex;gap:0.5rem;align-items:flex-end;margin-top:0.75rem">
        <div style="flex:1"><label>Label</label><input v-model="label" placeholder="laptop, CI, …" /></div>
        <div><label>Valid (days)</label><input type="number" v-model.number="ttlDays" min="1" style="width:7rem" /></div>
        <button :disabled="busy" @click="mint">Create token</button>
      </div>

      <div style="display:flex;justify-content:flex-end;margin-top:0.75rem">
        <button style="background:var(--panel-2)" @click="$emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'

const props = defineProps({ user: { type: String, required: true } })
defineEmits(['close'])

const tokens = ref([])
const minted = ref(null)
const label = ref('')
const ttlDays = ref(90)
const busy = ref(false)
const error = ref('')

async function load() {
  try {
    tokens.value = (await api.userTokens(props.user)).tokens || []
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

function mint() {
  run(async () => {
    minted.value = await api.createUserToken(props.user, label.value.trim(), ttlDays.value)
    label.value = ''
    await load()
  })
}

function revoke(id) {
  run(async () => {
    await api.revokeUserToken(props.user, id)
    await load()
  })
}

function copy(text) {
  navigator.clipboard?.writeText(text)
}

function fmt(unix) {
  return unix ? new Date(unix * 1000).toLocaleDateString() : '—'
}
function expired(unix) {
  return unix && unix * 1000 < Date.now()
}

onMounted(load)
</script>
