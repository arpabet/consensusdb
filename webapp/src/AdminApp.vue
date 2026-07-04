<script setup>
import { ref, onMounted } from 'vue'
import { api, clearCredential, hasCredential } from './api.js'
import Onboarding from './components/Onboarding.vue'
import Login from './components/Login.vue'
import IAM from './components/IAM.vue'
import Users from './components/Users.vue'
import Access from './components/Access.vue'
import Nodes from './components/Nodes.vue'
import Database from './components/Database.vue'
import VerifyBackup from './components/VerifyBackup.vue'

// The admin console, served at /console. Everything here mutates or manages the
// cluster, so it requires an admin sign-in. Read-only monitoring is at /dashboard.
const phase = ref('loading') // loading | onboarding | login | app
const me = ref(null)
const tab = ref('iam')

async function boot() {
  try {
    const s = await api.setupStatus()
    if (s.needsSetup) {
      phase.value = 'onboarding'
      return
    }
  } catch (e) {
    /* fall through to login */
  }
  if (hasCredential()) {
    try {
      me.value = await api.me()
      phase.value = me.value.isAdmin ? 'app' : 'login'
      return
    } catch (e) {
      /* fall through to login */
    }
  }
  phase.value = 'login'
}

function onboarded() {
  phase.value = 'login'
}
function onAuthed(m) {
  me.value = m
  phase.value = 'app'
}
function signOut() {
  clearCredential()
  me.value = null
  phase.value = 'login'
}

onMounted(boot)
</script>

<template>
  <div class="wrap">
    <header>
      <h1>ConsensusDB</h1>
      <span class="sub">admin console</span>
      <span style="margin-left:auto;display:flex;gap:0.75rem;align-items:center">
        <a href="/dashboard" class="hint">← dashboard</a>
        <template v-if="me">
          <span class="hint">{{ me.principal }} <span class="badge run">admin</span></span>
          <a class="hint" style="cursor:pointer" @click="signOut">sign out</a>
        </template>
      </span>
    </header>

    <div v-if="phase === 'loading'" class="panel">Loading…</div>

    <Onboarding v-else-if="phase === 'onboarding'" @done="onboarded" />

    <Login v-else-if="phase === 'login'" :require-admin="true" @authed="onAuthed" />

    <template v-else-if="phase === 'app'">
      <nav style="display:flex;gap:0.5rem;margin-bottom:1rem;flex-wrap:wrap">
        <button :style="tab === 'iam' ? '' : 'background:var(--panel-2)'" @click="tab = 'iam'">IAM</button>
        <button :style="tab === 'users' ? '' : 'background:var(--panel-2)'" @click="tab = 'users'">Users</button>
        <button :style="tab === 'access' ? '' : 'background:var(--panel-2)'" @click="tab = 'access'">Access</button>
        <button :style="tab === 'nodes' ? '' : 'background:var(--panel-2)'" @click="tab = 'nodes'">Nodes</button>
        <button :style="tab === 'database' ? '' : 'background:var(--panel-2)'" @click="tab = 'database'">Database</button>
        <button :style="tab === 'verify' ? '' : 'background:var(--panel-2)'" @click="tab = 'verify'">Verify ledger</button>
      </nav>

      <IAM v-if="tab === 'iam'" />
      <Users v-else-if="tab === 'users'" />
      <Access v-else-if="tab === 'access'" />
      <Nodes v-else-if="tab === 'nodes'" />
      <Database v-else-if="tab === 'database'" />
      <div v-else-if="tab === 'verify'" class="panel">
        <h2>Verify a backup against a quorum certificate</h2>
        <VerifyBackup />
      </div>
    </template>
  </div>
</template>
