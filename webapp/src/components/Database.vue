<script setup>
import { ref } from 'vue'
import { authHeader } from '../api.js'

// Database export / import (admin only). Export streams a download; import
// uploads a dump file. Both hit the authenticated /api endpoints directly so
// large files transfer without buffering in the app.
const exportPassword = ref('')
const importPassword = ref('')
const importFile = ref(null)
const busy = ref(false)
const message = ref('')
const error = ref('')

async function doExport() {
  error.value = ''; message.value = ''
  busy.value = true
  try {
    const url = '/api/database/export' + (exportPassword.value ? `?password=${encodeURIComponent(exportPassword.value)}` : '')
    const res = await fetch(url, { headers: { Authorization: authHeader() } })
    if (!res.ok) throw new Error((await res.json()).error || `HTTP ${res.status}`)
    const blob = await res.blob()
    const cd = res.headers.get('Content-Disposition') || ''
    const name = /filename="([^"]+)"/.exec(cd)?.[1] || 'consensusdb.dump'
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob); a.download = name; a.click()
    URL.revokeObjectURL(a.href)
    message.value = `Exported ${name}`
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function doImport() {
  error.value = ''; message.value = ''
  if (!importFile.value) { error.value = 'Choose a dump file.'; return }
  busy.value = true
  try {
    const url = '/api/database/import' + (importPassword.value ? `?password=${encodeURIComponent(importPassword.value)}` : '')
    const res = await fetch(url, { method: 'POST', headers: { Authorization: authHeader() }, body: importFile.value })
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
    message.value = 'Import complete.'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}
</script>

<template>
  <div class="grid">
    <div class="panel">
      <h2>Export database</h2>
      <p class="hint">Download a full dump. Optionally encrypt it with a password (argon2id + AES-256-GCM).</p>
      <label>Password (blank = plain dump)</label>
      <input v-model="exportPassword" type="password" />
      <button :disabled="busy" @click="doExport">Export &amp; download</button>
    </div>

    <div class="panel">
      <h2>Import from file</h2>
      <p class="hint">Load a dump into this node. Refused while replication is active — import into a fresh node.</p>
      <label>Dump file</label>
      <input type="file" @change="(e) => (importFile = e.target.files[0])" />
      <label>Password (if encrypted)</label>
      <input v-model="importPassword" type="password" />
      <button :disabled="busy || !importFile" @click="doImport">Import</button>
    </div>
  </div>
  <p v-if="message" class="hint" style="margin-top:0.75rem">{{ message }}</p>
  <p v-if="error" class="err-text">{{ error }}</p>
</template>
