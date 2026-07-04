<script setup>
import { ref, onMounted } from 'vue'
import { api, setToken, hasCredential } from './api.js'
import Onboarding from './components/Onboarding.vue'
import Dashboard from './components/Dashboard.vue'
import Nodes from './components/Nodes.vue'
import Database from './components/Database.vue'
import VerifyBackup from './components/VerifyBackup.vue'

const phase = ref('loading') // loading | onboarding | login | app
const me = ref(null)
const token = ref('')
const error = ref('')
const tab = ref('dashboard')

async function boot() {
  try {
    const s = await api.setupStatus()
    if (s.needsSetup) { phase.value = 'onboarding'; return }
    if (hasCredential()) {
      me.value = await api.me()
      phase.value = 'app'
    } else {
      phase.value = 'login'
    }
  } catch (e) {
    phase.value = 'login'
  }
}

async function connect() {
  error.value = ''
  setToken(token.value.trim())
  try {
    me.value = await api.me()
    phase.value = 'app'
  } catch (e) {
    error.value = e.message
  }
}

function onboarded() {
  phase.value = 'login'
}

onMounted(boot)
</script>

<template>
  <div class="wrap">
    <header>
      <h1>ConsensusDB</h1>
      <span class="sub">admin console</span>
      <span v-if="me" style="margin-left:auto;color:var(--muted);font-size:0.85rem">
        {{ me.principal }} <span v-if="me.isAdmin" class="badge run">admin</span>
      </span>
    </header>

    <Onboarding v-if="phase === 'onboarding'" @done="onboarded" />

    <div v-else-if="phase === 'login'" class="panel" style="max-width:28rem">
      <h2>Sign in</h2>
      <label>IAM token</label>
      <input v-model="token" type="password" placeholder="serviceAccount.xxxxx" @keyup.enter="connect" />
      <p class="hint">A token with <code>cdb.proofs.read</code> (auditor) for read access; an admin token unlocks export/import.</p>
      <button @click="connect">Sign in</button>
      <p v-if="error" class="err-text">{{ error }}</p>
    </div>

    <template v-else-if="phase === 'app'">
      <nav style="display:flex;gap:0.5rem;margin-bottom:1rem">
        <button :style="tab === 'dashboard' ? '' : 'background:var(--panel-2)'" @click="tab = 'dashboard'">Dashboard</button>
        <!-- Admin-only tabs: shown only for admins -->
        <button v-if="me.isAdmin" :style="tab === 'nodes' ? '' : 'background:var(--panel-2)'" @click="tab = 'nodes'">Nodes</button>
        <button :style="tab === 'ledger' ? '' : 'background:var(--panel-2)'" @click="tab = 'ledger'">Verify ledger</button>
        <button v-if="me.isAdmin" :style="tab === 'database' ? '' : 'background:var(--panel-2)'" @click="tab = 'database'">Database</button>
      </nav>

      <Dashboard v-if="tab === 'dashboard'" />

      <Nodes v-else-if="tab === 'nodes' && me.isAdmin" />

      <div v-else-if="tab === 'ledger'" class="panel">
        <h2>Verify a backup against a quorum certificate</h2>
        <VerifyBackup />
      </div>

      <Database v-else-if="tab === 'database' && me.isAdmin" />
    </template>
  </div>
</template>
