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
  const isDev = typeof import.meta !== 'undefined' && import.meta.env && import.meta.env.DEV

  // 1. Explicit override via environment variable
  if (overrideHost) {
    if (overridePort && !overrideHost.includes(':')) {
      return `${overrideHost}:${overridePort}`
    }
    return overrideHost
  }

  // 2. Localhost or dev mode: include port for local testing
  if (isLocalhost(hostname) || isDev) {
    return window.location.host // includes port if present
  }

  // 3. Production: standard ports (80/443) or no port
  //    Use just hostname - browser will use default port for wss:// (443) or ws:// (80)
  if (!port || port === '80' || port === '443') {
    return hostname // just hostname, no port
  }

  // 4. Non-standard port in production (edge case): include port
  return window.location.host
}
