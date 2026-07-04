<script setup>
import { ref } from 'vue'
import { api, setToken, setBasic, clearCredential } from '../api.js'

// Shared sign-in for both apps. Accepts a username + password (HTTP Basic) or an
// IAM token, so a password admin created during onboarding can actually sign in.
// requireAdmin gates the admin console; the read-only dashboard leaves it false.
const props = defineProps({ requireAdmin: { type: Boolean, default: false } })
const emit = defineEmits(['authed'])

const method = ref('password') // password | token
const user = ref('')
const pass = ref('')
const token = ref('')
const error = ref('')
const busy = ref(false)

async function signIn() {
  error.value = ''
  busy.value = true
  try {
    if (method.value === 'password') {
      setBasic(user.value.trim(), pass.value)
    } else {
      setToken(token.value.trim())
    }
    const me = await api.me()
    if (props.requireAdmin && !me.isAdmin) {
      clearCredential()
      error.value = 'This account is not an administrator. Use the read-only dashboard, or sign in as an admin.'
      return
    }
    emit('authed', me)
  } catch (e) {
    clearCredential()
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="panel" style="max-width:28rem">
    <h2>Sign in</h2>
    <div style="display:flex;gap:0.5rem;margin-bottom:0.75rem">
      <button :style="method === 'password' ? '' : 'background:var(--panel-2)'" @click="method = 'password'">Username &amp; password</button>
      <button :style="method === 'token' ? '' : 'background:var(--panel-2)'" @click="method = 'token'">Token</button>
    </div>

    <template v-if="method === 'password'">
      <label>Username</label>
      <input v-model="user" placeholder="admin" @keyup.enter="signIn" />
      <label>Password</label>
      <input v-model="pass" type="password" @keyup.enter="signIn" />
    </template>
    <template v-else>
      <label>IAM token</label>
      <input v-model="token" type="password" placeholder="serviceAccount.xxxxx" @keyup.enter="signIn" />
      <p class="hint">A service-account token (<code>consensusdb iam sa-add</code> or the admin console's Access page).</p>
    </template>

    <button :disabled="busy" @click="signIn">Sign in</button>
    <p v-if="error" class="err-text">{{ error }}</p>
  </div>
</template>
