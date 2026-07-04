<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'

// Admin IAM management: users (password login), service accounts (application
// tokens), and role bindings at instance / tenant / region scope.
const users = ref([])
const accounts = ref([])
const roles = ref({})
const bindings = ref([])
const error = ref('')
const busy = ref(false)

// forms
const newUser = ref({ username: '', password: '', admin: false })
const newSA = ref({ name: '' })
const newToken = ref(null) // { name, token } shown once after creation
const grantForm = ref({ role: '', members: '', tenant: '', region: '' })
const confirmDelete = ref(null) // { kind, name }

const roleNames = computed(() => Object.keys(roles.value).sort())

async function refresh() {
  error.value = ''
  try {
    const [u, s, r, b] = await Promise.all([api.users(), api.serviceAccounts(), api.roles(), api.bindings()])
    users.value = u.users || []
    accounts.value = s.serviceAccounts || []
    roles.value = r.roles || {}
    bindings.value = b.bindings || []
    if (!grantForm.value.role && roleNames.value.length) grantForm.value.role = roleNames.value.find((n) => n.includes('auditor')) || roleNames.value[0]
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
    await api.createUser(newUser.value.username.trim(), newUser.value.password, newUser.value.admin)
    newUser.value = { username: '', password: '', admin: false }
  })
}

