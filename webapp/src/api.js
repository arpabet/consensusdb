// Thin client for the consensusdb admin REST API (/api). The credential is an
// IAM token (service account) or user:password, sent on every request; it is
// held only in memory for the session.

let auth = { header: '' }

export function setToken(token) {
  auth.header = token ? `Bearer ${token}` : ''
}

export function setBasic(user, pass) {
  auth.header = user ? `Basic ${btoa(`${user}:${pass}`)}` : ''
}

export function hasCredential() {
  return !!auth.header
}

async function req(method, path, body) {
  const headers = { 'Content-Type': 'application/json' }
  if (auth.header) headers['Authorization'] = auth.header
  const res = await fetch(`/api${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  const data = text ? JSON.parse(text) : {}
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`)
  }
  return data
}

export const api = {
  // onboarding (unauthenticated)
  setupStatus: () => req('GET', '/setup/status'),
  bootstrap: (username, password) => req('POST', '/setup/bootstrap', { username, password }),
  generateCA: () => req('POST', '/setup/ledger-ca'),
  // session
  me: () => req('GET', '/me'),
  cluster: () => req('GET', '/cluster'),
  stats: () => req('GET', '/stats'),
  regions: () => req('GET', '/regions'),
  ledgerStatus: () => req('GET', '/ledger/status'),
  startVerify: (payload) => req('POST', '/ledger/verify', payload),
  verifyJob: (id) => req('GET', `/ledger/verify/${id}`),
  // cluster node management
  nodes: () => req('GET', '/cluster/nodes'),
  addNode: (nodeId, address) => req('POST', '/cluster/nodes', { nodeId, address }),
  removeNode: (id) => req('DELETE', `/cluster/nodes/${encodeURIComponent(id)}`),
}

// exportUrl / importUrl build authenticated links for large file transfers that
// go directly through the browser (download / upload).
export function authHeader() {
  return auth.header
}
