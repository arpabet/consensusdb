<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'

// GCP-style IAM: every principal (user / service account / group) and the roles
// granted to it. A binding's scope is the whole database (all tenants & regions),
// a tenant (major key, all its regions), or a specific region within a tenant.
// Admin-ness is simply the roles/cdb.admin role bound at the whole-database scope
// (the first "root" user gets it at bootstrap) — there is no separate flag.
const users = ref([])
const serviceAccounts = ref([])
const groups = ref([])
const bindings = ref([])
const roleNames = ref([])
const error = ref('')
const busy = ref(false)

const editor = ref(null) // { member } while the role editor is open
const addForm = ref({ role: '', scope: 'instance', tenant: '', region: '' })

// Every principal from the catalog (shown even with no roles) merged with the
// roles granted to it by bindings.
const principals = computed(() => {
  const map = {}
  const ensure = (member, type) => (map[member] ||= { member, type, roles: [] })
  for (const u of users.value) ensure('user:' + u.name, 'user')
  for (const s of serviceAccounts.value) ensure('serviceAccount:' + s.name, 'serviceAccount')
  for (const g of groups.value) ensure('group:' + g.name, 'group')
  for (const b of bindings.value) {
    for (const m of b.members || []) {
      ensure(m, m.split(':')[0] || 'principal').roles.push({ role: b.role, scope: b.scope })
    }
  }
  return Object.values(map).sort((a, b) => a.member.localeCompare(b.member))
})

// The roles of the principal currently being edited (kept in sync after changes).
const editorRoles = computed(() => {
  const p = principals.value.find((x) => x.member === editor.value?.member)
  return p ? p.roles : []
})

function scopeLabel(scope) {
  if (!scope) return 'all tenants & regions'
  const [tenant, region] = scope.split('/')
  return region ? `tenant ${tenant} · region ${region}` : `tenant ${tenant} (all regions)`
}
function typeBadge(t) { return t === 'user' ? 'user' : t === 'serviceAccount' ? 'svc-acct' : t === 'group' ? 'group' : t }

async function refresh() {
  error.value = ''
  try {
    const [u, s, g, b, r] = await Promise.all([api.users(), api.serviceAccounts(), api.groups(), api.bindings(), api.roles()])
    users.value = u.users || []
    serviceAccounts.value = s.serviceAccounts || []
    groups.value = g.groups || []
    bindings.value = b.bindings || []
    roleNames.value = Object.keys(r.roles || {}).sort()
    if (!addForm.value.role) addForm.value.role = roleNames.value.find((n) => n.endsWith('viewer')) || roleNames.value[0] || ''
  } catch (e) {
    error.value = e.message
  }
}

async function run(fn) {
  error.value = ''
  busy.value = true
  try { await fn(); await refresh() } catch (e) { error.value = e.message } finally { busy.value = false }
}

function openEdit(p) {
  editor.value = { member: p.member }
  addForm.value = { role: addForm.value.role, scope: 'instance', tenant: '', region: '' }
}

function scopeArgs(f) {
  if (f.scope === 'tenant') return [f.tenant.trim(), '']
  if (f.scope === 'region') return [f.tenant.trim(), f.region.trim()]
  return ['', '']
}
function addRole() {
  const [tenant, region] = scopeArgs(addForm.value)
  run(() => api.grant(addForm.value.role, [editor.value.member], tenant, region))
}
function removeRole(role, scope) {
  const [tenant, region] = (scope || '').split('/')
  run(() => api.revoke(role, [editor.value.member], tenant || '', region || ''))
}

onMounted(refresh)
</script>

<template>
  <div class="panel">
    <h2 style="margin-top:0">IAM — access to this database</h2>
    <p class="hint">Every principal and the roles granted to it. Use <strong>Edit</strong> to add or remove roles
      (requires <code>roles/cdb.admin</code>).</p>

    <table style="width:100%;border-collapse:collapse;font-size:0.85rem;margin-top:0.5rem">
      <thead>
        <tr style="color:var(--muted);text-align:left">
          <th style="padding:0.4rem 0;width:20rem">Principal</th><th>Roles</th><th style="width:5rem"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in principals" :key="p.member" style="border-top:1px solid var(--border)">
          <td style="padding:0.5rem 0;vertical-align:top">
            <span class="mono">{{ p.member }}</span>
            <span class="badge" style="margin-left:0.4rem;background:var(--panel-2);font-size:0.7rem">{{ typeBadge(p.type) }}</span>
          </td>
          <td style="padding:0.5rem 0">
            <div v-for="(a, i) in p.roles" :key="i" style="padding:0.15rem 0">
              <span class="mono" style="font-size:0.8rem">{{ a.role }}</span>
              <span class="hint"> · {{ scopeLabel(a.scope) }}</span>
            </div>
            <span v-if="!p.roles.length" class="hint">— no roles</span>
          </td>
          <td style="padding:0.5rem 0;text-align:right;vertical-align:top">
            <button style="background:var(--panel-2);padding:0.25rem 0.6rem" @click="openEdit(p)">Edit</button>
          </td>
        </tr>
        <tr v-if="!principals.length"><td colspan="3" class="hint" style="padding:0.5rem 0">No principals yet.</td></tr>
      </tbody>
    </table>
    <p v-if="error" class="err-text">{{ error }}</p>
  </div>

  <!-- per-principal role editor -->
  <div v-if="editor" class="modal-backdrop" @click.self="editor = null">
    <div class="panel" style="max-width:32rem;width:100%">
      <h2>Roles — <span class="mono">{{ editor.member }}</span></h2>

      <div v-if="!editorRoles.length" class="hint" style="padding:0.3rem 0">No roles yet.</div>
      <div v-for="(a, i) in editorRoles" :key="i" style="display:flex;align-items:center;gap:0.5rem;padding:0.25rem 0;border-top:1px solid var(--border)">
        <span class="mono" style="font-size:0.8rem">{{ a.role }}</span>
        <span class="hint">· {{ scopeLabel(a.scope) }}</span>
        <button style="background:var(--err);padding:0.1rem 0.45rem;font-size:0.75rem;margin-left:auto" :disabled="busy" @click="removeRole(a.role, a.scope)">remove</button>
      </div>

      <h3 style="margin:0.9rem 0 0.3rem">Add a role</h3>
      <label>Role</label>
      <select v-model="addForm.role" style="width:100%">
        <option v-for="n in roleNames" :key="n" :value="n">{{ n }}</option>
      </select>
      <label>Scope</label>
      <select v-model="addForm.scope" style="width:100%">
        <option value="instance">Whole database — all tenants &amp; regions</option>
        <option value="tenant">A tenant (major key) — all its regions</option>
        <option value="region">A specific region within a tenant</option>
      </select>
      <template v-if="addForm.scope !== 'instance'">
        <label>Tenant (major key)</label>
        <input v-model="addForm.tenant" placeholder="acme" />
      </template>
      <template v-if="addForm.scope === 'region'">
        <label>Region</label>
        <input v-model="addForm.region" placeholder="USERS" />
      </template>
      <button :disabled="busy || !addForm.role || (addForm.scope !== 'instance' && !addForm.tenant)" @click="addRole">Add role</button>

      <div style="display:flex;justify-content:flex-end;margin-top:0.75rem">
        <button style="background:var(--panel-2)" @click="editor = null">Close</button>
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
