export function isLocalhost(hostname) {
  if (!hostname) return false
  if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '0.0.0.0') {
    return true
  }
  return hostname.endsWith('.local')
}

/**
 * Determine the WebSocket host for connecting to /ws
 * 
 * Production (HTTPS/443 or HTTP/80): just hostname → wss://domain.com/ws
 * Development (localhost or non-standard port): include port → ws://localhost:3010/ws
 */
export function getWebSocketHost() {
  if (typeof window === 'undefined') return ''

  const env = typeof import.meta !== 'undefined' && import.meta.env ? import.meta.env : {}
  const overrideHost = env.VITE_WS_HOST || ''
  const overridePort = env.VITE_WS_PORT || ''
  const hostname = window.location.hostname || ''
  const port = window.location.port || ''
  const host = window.location.host || ''
  const isDev = typeof import.meta !== 'undefined' && import.meta.env && import.meta.env.DEV

  // 1. Explicit override via environment variable (highest priority)
  if (overrideHost) {
    if (overridePort && !overrideHost.includes(':')) {
      return `${overrideHost}:${overridePort}`
    }
    return overrideHost
  }

  // 2. Force localhost for local development (when accessing via localhost or 127.0.0.1)
  if (isLocalhost(hostname)) {
    return host // Returns "localhost:3010" or "127.0.0.1:3010"
  }

  // 3. In dev mode, always prefer localhost if available
  if (isDev && (hostname.includes('localhost') || hostname === '127.0.0.1')) {
    return port ? `localhost:${port}` : 'localhost'
  }

  // 4. Special case: If we are on a subdomain but the port is 3010 or 5173, 
  // it might be a tunnel or specific dev setup. 
  // BUT the user reported "pending" on 5173 when accessing a public URL.
  // If the port is standard (80/443), we definitely don't want to include it.
  if (!port || port === '80' || port === '443') {
    return hostname
  }

  // 5. Fallback: use the current host (includes port)
  return host
}

/**
 * Generate a UUID v4
 * Falls back to Math.random if crypto.randomUUID is not available (insecure contexts/IP)
 */
export function generateUUID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }

  // Fallback for insecure contexts (HTTP/IP)
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = Math.random() * 16 | 0
    const v = c === 'x' ? r : (r & 0x3 | 0x8)
    return v.toString(16)
  })
}
