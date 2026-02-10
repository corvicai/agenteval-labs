export function isLocalhost(hostname) {
  if (!hostname) return false
  if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '0.0.0.0') {
    return true
  }
  return hostname.endsWith('.local')
}

import { config } from '../config'

function parseHostFromUrl(rawUrl) {
  const value = (rawUrl || '').trim()
  if (!value) return ''

  try {
    return new URL(value).host || ''
  } catch {
    return ''
  }
}

function normalizeHost(rawHost) {
  const value = (rawHost || '').trim()
  if (!value) return ''
  return value.replace(/^[a-z]+:\/\//i, '').split('/')[0]
}

/**
 * Determine the WebSocket host for connecting to /ws
 * 
 * Production (HTTPS/443 or HTTP/80): just hostname → wss://domain.com/ws
 * Development (localhost or non-standard port): include port → ws://localhost:3010/ws
 */
export function getWebSocketHost() {
  if (typeof window === 'undefined') return ''

  const overrideHost = config.WS_HOST || ''
  const overridePort = config.WS_PORT || ''
  const hostname = window.location.hostname || ''
  const port = window.location.port || ''
  const host = window.location.host || ''
  const isDev = config.DEV

  // 1. Explicit full URL override (highest priority)
  const explicitWsUrl = config.WS_URL || ''
  if (explicitWsUrl) {
    const urlHost = parseHostFromUrl(explicitWsUrl)
    if (urlHost) return urlHost

    const literalHost = normalizeHost(explicitWsUrl)
    if (literalHost) return literalHost
  }

  // 2. Explicit host override
  if (overrideHost) {
    if (overridePort && !overrideHost.includes(':')) {
      return `${overrideHost}:${overridePort}`
    }
    return overrideHost
  }

  // 3. Force localhost for local development (when accessing via localhost or 127.0.0.1)
  if (isLocalhost(hostname)) {
    return host // Returns "localhost:3010" or "127.0.0.1:3010"
  }

  // 4. In dev mode, always prefer localhost if available
  if (isDev && (hostname.includes('localhost') || hostname === '127.0.0.1')) {
    return port ? `localhost:${port}` : 'localhost'
  }

  // 5. For standard ports, use hostname only.
  if (!port || port === '80' || port === '443') {
    return hostname
  }

  // 6. Fallback: use the current host (includes port)
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
