<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'

// Service accounts (application tokens + mutual-TLS certificate identities) and
// groups. Human users are on the Users tab; role assignments on the IAM tab.
const accounts = ref([])
const groups = ref([])
const bindings = ref([])
const error = ref('')
const busy = ref(false)

const newSA = ref({ name: '' })
const newToken = ref(null) // { name, token } shown once
const certFor = ref(null) // service account whose certs are being managed
const newCert = ref('')
const newGroup = ref({ name: '', members: '' })
const confirmDelete = ref(null) // { kind, name }

// serviceAccount:name → [{role, scope}]
const accessBySA = computed(() => {
  const map = {}
  for (const b of bindings.value) {
    for (const m of b.members || []) {
      if (m.startsWith('serviceAccount:')) (map[m] || (map[m] = [])).push({ role: b.role, scope: b.scope })
    }
  }
  return map
})

function shortRole(r) { return r.replace(/^roles\/cdb\./, '') }
function scopeLabel(s) { if (!s) return 'db'; const [t, r] = s.split('/'); return r ? `${t}/${r}` : t }

async function refresh() {
  error.value = ''
  try {
    const [s, g, b] = await Promise.all([api.serviceAccounts(), api.groups(), api.bindings()])
    accounts.value = s.serviceAccounts || []
    groups.value = g.groups || []
    bindings.value = b.bindings || []
    if (certFor.value) certFor.value = accounts.value.find((a) => a.name === certFor.value.name) || null
  } catch (e) { error.value = e.message }
}

