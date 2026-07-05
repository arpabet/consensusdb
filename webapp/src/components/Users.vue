<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'
import CertManager from './CertManager.vue'

// User management: password identities. Their role/tenant assignments are managed
// on the IAM tab; here we show a summary and allow create/delete. Filterable
// because the list can get long.
const users = ref([])
const bindings = ref([])
const error = ref('')
const busy = ref(false)
const filter = ref('')
const newUser = ref({ username: '', password: '' })
const confirmDelete = ref(null)
const certFor = ref(null) // user whose mTLS certificates are being managed

// user:name → [{role, scope}]
const accessByUser = computed(() => {
  const map = {}
  for (const b of bindings.value) {
    for (const m of b.members || []) {
      if (m.startsWith('user:')) (map[m] || (map[m] = [])).push({ role: b.role, scope: b.scope })
    }
  }
  return map
})

const shown = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return users.value.filter((u) => !f || u.name.toLowerCase().includes(f))
})

function scopeLabel(scope) {
  if (!scope) return 'all'
  const [t, r] = scope.split('/')
  return r ? `${t}/${r}` : t
}

async function refresh() {
  error.value = ''
  try {
    const [u, b] = await Promise.all([api.users(), api.bindings()])
    users.value = u.users || []
    bindings.value = b.bindings || []
  } catch (e) {
    error.value = e.message
  }
}

async function run(fn) {
  error.value = ''
  busy.value = true
  try { await fn(); await refresh() } catch (e) { error.value = e.message } finally { busy.value = false }
}

function createUser() {
  run(async () => {
    await api.createUser(newUser.value.username.trim(), newUser.value.password)
    newUser.value = { username: '', password: '' }
  })
}

function doDelete() {
  const name = confirmDelete.value.name
  run(() => api.deleteUser(name))
  confirmDelete.value = null
}

onMounted(refresh)
</script>

<template>
  <div class="panel" style="margin-bottom:1rem">
    <h2>Create user</h2>
    <div class="grid" style="align-items:end">
      <div><label>Username</label><input v-model="newUser.username" placeholder="alice" /></div>
      <div><label>Password (min 8)</label><input v-model="newUser.password" type="password" /></div>
      <div><button :disabled="busy || !newUser.username || newUser.password.length < 8" @click="createUser">Create user</button></div>
    </div>
  </div>

  <div class="panel">
    <div style="display:flex;align-items:center;margin-bottom:0.5rem">
      <h2 style="margin:0">Users ({{ users.length }})</h2>
      <input v-model="filter" placeholder="filter…" style="margin-left:auto;max-width:14rem" />
    </div>
    <table style="width:100%;border-collapse:collapse;font-size:0.85rem">
      <thead>
        <tr style="color:var(--muted);text-align:left">
          <th style="padding:0.4rem 0">Username</th><th>Roles (role @ scope)</th><th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="u in shown" :key="u.name" style="border-top:1px solid var(--border)">
          <td style="padding:0.45rem 0" class="mono">{{ u.name }}</td>
          <td>
            <span v-for="(a, i) in (accessByUser['user:' + u.name] || [])" :key="i" class="badge"
              style="margin:0.1rem 0.2rem 0.1rem 0;background:var(--panel-2)">{{ a.role }} @ {{ scopeLabel(a.scope) }}</span>
            <span v-if="!(accessByUser['user:' + u.name] || []).length" class="hint">—</span>
          </td>
          <td style="text-align:right;white-space:nowrap">
            <button style="background:var(--panel-2);padding:0.25rem 0.5rem;margin-right:0.35rem" @click="certFor = u">Certificates</button>
            <button style="background:var(--err);padding:0.25rem 0.5rem" @click="confirmDelete = u">Delete</button>
          </td>
        </tr>
        <tr v-if="!shown.length"><td colspan="3" class="hint" style="padding:0.5rem 0">No users.</td></tr>
      </tbody>
    </table>
    <p class="hint" style="margin-top:0.5rem">Grant roles / assign tenants on the <strong>IAM</strong> tab.</p>
    <p v-if="error" class="err-text">{{ error }}</p>
  </div>

  <CertManager
    v-if="certFor"
    :principal="'user:' + certFor.name"
    :label="certFor.name"
    @close="certFor = null"
  />

  <div v-if="confirmDelete" class="modal-backdrop" @click.self="confirmDelete = null">
    <div class="panel" style="max-width:24rem">
      <h2>Delete user</h2>
      <p>Remove <strong>{{ confirmDelete.name }}</strong>? Their sign-in stops working after the policy reloads.</p>
      <div style="display:flex;gap:0.5rem;justify-content:flex-end">
        <button style="background:var(--panel-2)" @click="confirmDelete = null">Cancel</button>
        <button :disabled="busy" style="background:var(--err)" @click="doDelete">Delete</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-backdrop {
  position: fixed; inset: 0; background: rgba(0, 0, 0, 0.6);
  display: flex; align-items: center; justify-content: center; padding: 1rem;
}
</style>
