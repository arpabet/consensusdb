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
  cluster: () => req('GET', '/cluster'),
  ledgerStatus: () => req('GET', '/ledger/status'),
  startVerify: (payload) => req('POST', '/ledger/verify', payload),
  verifyJob: (id) => req('GET', `/ledger/verify/${id}`),
}