async function createSA() {
  error.value = ''
  busy.value = true
  try {
    const res = await api.createServiceAccount(newSA.value.name.trim())
    newToken.value = res // { name, token } — shown once
    newSA.value = { name: '' }
    await refresh()
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

function grant() {
  const members = grantForm.value.members.split(',').map((m) => m.trim()).filter(Boolean)
  run(() => api.grant(grantForm.value.role, members, grantForm.value.tenant.trim(), grantForm.value.region.trim()))
}

function revokeBinding(b) {
  run(() => {
    const [tenant, region] = (b.scope || '').split('/')
    return api.revoke(b.role, b.members, tenant || '', region || '')
  })
}

function doDelete() {
  const d = confirmDelete.value
  run(() => (d.kind === 'user' ? api.deleteUser(d.name) : api.deleteServiceAccount(d.name)))
  confirmDelete.value = null
}

function scopeLabel(s) { return s === '' ? 'instance (whole cluster)' : s }

onMounted(refresh)
</script>

<template>
  <p v-if="error" class="err-text" style="margin-bottom:1rem">{{ error }}</p>

  <!-- token shown once -->
  <div v-if="newToken" class="panel" style="margin-bottom:1rem;border-color:var(--accent)">
    <h2>Token for “{{ newToken.name }}” — copy it now</h2>
    <p class="hint">This is shown only once; it is not recoverable. Give it to the client app as its credential.</p>
    <input readonly :value="newToken.token" class="mono" style="width:100%" @focus="$event.target.select()" />
    <button style="margin-top:0.5rem" @click="newToken = null">Done</button>
  </div>

  <div class="grid" style="margin-bottom:1rem">
    <!-- Users -->
    <div class="panel">
      <h2>Users</h2>
      <p class="hint">Human identities with a password login.</p>
      <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin:0.5rem 0">
        <thead><tr style="color:var(--muted);text-align:left"><th style="padding:0.3rem 0">User</th><th>Role</th><th></th></tr></thead>
        <tbody>
          <tr v-for="u in users" :key="u.name" style="border-top:1px solid var(--border)">
            <td style="padding:0.4rem 0">{{ u.name }}</td>
            <td><span v-if="u.admin" class="badge run">admin</span><span v-else class="hint">user</span></td>
            <td style="text-align:right"><button style="background:var(--err);padding:0.25rem 0.5rem" @click="confirmDelete = { kind: 'user', name: u.name }">Delete</button></td>
          </tr>
        </tbody>
      </table>
      <label>Username</label>
      <input v-model="newUser.username" placeholder="alice" />
      <label>Password (min 8)</label>
      <input v-model="newUser.password" type="password" />
      <label style="display:flex;gap:0.4rem;align-items:center;margin-top:0.4rem"><input type="checkbox" v-model="newUser.admin" /> Administrator</label>
      <button :disabled="busy || !newUser.username || newUser.password.length < 8" @click="createUser">Create user</button>
    </div>

    <!-- Service accounts / tokens -->
    <div class="panel">
      <h2>Application tokens</h2>
      <p class="hint">Service accounts authenticated by an API token (for client apps).</p>
      <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin:0.5rem 0">
        <thead><tr style="color:var(--muted);text-align:left"><th style="padding:0.3rem 0">Service account</th><th>Token</th><th></th></tr></thead>
        <tbody>
          <tr v-for="s in accounts" :key="s.name" style="border-top:1px solid var(--border)">
            <td style="padding:0.4rem 0">{{ s.name }}</td>
            <td><span :class="'badge ' + (s.hasToken ? 'ok' : '')">{{ s.hasToken ? 'set' : 'none' }}</span></td>
            <td style="text-align:right"><button style="background:var(--err);padding:0.25rem 0.5rem" @click="confirmDelete = { kind: 'sa', name: s.name }">Revoke</button></td>
          </tr>
        </tbody>
      </table>
      <label>Name</label>
      <input v-model="newSA.name" placeholder="my-app" />
      <button :disabled="busy || !newSA.name" @click="createSA">Create &amp; mint token</button>
    </div>
  </div>

  <!-- Access bindings -->
  <div class="panel">
    <h2>Access (role bindings)</h2>
    <p class="hint">Grant a role to a member (<code>user:alice</code>, <code>serviceAccount:my-app</code>, <code>group:team</code>)
      at instance, tenant, or a tenant's region.</p>
    <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin:0.5rem 0">
      <thead><tr style="color:var(--muted);text-align:left"><th style="padding:0.3rem 0">Scope</th><th>Role</th><th>Members</th><th></th></tr></thead>
      <tbody>
        <tr v-for="(b, i) in bindings" :key="i" style="border-top:1px solid var(--border)">
          <td style="padding:0.4rem 0">{{ scopeLabel(b.scope) }}</td>
          <td class="mono" style="font-size:0.78rem">{{ b.role }}</td>
          <td>{{ (b.members || []).join(', ') }}</td>
          <td style="text-align:right"><button style="background:var(--err);padding:0.25rem 0.5rem" @click="revokeBinding(b)">Revoke</button></td>
        </tr>
        <tr v-if="!bindings.length"><td colspan="4" class="hint" style="padding:0.5rem 0">No bindings yet.</td></tr>
      </tbody>
    </table>

    <div class="grid" style="align-items:end">
      <div>
        <label>Role</label>
        <select v-model="grantForm.role" style="width:100%">
          <option v-for="n in roleNames" :key="n" :value="n">{{ n }}</option>
        </select>
      </div>
      <div>
        <label>Members (comma-separated)</label>
        <input v-model="grantForm.members" placeholder="user:alice, serviceAccount:my-app" />
      </div>
      <div>
        <label>Tenant (optional)</label>
        <input v-model="grantForm.tenant" placeholder="acme" />
      </div>
      <div>
        <label>Region (optional)</label>
        <input v-model="grantForm.region" placeholder="USERS" />
      </div>
    </div>
    <button :disabled="busy || !grantForm.role || !grantForm.members" @click="grant">Grant</button>
  </div>

  <!-- delete confirmation -->
  <div v-if="confirmDelete" class="modal-backdrop" @click.self="confirmDelete = null">
    <div class="panel" style="max-width:24rem">
      <h2>{{ confirmDelete.kind === 'user' ? 'Delete user' : 'Revoke service account' }}</h2>
      <p>Remove <strong>{{ confirmDelete.name }}</strong>? Existing sessions/tokens stop working after the policy reloads.</p>
      <div style="display:flex;gap:0.5rem;justify-content:flex-end">
        <button style="background:var(--panel-2)" @click="confirmDelete = null">Cancel</button>
        <button :disabled="busy" style="background:var(--err)" @click="doDelete">Confirm</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-backdrop {
  position: fixed; inset: 0; background: rgba(0, 0, 0, 0.6);
  display: flex; align-items: center; justify-content: center; padding: 1rem;
}
select { padding: 0.5rem; background: var(--panel-2); color: var(--fg); border: 1px solid var(--border); border-radius: 6px; }
</style>
