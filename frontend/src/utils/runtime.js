export function isLocalhost(hostname) {
  if (!hostname) return false
  if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '0.0.0.0') {
    return true
  }
  return hostname.endsWith('.local')
}

export function getWebSocketHost() {
  if (typeof window === 'undefined') return ''

  const env = typeof import.meta !== 'undefined' && import.meta.env ? import.meta.env : {}
  const overrideHost = env.VITE_WS_HOST || ''
  const overridePort = env.VITE_WS_PORT || ''
  const hostname = window.location.hostname || ''
  const port = window.location.port || ''
  const isDev = typeof import.meta !== 'undefined' && import.meta.env && import.meta.env.DEV

  if (overrideHost) {
    if (overridePort && !overrideHost.includes(':')) {
      return `${overrideHost}:${overridePort}`
    }
    return overrideHost
  }

  if (isLocalhost(hostname)) {
    return window.location.host
  }

  if (isDev) {
    return window.location.host
  }

  if (!port || port === '80' || port === '443') {
    return window.location.host
  }

  return hostname
}