async function run(fn) {
  error.value = ''
  busy.value = true
  try { await fn(); await refresh() } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function createSA() {
  error.value = ''
  busy.value = true
  try {
    newToken.value = await api.createServiceAccount(newSA.value.name.trim())
    newSA.value = { name: '' }
    await refresh()
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

function addCert() {
  const id = newCert.value.trim()
  if (!id) return
  run(async () => { await api.addCert(certFor.value.name, id); newCert.value = '' })
}
function removeCert(id) { run(() => api.removeCert(certFor.value.name, id)) }

function setGroup() {
  const members = newGroup.value.members.split(/[\s,]+/).map((m) => m.trim()).filter(Boolean)
  run(async () => { await api.setGroup(newGroup.value.name.trim(), members); newGroup.value = { name: '', members: '' } })
}

function doDelete() {
  const d = confirmDelete.value
  run(() => (d.kind === 'sa' ? api.deleteServiceAccount(d.name) : api.deleteGroup(d.name)))
  confirmDelete.value = null
}

onMounted(refresh)
</script>

<template>
  <p v-if="error" class="err-text" style="margin-bottom:1rem">{{ error }}</p>

  <div v-if="newToken" class="panel" style="margin-bottom:1rem;border-color:var(--accent)">
    <h2>Token for “{{ newToken.name }}” — copy it now</h2>
    <p class="hint">Shown only once, not recoverable. Give it to the client app as its credential.</p>
    <input readonly :value="newToken.token" class="mono" style="width:100%" @focus="$event.target.select()" />
    <button style="margin-top:0.5rem" @click="newToken = null">Done</button>
  </div>

  <!-- Service accounts -->
  <div class="panel" style="margin-bottom:1rem">
    <h2>Service accounts</h2>
    <p class="hint">Workload identities for client apps — an API token and/or mutual-TLS certificate identities.</p>
    <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin:0.5rem 0">
      <thead>
        <tr style="color:var(--muted);text-align:left">
          <th style="padding:0.3rem 0">Name</th><th>Token</th><th>mTLS certs</th><th>Access</th><th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="s in accounts" :key="s.name" style="border-top:1px solid var(--border)">
          <td style="padding:0.45rem 0" class="mono">{{ s.name }}</td>
          <td><span :class="'badge ' + (s.hasToken ? 'ok' : '')">{{ s.hasToken ? 'set' : 'none' }}</span></td>
          <td><a style="cursor:pointer" @click="certFor = s; newCert = ''">{{ (s.certIdentities || []).length }} cert(s) →</a></td>
          <td>
            <span v-for="(a, i) in (accessBySA['serviceAccount:' + s.name] || [])" :key="i" class="badge"
              style="margin:0.1rem 0.2rem 0.1rem 0;background:var(--panel-2)">{{ shortRole(a.role) }}@{{ scopeLabel(a.scope) }}</span>
            <span v-if="!(accessBySA['serviceAccount:' + s.name] || []).length" class="hint">—</span>
          </td>
          <td style="text-align:right"><button style="background:var(--err);padding:0.25rem 0.5rem" @click="confirmDelete = { kind: 'sa', name: s.name }">Revoke</button></td>
        </tr>
        <tr v-if="!accounts.length"><td colspan="5" class="hint" style="padding:0.5rem 0">No service accounts.</td></tr>
      </tbody>
    </table>
    <div class="grid" style="align-items:end">
      <div><label>Name</label><input v-model="newSA.name" placeholder="my-app" /></div>
      <div><button :disabled="busy || !newSA.name" @click="createSA">Create &amp; mint token</button></div>
    </div>
    <p class="hint" style="margin-top:0.5rem">Grant this account a role / tenant on the <strong>IAM</strong> tab.</p>
  </div>

  <!-- Groups -->
  <div class="panel">
    <h2>Groups</h2>
    <p class="hint">A named set of members you can grant roles to at once (on the IAM tab).</p>
    <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin:0.5rem 0">
      <thead><tr style="color:var(--muted);text-align:left"><th style="padding:0.3rem 0">Group</th><th>Members</th><th></th></tr></thead>
      <tbody>
        <tr v-for="g in groups" :key="g.name" style="border-top:1px solid var(--border)">
          <td style="padding:0.45rem 0" class="mono">{{ g.name }}</td>
          <td>{{ (g.members || []).join(', ') || '—' }}</td>
          <td style="text-align:right"><button style="background:var(--err);padding:0.25rem 0.5rem" @click="confirmDelete = { kind: 'group', name: g.name }">Delete</button></td>
        </tr>
        <tr v-if="!groups.length"><td colspan="3" class="hint" style="padding:0.5rem 0">No groups.</td></tr>
      </tbody>
    </table>
    <div class="grid" style="align-items:end">
      <div><label>Group name</label><input v-model="newGroup.name" placeholder="accounting" /></div>
      <div><label>Members (user:… / serviceAccount:…, comma-separated)</label><input v-model="newGroup.members" placeholder="user:alice, user:bob" /></div>
      <div><button :disabled="busy || !newGroup.name" @click="setGroup">Save group</button></div>
    </div>
  </div>

  <!-- cert-identity modal -->
  <div v-if="certFor" class="modal-backdrop" @click.self="certFor = null">
    <div class="panel" style="max-width:32rem;width:100%">
      <h2>mTLS identities — {{ certFor.name }}</h2>
      <p class="hint">A client certificate whose SAN URI or CN matches one of these authenticates as this account.</p>
      <div v-for="id in (certFor.certIdentities || [])" :key="id" style="display:flex;align-items:center;gap:0.5rem;padding:0.2rem 0;border-top:1px solid var(--border)">
        <span class="mono" style="font-size:0.8rem;word-break:break-all">{{ id }}</span>
        <button style="background:var(--err);padding:0.1rem 0.45rem;font-size:0.75rem;margin-left:auto" @click="removeCert(id)">remove</button>
      </div>
      <p v-if="!(certFor.certIdentities || []).length" class="hint">No certificate identities.</p>
      <label style="margin-top:0.5rem">Add identity (SAN URI or CN)</label>
      <input v-model="newCert" placeholder="spiffe://cluster/my-app  or  CN=my-app" @keyup.enter="addCert" />
      <div style="display:flex;gap:0.5rem;justify-content:flex-end;margin-top:0.75rem">
        <button style="background:var(--panel-2)" @click="certFor = null">Close</button>
        <button :disabled="busy || !newCert.trim()" @click="addCert">Add</button>
      </div>
    </div>
  </div>

  <!-- delete confirmation -->
  <div v-if="confirmDelete" class="modal-backdrop" @click.self="confirmDelete = null">
    <div class="panel" style="max-width:24rem">
      <h2>{{ confirmDelete.kind === 'sa' ? 'Revoke service account' : 'Delete group' }}</h2>
      <p>Remove <strong>{{ confirmDelete.name }}</strong>?</p>
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
</style>
