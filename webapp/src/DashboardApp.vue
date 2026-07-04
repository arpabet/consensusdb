<script setup>
import { ref, onMounted } from 'vue'
import { api, clearCredential, hasCredential } from './api.js'
import Login from './components/Login.vue'
import Dashboard from './components/Dashboard.vue'

// The read-only monitoring dashboard, served at /. It never mutates anything —
// all management lives in the admin console at /console.
const phase = ref('loading') // loading | setup | login | denied | app
const me = ref(null)

function gate(m) {
  me.value = m
  // The dashboard needs at least roles/cdb.viewer (records.get); the server
  // reports this as canRead.
  phase.value = m.canRead ? 'app' : 'denied'
}

async function boot() {
  try {
    const s = await api.setupStatus()
    if (s.needsSetup) {
      phase.value = 'setup'
      return
    }
  } catch (e) {
    /* fall through and try to read */
  }
  try {
    gate(await api.me())
  } catch (e) {
    phase.value = 'login'
  }
}

function onAuthed(m) {
  gate(m)
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
      <span class="sub">dashboard</span>
      <span style="margin-left:auto;display:flex;gap:0.75rem;align-items:center">
        <span v-if="me && me.principal" class="hint">{{ me.principal }}</span>
        <a href="/console" class="hint">admin console →</a>
        <a v-if="hasCredential()" class="hint" style="cursor:pointer" @click="signOut">sign out</a>
      </span>
    </header>

    <div v-if="phase === 'loading'" class="panel">Loading…</div>

    <div v-else-if="phase === 'setup'" class="panel" style="max-width:32rem">
      <h2>Cluster not set up yet</h2>
      <p>Create the first administrator in the <a href="/console">admin console</a>, then return here to monitor the cluster.</p>
    </div>

    <Login v-else-if="phase === 'login'" @authed="onAuthed" />

    <div v-else-if="phase === 'denied'" class="panel" style="max-width:32rem">
      <h2>Access denied</h2>
      <p>Your account can't view the dashboard. It needs at least the
        <code>roles/cdb.viewer</code> role. Ask an administrator to grant it in the
        <a href="/console">admin console → IAM</a>.</p>
    </div>

    <Dashboard v-else-if="phase === 'app'" />
  </div>
</template>
