// Thin client for the consensusdb admin REST API (/api). The credential is an
// IAM token (service account) or user:password, sent on every request; it is
// held only in memory for the session. Shared by the read-only dashboard (at /)
// and the admin console (at /console).

let auth = { header: '' }

export function setToken(token) {
  auth.header = token ? `Bearer ${token}` : ''
}

export function setBasic(user, pass) {
  auth.header = user ? `Basic ${btoa(`${user}:${pass}`)}` : ''
}

export function clearCredential() {
  auth.header = ''
}

export function hasCredential() {
  return !!auth.header
}

export function authHeader() {
  return auth.header
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
  // session + read-only monitoring
  me: () => req('GET', '/me'),
  cluster: () => req('GET', '/cluster'),
  stats: () => req('GET', '/stats'),
  regions: () => req('GET', '/regions'),
  ledgerStatus: () => req('GET', '/ledger/status'),
  startVerify: (payload) => req('POST', '/ledger/verify', payload),
  verifyJob: (id) => req('GET', `/ledger/verify/${id}`),
  // cluster node management (admin)
  nodes: () => req('GET', '/cluster/nodes'),
  addNode: (nodeId, address) => req('POST', '/cluster/nodes', { nodeId, address }),
  removeNode: (id) => req('DELETE', `/cluster/nodes/${encodeURIComponent(id)}`),
  // IAM management (admin)
  users: () => req('GET', '/iam/users'),
  createUser: (username, password) => req('POST', '/iam/users', { username, password }),
  deleteUser: (name) => req('DELETE', `/iam/users/${encodeURIComponent(name)}`),
  serviceAccounts: () => req('GET', '/iam/service-accounts'),
  createServiceAccount: (name) => req('POST', '/iam/service-accounts', { name }),
  deleteServiceAccount: (name) => req('DELETE', `/iam/service-accounts/${encodeURIComponent(name)}`),
  // client certificates (mTLS) — one CA, keyed by principal (user: or serviceAccount:)
  certs: (principal) => req('GET', `/iam/certs?principal=${encodeURIComponent(principal)}`),
  issueCert: (principal, ttlDays) => req('POST', '/iam/certs/issue', { principal, ttlDays }),
  registerCert: (principal, identity) => req('POST', '/iam/certs/register', { principal, identity }),
  revokeCert: (identity) => req('DELETE', `/iam/certs?identity=${encodeURIComponent(identity)}`),
  roles: () => req('GET', '/iam/roles'),
  bindings: () => req('GET', '/iam/bindings'),
  grant: (role, members, tenant, region) => req('POST', '/iam/bindings', { role, members, tenant, region }),
  revoke: (role, members, tenant, region) => req('POST', '/iam/bindings/revoke', { role, members, tenant, region }),
  groups: () => req('GET', '/iam/groups'),
  setGroup: (name, members) => req('POST', '/iam/groups', { name, members }),
  deleteGroup: (name) => req('DELETE', `/iam/groups/${encodeURIComponent(name)}`),
}
