<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'

// GCP-style IAM: every principal (user / service account / group) and the roles
// granted to it, each scoped to the whole database, a tenant (major key), or a
// tenant's region. Multiple assignments per principal are shown together.
const bindings = ref([])
const roleNames = ref([])
const catalog = ref([]) // known principals for the picker
const error = ref('')
const busy = ref(false)
const showGrant = ref(false)

const grantForm = ref({ member: '', role: '', scope: 'instance', tenant: '', region: '' })

// Invert bindings (scope, role, members) into one row per principal.
const principals = computed(() => {
  const map = {}
  for (const b of bindings.value) {
    for (const m of b.members || []) {
      ;(map[m] || (map[m] = [])).push({ role: b.role, scope: b.scope })
    }
  }
  return Object.keys(map)
    .sort()
    .map((member) => ({ member, roles: map[member].sort((a, b) => (a.role + a.scope).localeCompare(b.role + b.scope)) }))
})

function scopeLabel(scope) {
  if (!scope) return 'whole database'
  const [tenant, region] = scope.split('/')
  return region ? `tenant ${tenant} · region ${region}` : `tenant ${tenant}`
}

async function refresh() {
  error.value = ''
  try {
    const [b, r, u, s, g] = await Promise.all([api.bindings(), api.roles(), api.users(), api.serviceAccounts(), api.groups()])
    bindings.value = b.bindings || []
    roleNames.value = Object.keys(r.roles || {}).sort()
    if (!grantForm.value.role) grantForm.value.role = roleNames.value.find((n) => n.endsWith('viewer')) || roleNames.value[0] || ''
    catalog.value = [
      ...(u.users || []).map((x) => 'user:' + x.name),
      ...(s.serviceAccounts || []).map((x) => 'serviceAccount:' + x.name),
      ...(g.groups || []).map((x) => 'group:' + x.name),
    ]
  } catch (e) {
    error.value = e.message
  }
}

async function run(fn) {
  error.value = ''
  busy.value = true
  try { await fn(); await refresh() } catch (e) { error.value = e.message } finally { busy.value = false }
}

function scopeArgs(f) {
  if (f.scope === 'tenant') return [f.tenant.trim(), '']
  if (f.scope === 'region') return [f.tenant.trim(), f.region.trim()]
  return ['', '']
}

function grant() {
  const [tenant, region] = scopeArgs(grantForm.value)
  run(async () => {
    await api.grant(grantForm.value.role, [grantForm.value.member.trim()], tenant, region)
    showGrant.value = false
    grantForm.value = { member: '', role: grantForm.value.role, scope: 'instance', tenant: '', region: '' }
  })
}

function revoke(member, role, scope) {
  const [tenant, region] = (scope || '').split('/')
  run(() => api.revoke(role, [member], tenant || '', region || ''))
}

onMounted(refresh)
</script>

<template>
  <div class="panel">
    <div style="display:flex;align-items:center;margin-bottom:0.5rem">
      <h2 style="margin:0">IAM — access to this database</h2>
      <button style="margin-left:auto" @click="showGrant = !showGrant">+ Grant access</button>
    </div>
    <p class="hint">Roles granted to each principal, scoped to the whole database, a tenant (major key), or a region.</p>

    <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin-top:0.5rem">
      <thead>
        <tr style="color:var(--muted);text-align:left">
          <th style="padding:0.4rem 0;width:22rem">Principal</th><th>Role · scope</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in principals" :key="p.member" style="border-top:1px solid var(--border)">
          <td style="padding:0.5rem 0;vertical-align:top" class="mono">{{ p.member }}</td>
          <td style="padding:0.5rem 0">
            <div v-for="(a, i) in p.roles" :key="i" style="display:flex;align-items:center;gap:0.5rem;padding:0.15rem 0">
              <span class="mono" style="font-size:0.78rem">{{ a.role }}</span>
              <span class="hint">·</span>
              <span class="hint">{{ scopeLabel(a.scope) }}</span>
              <button style="background:var(--panel-2);padding:0.1rem 0.45rem;font-size:0.75rem;margin-left:auto"
                @click="revoke(p.member, a.role, a.scope)">revoke</button>
            </div>
          </td>
        </tr>
        <tr v-if="!principals.length"><td colspan="2" class="hint" style="padding:0.5rem 0">No access granted yet.</td></tr>
      </tbody>
    </table>
    <p v-if="error" class="err-text">{{ error }}</p>
  </div>

  <!-- grant dialog -->
  <div v-if="showGrant" class="modal-backdrop" @click.self="showGrant = false">
    <div class="panel" style="max-width:30rem;width:100%">
      <h2>Grant access</h2>
      <label>Principal</label>
      <input v-model="grantForm.member" list="principal-catalog" placeholder="user:alice · serviceAccount:my-app · group:team" />
      <datalist id="principal-catalog">
        <option v-for="c in catalog" :key="c" :value="c" />
      </datalist>
      <label>Role</label>
      <select v-model="grantForm.role" style="width:100%">
        <option v-for="n in roleNames" :key="n" :value="n">{{ n }}</option>
      </select>
      <label>Scope</label>
      <select v-model="grantForm.scope" style="width:100%">
        <option value="instance">Whole database</option>
        <option value="tenant">A tenant (major key)</option>
        <option value="region">A tenant's region</option>
      </select>
      <template v-if="grantForm.scope !== 'instance'">
        <label>Tenant (major key)</label>
        <input v-model="grantForm.tenant" placeholder="acme" />
      </template>
      <template v-if="grantForm.scope === 'region'">
        <label>Region</label>
        <input v-model="grantForm.region" placeholder="USERS" />
      </template>
      <div style="display:flex;gap:0.5rem;justify-content:flex-end;margin-top:0.75rem">
        <button style="background:var(--panel-2)" @click="showGrant = false">Cancel</button>
        <button :disabled="busy || !grantForm.member || !grantForm.role || (grantForm.scope !== 'instance' && !grantForm.tenant)" @click="grant">Grant</button>
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
