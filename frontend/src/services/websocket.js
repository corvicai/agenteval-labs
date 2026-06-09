// WebSocket connection manager for real-time updates
import { getWebSocketHost, generateUUID } from '../utils/runtime.js'

class WebSocketService {
    constructor() {
        this.ws = null
        this.workspaceId = null
        this.listeners = new Map()
        this.pendingRequests = new Map() // correlationId -> { resolve, reject, timeout }
        this.reconnectAttempts = 0
        this.connectionPromise = null
        this.shouldReconnect = true
        this.suppressNextReconnect = false
        this._iapTokenPromise = null
        this.lastDisconnectReason = null
        // Cursor for REQ_GET_MISSED_EVENTS. Stamped by the server on every
        // broadcast event; kept in memory only (not persisted) so a fresh
        // tab always falls back to REQ_SYNC_STATE.
        this.lastEventId = null

        // Client-side heartbeat (Plan 25B). Sends REQ_PING every 30 s and
        // considers the connection zombie if no DATA_PONG arrives within 10 s.
        this._heartbeatInterval = null
        this._heartbeatTimeout = null

        // Plan 24, Layer 4 — pending requests parked during a transient
        // (unintentional) network drop. Keyed by correlationId; value is
        // { resolve, reject, parkedAt }. Drained on reconnect via
        // drainPendingOnReconnect(). NOT populated for intentional
        // disconnects (AFK, logout, workspace-switch) which call
        // _rejectPendingRequests() directly through disconnect().
        this.pendingOnReconnect = new Map()

        // Visibility-aware reconnect (Plan 25C): re-verify health on tab focus.
        this._onVisibilityChange = this._handleVisibilityChange.bind(this)
        if (typeof document !== 'undefined') {
            document.addEventListener('visibilitychange', this._onVisibilityChange)
        }
    }

    static REQUEST_TIMEOUTS = {
        DEFAULT: 10000,
        START_RUN: 120000,
        RUN_EVALUATORS: 120000,
        RERUN_TASK: 45000,
        RUN_DETAILS: 30000,
        RUN_LITE: 30000,
        LATEST_RUN_BY_QS: 30000,
        UPDATE_AGENT: 30000
    }

    async _probeWebSocketHttpEndpoint(wsUrl) {
        const probeUrl = wsUrl.replace(/^wss:/, 'https:').replace(/^ws:/, 'http:')
        const controller = new AbortController()
        const timeout = setTimeout(() => controller.abort(), 8000)

        try {
            const response = await fetch(probeUrl, {
                method: 'GET',
                credentials: 'include',
                redirect: 'manual',
                cache: 'no-store',
                headers: {
                    'X-Requested-With': 'XMLHttpRequest'
                },
                signal: controller.signal
            })

            const contentType = response.headers?.get?.('content-type') || ''
            const location = response.headers?.get?.('location') || ''
            let bodyPreview = ''

            if (contentType.includes('text/plain') || contentType.includes('text/html') || contentType.includes('application/json')) {
                bodyPreview = (await response.text()).trim().slice(0, 120)
            }

            console.warn('[WS] HTTP probe result', {
                url: probeUrl,
                status: response.status,
                statusText: response.statusText,
                contentType,
                location,
                bodyPreview
            })
        } catch (e) {
            console.warn('[WS] HTTP probe failed', { url: probeUrl, error: e?.message || e })
        } finally {
            clearTimeout(timeout)
        }
    }

    async _fetchIAPToken() {
        // Return cached promise if already fetching or fetched
        if (this._iapTokenPromise) return this._iapTokenPromise

        this._iapTokenPromise = (async () => {
            const hostname = window.location.hostname

            // Only attempt on the specific dev domain or any non-local domain
            const isLocal = hostname === 'localhost' || hostname === '127.0.0.1' || hostname.endsWith('.local')
            const isIAPDomain = hostname === 'agenteval-dev.corviclabs.ai' || hostname.includes('corviclabs.ai')

            if (isLocal && !isIAPDomain) {
                return null
            }

            try {
                // Special GCP IAP endpoint for identity tokens
                // Documentation: https://cloud.google.com/iap/docs/sessions-howto#websockets
                const response = await fetch('/_gcp_iap/identityToken', {
                    credentials: 'include',
                    redirect: 'error' // IAP token should NOT be a redirect
                })

                if (!response.ok) {
                    if (response.status === 404) {
                        return null
                    }
                    console.warn('[WS] Failed to fetch IAP token:', response.status, response.statusText)
                    return null
                }

                const contentType = response.headers && typeof response.headers.get === 'function'
                    ? (response.headers.get('content-type') || '')
                    : ''
                const token = await response.text()
                const trimmedToken = token.trim()

                // Validate the token looks like a JWT (header.payload.signature)
                // If it's HTML (starting with <! or <html), it's invalid.
                if (contentType.includes('text/html') || trimmedToken.startsWith('<!') || trimmedToken.startsWith('<html') || trimmedToken.split('.').length !== 3) {
                    console.warn('[WS] IAP Token fetch returned HTML/Invalid content. Type:', contentType, 'Start:', trimmedToken.substring(0, 30))
                    return null
                }

                console.log('[WS] IAP identity token retrieved and validated')
                return trimmedToken
            } catch (e) {
                console.warn('[WS] Error fetching IAP token:', e)
                return null
            }
        })()

        return this._iapTokenPromise
    }

    isConnected() {
        return this.ws && this.ws.readyState === WebSocket.OPEN
    }

    // Connect anonymously (for login page)
    connectAnonymous() {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            return Promise.resolve() // Already connected
        }

        this.shouldReconnect = true
        this.workspaceId = null
        this.token = ''
        return this._establishConnection()
    }

    connect(workspaceId, token = null, options = {}) {
        this.shouldReconnect = true
        const skipStoredToken = options?.skipStoredToken === true
        const storedUser = localStorage.getItem('user')
        let userIsImpersonating = false
        if (storedUser) {
            try {
                userIsImpersonating = !!JSON.parse(storedUser)?.impersonator_id
            } catch (e) {
                userIsImpersonating = false
            }
        }
        const impersonationFlag = localStorage.getItem('is_impersonating') === '1' || userIsImpersonating
        const legacyToken = localStorage.getItem('token') || ''
        const storedImpersonationToken = localStorage.getItem('impersonation_token') || legacyToken
        let newToken = token || ''

        if (!newToken && impersonationFlag && storedImpersonationToken) {
            newToken = storedImpersonationToken
            if (!localStorage.getItem('impersonation_token') && legacyToken) {
                localStorage.setItem('impersonation_token', legacyToken)
            }
        } else if (!newToken && legacyToken && !skipStoredToken) {
            // Keep legacy token for Authorization header fallback in api.js
            newToken = legacyToken
        }

        // If we are already connected with the SAME workspace and the SAME token, return
        if (this.ws &&
            this.workspaceId === workspaceId &&
            this.token === newToken &&
            this.ws.readyState === WebSocket.OPEN) {
            return Promise.resolve()
        }

        // If we have an existing connection but the token or workspace changed, close it
        if (this.ws) {
            console.log('[WS] Closing existing connection to re-authenticate or switch workspace')
            this._rejectPendingRequests('WebSocket switching')
            this.suppressNextReconnect = true
            this.ws.close()
            this.ws = null
            this.connectionPromise = null
        }

        // Workspace or token switch invalidates the replay cursor — events
        // from the previous session don't apply here.
        if (this.workspaceId && this.workspaceId !== workspaceId) {
            this.lastEventId = null
        }

        this.workspaceId = workspaceId
        this.token = newToken
        this.reconnectAttempts = 0
        return this._establishConnection()
    }

    _establishConnection() {
        // If there's an existing connection promise that is still pending or open, return it
        if (this.connectionPromise && this.ws && (this.ws.readyState === WebSocket.CONNECTING || this.ws.readyState === WebSocket.OPEN)) {
            return this.connectionPromise
        }

        this.connectionPromise = new Promise((resolve, reject) => {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
            const wsHost = getWebSocketHost()
            let wsUrl = `${protocol}//${wsHost}/ws`

            // Add params if available
            // Add params if available
            const params = []
            if (this.workspaceId) params.push(`workspace_id=${this.workspaceId}`)
            // Only add token if explicitly provided (e.g. workspace switch). 
            // Otherwise rely on cookie.
            if (this.token) params.push(`token=${this.token}`)
            if (params.length > 0) wsUrl += '?' + params.join('&')

            console.log('[WS] Connecting to', wsUrl)

            // Fetch IAP token if needed (Google Cloud recommendation)
            this._fetchIAPToken().then(iapToken => {
                const subprotocols = iapToken ? ['iap-bearer-token', iapToken] : undefined
                let retriedWithoutSubprotocol = false

                const openSocket = (protocolList) => {
                    const ws = new WebSocket(wsUrl, protocolList)
                    this.ws = ws

                    ws.onopen = async () => {
                        if (this.ws !== ws) return
                        console.log('[WS] Connected')
                        this.reconnectAttempts = 0

                        // If we have a firebase token, authenticate immediately
                        if (this.firebaseToken) {
                            try {
                                const result = await this.request('AUTH', { token: this.firebaseToken })
                                console.log('[WS] Firebase Authentication successful')
                                this._emit('authenticated', result)
                            } catch (e) {
                                console.error('[WS] Firebase Authentication failed:', e)
                                this._emit('auth_failed', { error: e.message })
                                // If auth fails, we might want to stay connected but limited, or close.
                                // For now we just stay connected and let the UI handle the error.
                            }
                        }

                        this._startClientHeartbeat()
                        this._emit('connected', {})
                        resolve()
                    }

                    ws.onerror = (e) => {
                        if (this.ws !== ws) return

                        // Some edge proxies/IAP setups can reject unknown subprotocol negotiation.
                        // Retry once without subprotocol before surfacing the failure.
                        if (!retriedWithoutSubprotocol && protocolList && protocolList.length > 0) {
                            retriedWithoutSubprotocol = true
                            console.warn('[WS] IAP subprotocol handshake failed; retrying without subprotocol')
                            this.ws = null
                            try { ws.close() } catch (_) { }
                            openSocket(undefined)
                            return
                        }

                        console.error('[WS] Connection error:', {
                            eventType: e?.type,
                            readyState: ws.readyState,
                            url: ws.url,
                            protocols: protocolList || []
                        })
                        this._probeWebSocketHttpEndpoint(wsUrl)
                        this._emit('error', { error: e })
                        reject(e)
                    }

                    ws.onmessage = (event) => {
                        if (this.ws !== ws) return
                        this._handleMessage(event)
                    }

                    ws.onclose = (closeEvent) => {
                        if (this.ws !== ws) return
                        this._stopClientHeartbeat()
                        const reconnectPlanned = !this.suppressNextReconnect && this.shouldReconnect
                        console.log('[WS] Disconnected', {
                            code: closeEvent?.code,
                            reason: closeEvent?.reason || '',
                            wasClean: closeEvent?.wasClean,
                            reconnectPlanned
                        })
                        this._emit('disconnected', {
                            code: closeEvent?.code,
                            reason: closeEvent?.reason || '',
                            wasClean: closeEvent?.wasClean,
                            reconnectPlanned,
                            disconnectReason: this.lastDisconnectReason
                        })
                        this.lastDisconnectReason = null
                        // Park (not reject) pending requests for unintentional network
                        // drops so drainPendingOnReconnect() can recover server
                        // responses from the short-lived cache (Plan 24, Layer 4).
                        // Intentional disconnects (AFK, logout, workspace-switch) call
                        // _rejectPendingRequests() from disconnect() before ws.close(),
                        // so pendingRequests is already empty here in those cases.
                        this._rejectPendingRequests('WebSocket disconnected', reconnectPlanned)
                        if (this.suppressNextReconnect) {
                            this.suppressNextReconnect = false
                            return
                        }
                        if (!this.shouldReconnect) return
                        this._attemptReconnect()
                    }
                }

                openSocket(subprotocols)
            }).catch(reject)
        })

        return this.connectionPromise
    }

    _handleMessage(event) {
        try {
            const envelope = typeof event.data === 'string' ? JSON.parse(event.data) : event.data

            // Payload might be double-encoded or already an object
            let payload = envelope.payload
            if (typeof payload === 'string') {
                try {
                    payload = JSON.parse(payload)
                } catch (e) { }
            }

            // Track the latest broadcast event so we can ask for missed ones
            // after a transient disconnect (REQ_GET_MISSED_EVENTS).
            if (envelope.event_id) {
                this.lastEventId = envelope.event_id
            }

            // Check for pending request
            if (envelope.correlation_id && this.pendingRequests.has(envelope.correlation_id)) {
                const { resolve, reject, timeout } = this.pendingRequests.get(envelope.correlation_id)
                clearTimeout(timeout)
                this.pendingRequests.delete(envelope.correlation_id)

                if (envelope.type === 'EVT_ERROR') {
                    const details = payload && payload.details !== undefined ? payload.details : null
                    const detailsText = details == null
                        ? ''
                        : (typeof details === 'string' ? details : JSON.stringify(details))
                    const message = detailsText
                        ? `${payload.error || 'Server error'} (${detailsText})`
                        : (payload.error || 'Server error')
                    reject(new Error(message))
                } else {
                    resolve(payload)
                }
            }

            this._emit(envelope.type, payload || {})
            this._emit('message', envelope)
        } catch (e) {
            console.error('[WS] Parse error:', e)
        }
    }

    /**
     * Replay broadcast events the client missed while disconnected. Called
     * on reconnect BEFORE REQ_SYNC_STATE so the UI catches up on task
     * progress without a heavy full snapshot.
     *
     * Returns the DATA_MISSED_EVENTS payload:
     *   { needs_full_sync: boolean, events?: Envelope[], last_event_id?: string }
     */
    async getMissedEvents(sinceEventId) {
        if (!sinceEventId) {
            return { needs_full_sync: true, events: [] }
        }
        return this.request('REQ_GET_MISSED_EVENTS', { since_event_id: sinceEventId })
    }

    /**
     * Re-inject a replayed envelope into the normal message pipeline so all
     * listeners handle it identically to a fresh delivery. Called by the
     * store after a successful getMissedEvents() round-trip.
     */
    replayEvent(envelope) {
        if (!envelope || !envelope.type) return
        // _handleMessage parses strings; we already have an object, so wrap.
        this._handleMessage({ data: JSON.stringify(envelope) })
    }

    _attemptReconnect() {
        // Retry indefinitely while shouldReconnect is true (only stops on
        // manual disconnect, logout, or session_expired). Exponential backoff
        // capped at 30 s, plus up to 30% jitter to avoid thundering herd.
        if (!this.shouldReconnect) return
        this.reconnectAttempts++
        const base = Math.min(1000 * Math.pow(2, Math.min(this.reconnectAttempts, 6)), 30000)
        const jitter = Math.random() * 0.3 * base
        const delay = Math.round(base + jitter)
        console.log(`[WS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`)
        setTimeout(() => this._establishConnection(), delay)
    }

    // ── Plan 25B: Client-side heartbeat ────────────────────────────────────
    // Sends REQ_PING every 30 s when the connection is open.
    // If DATA_PONG doesn't arrive within 10 s the socket is treated as a
    // zombie, force-closed, and reconnect is triggered immediately.
    _startClientHeartbeat() {
        this._stopClientHeartbeat()
        this._heartbeatInterval = setInterval(() => {
            if (!this.isConnected()) {
                this._stopClientHeartbeat()
                return
            }
            // Send ping with a 10-second timeout
            const pingTimeout = 10000
            this._heartbeatTimeout = setTimeout(() => {
                console.warn('[WS] Heartbeat pong timeout — zombie connection detected; reconnecting')
                this._heartbeatTimeout = null
                // Force-close without suppressing reconnect
                if (this.ws) {
                    const ws = this.ws
                    this.ws = null
                    this.connectionPromise = null
                    try { ws.close() } catch (_) {}
                }
                if (this.shouldReconnect) {
                    this._attemptReconnect()
                }
            }, pingTimeout)

            this.request('REQ_PING', {}, pingTimeout + 1000)
                .then(() => {
                    if (this._heartbeatTimeout) {
                        clearTimeout(this._heartbeatTimeout)
                        this._heartbeatTimeout = null
                    }
                })
                .catch(() => {
                    // request() will reject if connection closes; timeout above handles zombie case
                    if (this._heartbeatTimeout) {
                        clearTimeout(this._heartbeatTimeout)
                        this._heartbeatTimeout = null
                    }
                })
        }, 30000)
    }

    _stopClientHeartbeat() {
        if (this._heartbeatInterval) {
            clearInterval(this._heartbeatInterval)
            this._heartbeatInterval = null
        }
        if (this._heartbeatTimeout) {
            clearTimeout(this._heartbeatTimeout)
            this._heartbeatTimeout = null
        }
    }

    // ── Plan 25C: Visibility-aware reconnect ───────────────────────────────
    // When the tab becomes visible after being hidden, verify the connection
    // is still alive. If not, force an immediate reconnect attempt.
    _handleVisibilityChange() {
        if (typeof document === 'undefined') return
        if (document.visibilityState !== 'visible') return
        if (!this.shouldReconnect) return
        if (this.isConnected()) {
            // Quick ping to verify the connection is not a zombie
            this.request('REQ_PING', {}, 5000).catch(() => {
                console.warn('[WS] Visibility ping failed — reconnecting')
                if (this.ws) {
                    const ws = this.ws
                    this.ws = null
                    this.connectionPromise = null
                    try { ws.close() } catch (_) {}
                }
                this._attemptReconnect()
            })
        } else if (!this.connectionPromise) {
            console.log('[WS] Tab visible and disconnected — reconnecting immediately')
            this._attemptReconnect()
        }
    }

    disconnect(reason = 'manual') {
        this._stopClientHeartbeat()
        if (this.ws) {
            console.log('[WS] Disconnect requested', {
                reason,
                readyState: this.ws.readyState,
                workspaceId: this.workspaceId || null
            })
            this.lastDisconnectReason = reason
            this.shouldReconnect = false
            this.suppressNextReconnect = true
            this._rejectPendingRequests('WebSocket disconnected')
            this.ws.close()
            this.ws = null
            this.connectionPromise = null
            this._iapTokenPromise = null
        }
        // Reject any requests that were parked from a prior network drop so
        // they don't leak across a logout or workspace switch.
        for (const [, entry] of this.pendingOnReconnect.entries()) {
            entry.reject(new Error('WebSocket disconnected'))
        }
        this.pendingOnReconnect.clear()

        this.workspaceId = null
        this.token = null
        // Drop the replay cursor so a fresh login never attempts to resume
        // from a previous user's session.
        if (reason === 'logout' || reason === 'session_expired' || reason === 'app-unmount') {
            this.lastEventId = null
        }
    }

    /**
     * Plan 24, Layer 4 — drain parked pending requests after a transient
     * reconnect. For each request in pendingOnReconnect, asks the server
     * whether it processed the request via REQ_GET_PENDING_RESPONSE. If the
     * server has a cached response (within its 90 s TTL) the original promise
     * is resolved; otherwise it is rejected with a descriptive error.
     *
     * Called by wsStore after syncState() (or event replay) completes so the
     * caller sees the recovered response in the context of a fully-synced store.
     */
    async drainPendingOnReconnect() {
        if (this.pendingOnReconnect.size === 0) return
        // Match server-side ResponseCache TTL (90 s). Requests older than this
        // cannot be in the cache and should fail fast.
        const SERVER_CACHE_TTL_MS = 90000
        const parked = new Map(this.pendingOnReconnect)
        this.pendingOnReconnect.clear()

        const tasks = []
        for (const [correlationId, entry] of parked.entries()) {
            const ageMs = Date.now() - entry.parkedAt
            if (ageMs > SERVER_CACHE_TTL_MS) {
                entry.reject(new Error('Request timed out while disconnected'))
                continue
            }
            tasks.push(
                this.request('REQ_GET_PENDING_RESPONSE', { correlation_id: correlationId }, 10000)
                    .then((resp) => {
                        if (resp?.found && resp.payload != null) {
                            entry.resolve(resp.payload)
                        } else {
                            entry.reject(new Error('Request did not complete before connection was lost'))
                        }
                    })
                    .catch((e) => {
                        entry.reject(new Error('Could not verify request after reconnect: ' + (e?.message || 'unknown')))
                    })
            )
        }
        await Promise.allSettled(tasks)
    }

    async authenticateWithFirebase(token) {
        this.firebaseToken = token
        if (!this.isConnected()) {
            await this.connectAnonymous()
        }
        return this.request('AUTH', { token })
    }

    // Plan 24, Layer 4: when park=true the pending requests are moved to
    // pendingOnReconnect instead of being rejected. This is only safe for
    // unintentional network drops where shouldReconnect=true; intentional
    // disconnects (AFK, logout) always call with park=false (default).
    _rejectPendingRequests(message, park = false) {
        if (this.pendingRequests.size === 0) return
        for (const [correlationId, pending] of this.pendingRequests.entries()) {
            clearTimeout(pending.timeout)
            if (park) {
                this.pendingOnReconnect.set(correlationId, {
                    resolve: pending.resolve,
                    reject: pending.reject,
                    parkedAt: Date.now()
                })
            } else {
                pending.reject(new Error(message))
            }
            this.pendingRequests.delete(correlationId)
        }
    }

    _getStoredWorkspaceId() {
        const raw = localStorage.getItem('workspace')
        if (!raw) return null
        try {
            const parsed = JSON.parse(raw)
            return parsed?.id || null
        } catch (e) {
            return null
        }
    }

    _isLikelyAuthError(message) {
        const text = String(message || '').toLowerCase()
        return text.includes('401') ||
            text.includes('not authenticated') ||
            text.includes('unauthorized') ||
            text.includes('invalid token') ||
            text.includes('user not found')
    }

    async _checkSessionValidityViaRefresh() {
        if (!localStorage.getItem('user')) return null

        try {
            const api = await import('./api.js')
            await api.request('/auth/refresh', { method: 'POST' })
            return true
        } catch (e) {
            if (this._isLikelyAuthError(e?.message || e)) {
                return false
            }
            return null
        }
    }

    async _ensureConnectionForRequest(type) {
        if (this.isConnected()) return

        if (this.connectionPromise) {
            console.log(`[WS] Waiting for in-flight connection before request: ${type}`)
            try {
                await this.connectionPromise
            } catch (e) {
                console.warn('[WS] In-flight connection failed before request', {
                    type,
                    error: e?.message || String(e)
                })
            }
            if (this.isConnected()) return
        }

        const targetWorkspaceId = this.workspaceId || this._getStoredWorkspaceId() || null
        const hasLegacyToken = !!localStorage.getItem('token')
        const isImpersonating = localStorage.getItem('is_impersonating') === '1'
        let reconnectError = null

        try {
            console.warn('[WS] Auto-reconnecting before request', { type, workspaceId: targetWorkspaceId })
            await this.connect(targetWorkspaceId)
        } catch (e) {
            reconnectError = e
            console.warn('[WS] Auto-reconnect failed before request', {
                type,
                workspaceId: targetWorkspaceId,
                error: e?.message || String(e)
            })
        }

        // Legacy token can become stale while cookie session is still valid.
        // Retry once without forcing token query parameter.
        if (!this.isConnected() && hasLegacyToken && !isImpersonating) {
            try {
                console.warn('[WS] Retrying reconnect without legacy token', { type, workspaceId: targetWorkspaceId })
                await this.connect(targetWorkspaceId, null, { skipStoredToken: true })
            } catch (e) {
                if (!reconnectError) reconnectError = e
                console.warn('[WS] Cookie-only reconnect failed before request', {
                    type,
                    workspaceId: targetWorkspaceId,
                    error: e?.message || String(e)
                })
            }
        }

        if (this.isConnected()) return

        const sessionValidity = await this._checkSessionValidityViaRefresh()
        if (sessionValidity === false) {
            this._emit('session_expired', {
                source: 'request',
                requestType: type,
                error: reconnectError?.message || 'session-refresh-401'
            })
            throw new Error('Session expired. Please log in again.')
        }

        if (reconnectError) {
            throw new Error(`WebSocket not connected and failed to reconnect (${reconnectError?.message || 'unknown'})`)
        }

        throw new Error('WebSocket not connected')
    }

    /**
     * Sends a request and waits for a response matched by correlation_id
     * @returns {Promise}
     */
    _sendRequest(type, payload, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.DEFAULT) {
        return new Promise((resolve, reject) => {
            if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
                return reject(new Error('WebSocket not connected'))
            }

            const correlationId = generateUUID()
            const envelope = {
                type,
                correlation_id: correlationId,
                payload: payload
            }

            const timeout = setTimeout(() => {
                if (this.pendingRequests.has(correlationId)) {
                    this.pendingRequests.delete(correlationId)
                    reject(new Error(`Request timeout: ${type}`))
                }
            }, timeoutMs)

            this.pendingRequests.set(correlationId, { resolve, reject, timeout })
            this.ws.send(JSON.stringify(envelope))
        })
    }

    async request(type, payload, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.DEFAULT) {
        if (this.isConnected()) {
            return this._sendRequest(type, payload, timeoutMs)
        }

        await this._ensureConnectionForRequest(type)
        return this._sendRequest(type, payload, timeoutMs)
    }

    send(type, payload, correlationId = null) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.error('[WS] Not connected')
            return false
        }

        const envelope = {
            type,
            correlation_id: correlationId || crypto.randomUUID(),
            payload: payload
        }

        this.ws.send(JSON.stringify(envelope))
        return true
    }

    // Command shortcuts (now returning promises where appropriate)
    startRun(questionSetId, agentIds = [], timeoutMs = WebSocketService.REQUEST_TIMEOUTS.START_RUN) {
        return this.request('CMD_START_RUN', { question_set_id: questionSetId, agent_ids: agentIds }, timeoutMs)
    }

    rerunTask(runId, agentId, questionId, options = {}, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.RERUN_TASK) {
        return this.request('CMD_RERUN_TASK', {
            run_id: runId,
            agent_id: agentId,
            question_id: questionId,
            question_set_id: options.questionSetId || '',
            result_id: options.resultId || '',
            original_question: options.originalQuestion || '',
            expected_answer: options.expectedAnswer || ''
        }, timeoutMs)
    }

    cancelRun(runId) {
        return this.request('CMD_CANCEL_RUN', { run_id: runId })
    }

    /**
     * Plan 24, Layer 3 — Request delta run results since a given timestamp.
     * @param {string} runId  UUID of the run to inspect
     * @param {string|null} sinceTs  ISO-8601 timestamp (e.g. new Date().toISOString()).
     *                               If null, returns all results.
     */
    getRunProgress(runId, sinceTs = null) {
        const payload = { run_id: runId }
        if (sinceTs) payload.since_ts = sinceTs
        return this.request('REQ_GET_RUN_PROGRESS', payload, 30000)
    }

    runEvaluators(runId, evaluatorAgentIds = [], timeoutMs = WebSocketService.REQUEST_TIMEOUTS.RUN_EVALUATORS) {
        return this.request('CMD_RUN_EVALUATORS', { run_id: runId, evaluator_agent_ids: evaluatorAgentIds }, timeoutMs)
    }

    updateAgent(agentId, data, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.UPDATE_AGENT) {
        return this.request('REQ_UPDATE_AGENT', { id: agentId, ...data }, timeoutMs)
    }

    importQuestionSet(clientId, data) {
        return this.request('REQ_IMPORT_QUESTION_SET', { client_id: clientId, ...data })
    }

    exportQuestionSet(questionSetId) {
        return this.request('REQ_EXPORT_QUESTION_SET', { id: questionSetId })
    }

    updateQuestionSet(questionSetId, data) {
        return this.request('REQ_UPDATE_QUESTION_SET', { id: questionSetId, ...data })
    }

    createQuestionSet(workspaceId, data) {
        return this.request('REQ_CREATE_QUESTION_SET', { workspace_id: workspaceId, ...data })
    }

    deleteQuestionSet(questionSetId) {
        return this.request('REQ_DELETE_QUESTION_SET', { id: questionSetId })
    }

    createQuestionSetShareLink(questionSetId, expiresInHours = null) {
        const payload = { question_set_id: questionSetId }
        if (expiresInHours != null) payload.expires_in_hours = expiresInHours
        return this.request('REQ_CREATE_QUESTION_SET_SHARE_LINK', payload)
    }

    getQuestionSetShareLink(token) {
        return this.request('REQ_GET_QUESTION_SET_SHARE_LINK', { token })
    }

    acceptQuestionSetShareLink(token, targetWorkspaceId) {
        return this.request('REQ_ACCEPT_QUESTION_SET_SHARE_LINK', {
            token,
            target_workspace_id: targetWorkspaceId
        })
    }

    // ---- Collaborative Question Sets ----

    createCollabInvite(questionSetId, invitedEmail = null, role = 'editor') {
        const payload = { question_set_id: questionSetId, role }
        if (invitedEmail) payload.invited_email = invitedEmail
        return this.request('REQ_CREATE_QS_COLLAB_INVITE', payload)
    }

    getCollabInvite(token) {
        return this.request('REQ_GET_QS_COLLAB_INVITE', { token })
    }

    acceptCollabInvite(token) {
        return this.request('REQ_ACCEPT_QS_COLLAB_INVITE', { token })
    }

    listCollaborators(questionSetId) {
        return this.request('REQ_LIST_QS_COLLABORATORS', { question_set_id: questionSetId })
    }

    revokeCollaborator(questionSetId, userId) {
        return this.request('REQ_REVOKE_QS_COLLABORATOR', {
            question_set_id: questionSetId,
            user_id: userId
        })
    }

    // ---- Collaborative Agents (Plano 28) ----

    createAgentCollabInvite(agentId, invitedEmail = null, role = 'user') {
        const payload = { agent_id: agentId, role }
        if (invitedEmail) payload.invited_email = invitedEmail
        return this.request('REQ_CREATE_AGENT_COLLAB_INVITE', payload)
    }

    getAgentCollabInvite(token) {
        return this.request('REQ_GET_AGENT_COLLAB_INVITE', { token })
    }

    acceptAgentCollabInvite(token) {
        return this.request('REQ_ACCEPT_AGENT_COLLAB_INVITE', { token })
    }

    listAgentCollaborators(agentId) {
        return this.request('REQ_LIST_AGENT_COLLABORATORS', { agent_id: agentId })
    }

    revokeAgentCollaborator(agentId, userId) {
        return this.request('REQ_REVOKE_AGENT_COLLABORATOR', {
            agent_id: agentId,
            user_id: userId
        })
    }

    updateQuestionSetAgents(questionSetId, agents) {
        return this.request('REQ_UPDATE_QUESTION_SET_AGENTS', { question_set_id: questionSetId, agents })
    }

    getQuestionSetAgentEnvelope(questionSetId) {
        return this.request('REQ_GET_QUESTION_SET_AGENT_ENVELOPE', { question_set_id: questionSetId })
    }

    createAgent(workspaceId, data) {
        return this.request('REQ_CREATE_AGENT', { workspace_id: workspaceId, ...data })
    }

    deleteAgent(agentId, force = false) {
        return this.request('REQ_DELETE_AGENT', { id: agentId, force })
    }

    // Manager API via WS
    getManagerStats() {
        return this.request('REQ_GET_MANAGER_STATS', {})
    }

    getManagerUsers() {
        return this.request('REQ_GET_MANAGER_USERS', {})
    }

    getRunDetails(runId, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.RUN_DETAILS) {
        return this.request('REQ_GET_RUN_DETAILS', { run_id: runId }, timeoutMs)
    }

    deleteRun(runId) {
        return this.request('REQ_DELETE_RUN', { run_id: runId })
    }

    deleteAllRuns() {
        return this.request('REQ_DELETE_ALL_RUNS', {})
    }

    getRunLite(runId, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.RUN_LITE) {
        return this.request('REQ_GET_RUN_LITE', { run_id: runId }, timeoutMs)
    }

    getLatestRunByQuestionSet(questionSetId, timeoutMs = WebSocketService.REQUEST_TIMEOUTS.LATEST_RUN_BY_QS, includeRunning = false) {
        const payload = { question_set_id: questionSetId }
        if (includeRunning) payload.include_running = true
        return this.request('REQ_GET_LATEST_RUN_BY_QS', payload, timeoutMs)
    }

    getResultDetails(resultIds) {
        return this.request('REQ_GET_RESULT_DETAILS', { result_ids: resultIds })
    }

    getRetryStatus(retryIds) {
        return this.request('REQ_GET_RETRY_STATUS', { retry_ids: retryIds })
    }

    createEvaluation(runResultId, rating, comments = '') {
        return this.request('REQ_CREATE_EVALUATION', {
            run_result_id: runResultId,
            rating,
            comments
        })
    }

    getSpyPayload(agentId, question = '') {
        return this.request('REQ_GET_SPY_PAYLOAD', { agent_id: agentId, question })
    }

    getWorkspaceStats(workspaceId, force = false) {
        return this.request('REQ_GET_WORKSPACE_STATS', { workspace_id: workspaceId, force })
    }

    getOrganizationStats(force = false) {
        return this.request('REQ_GET_ORG_STATS', { force })
    }

    getGlobalStats(force = false) {
        return this.request('REQ_GET_GLOBAL_STATS', { force })
    }

    // Dev/Auth methods
    getDevManagers() {
        return this.request('REQ_DEV_GET_MANAGERS', {})
    }

    devLogin(userId) {
        return this.request('REQ_DEV_LOGIN', { user_id: userId })
    }

    getWorkspaces() {
        return this.request('REQ_GET_WORKSPACES', {})
    }

    checkManagerStatus() {
        return this.request('REQ_CHECK_MANAGER_STATUS', {})
    }

    switchWorkspace(workspaceId) {
        return this.request('REQ_SWITCH_WORKSPACE', { workspace_id: workspaceId })
    }

    createWorkspace(name) {
        return this.request('REQ_CREATE_WORKSPACE', { name })
    }

    cloneWorkspace(sourceWorkspaceId, newName) {
        return this.request('REQ_CLONE_WORKSPACE', {
            source_workspace_id: sourceWorkspaceId,
            new_name: newName
        })
    }

    getWorkspaceClients() {
        return this.request('REQ_GET_WORKSPACE_CLIENTS', {})
    }

    getMe() {
        return this.request('REQ_GET_ME', {})
    }

    checkAdminExists() {
        return this.request('REQ_CHECK_ADMIN_EXISTS', {})
    }

    // Auth methods removed - use REST API in api.js instead
    // login, register, bootstrapAdmin are handled via REST to set HttpOnly cookies

    getWorkspaceRuns() {
        return this.request('REQ_GET_WORKSPACE_RUNS', {})
    }

    // Admin methods
    adminGetUsers(filters = {}) {
        return this.request('REQ_ADMIN_GET_USERS', filters)
    }

    adminGetOrganizations(filters = {}) {
        return this.request('REQ_ADMIN_GET_ORGANIZATIONS', filters)
    }

    adminGetUserProfile(userId) {
        return this.request('REQ_ADMIN_GET_USER_PROFILE', { id: userId })
    }

    adminGetOrgProfile(orgId) {
        return this.request('REQ_ADMIN_GET_ORG_PROFILE', { id: orgId })
    }

    adminGetRuns(limit = 100) {
        return this.request('REQ_ADMIN_GET_RUNS', { limit })
    }

    adminCreateUser(userData) {
        return this.request('REQ_ADMIN_CREATE_USER', userData)
    }

    adminUpdateUser(userData) {
        return this.request('REQ_ADMIN_UPDATE_USER', userData)
    }

    adminDeleteUser(userId, mode = 'hard') {
        return this.request('REQ_ADMIN_DELETE_USER', { id: userId, mode: mode })
    }

    adminCreateOrg(orgData) {
        return this.request('REQ_ADMIN_CREATE_ORG', orgData)
    }

    adminUpdateOrg(orgData) {
        return this.request('REQ_ADMIN_UPDATE_ORG', orgData)
    }

    adminDeleteOrg(orgId) {
        return this.request('REQ_ADMIN_DELETE_ORG', { id: orgId })
    }

    adminRemoveUserFromOrg(userId, orgId) {
        return this.request('REQ_ADMIN_REMOVE_USER_FROM_ORG', { user_id: userId, organization_id: orgId })
    }

    adminGetLoginLogs(limit = 100) {
        return this.request('REQ_ADMIN_GET_LOGIN_LOGS', { limit })
    }

    adminGetDebugInfo() {
        return this.request('REQ_ADMIN_GET_DEBUG_INFO', {})
    }

    // Manager Panel Methods
    managerGetWorkspaces() {
        return this.request('REQ_MANAGER_GET_WORKSPACES', {})
    }

    managerGetAgents() {
        return this.request('REQ_MANAGER_GET_AGENTS', {})
    }

    managerGetRuns() {
        return this.request('REQ_MANAGER_GET_RUNS', {})
    }

    managerGetUsers() {
        return this.request('REQ_MANAGER_GET_USERS', {})
    }

    managerCreateUser(userData) {
        return this.request('REQ_MANAGER_CREATE_USER', userData)
    }

    managerUpdateUser(userData) {
        return this.request('REQ_MANAGER_UPDATE_USER', userData)
    }

    managerToggleUserSuspension(userId) {
        return this.request('REQ_MANAGER_TOGGLE_USER_SUSPENSION', { id: userId })
    }

    managerImpersonateUser(userId) {
        return this.request('REQ_MANAGER_IMPERSONATE_USER', { user_id: userId })
    }

    managerGetStats() {
        return this.request('REQ_MANAGER_GET_STATS', {})
    }

    managerGenerateInvite(maxUses = 1) {
        return this.request('REQ_MANAGER_GENERATE_INVITE', { max_uses: maxUses })
    }

    // WebAuthn methods
    async webAuthnRegisterBegin() {
        const data = await this.request('REQ_WEBAUTHN_REGISTER_BEGIN', {})
        this._webAuthnRegSessionId = data.session_id
        return data.options
    }

    webAuthnRegisterFinish(response) {
        return this.request('REQ_WEBAUTHN_REGISTER_FINISH', {
            response,
            session_id: this._webAuthnRegSessionId
        })
    }

    async webAuthnLoginBegin(email) {
        const data = await this.request('REQ_WEBAUTHN_LOGIN_BEGIN', { email })
        this._webAuthnLoginSessionId = data.session_id
        this._webAuthnLoginEmail = email
        return data.options
    }

    async webAuthnLoginFinish(email, response) {
        const targetEmail = email || this._webAuthnLoginEmail
        const sessionId = this._webAuthnLoginSessionId

        const result = await this.request('REQ_WEBAUTHN_LOGIN_FINISH', {
            email: targetEmail,
            response,
            session_id: sessionId
        })

        if (result.token) {
            localStorage.setItem('token', result.token)
            localStorage.setItem('user', JSON.stringify(result.user))
            this.token = result.token
            this._establishConnection() // Re-connect with token
        }

        return result
    }

    webAuthnDeleteKey(keyId) {
        return this.request('REQ_WEBAUTHN_DELETE_KEY', { id: keyId })
    }

    createOrganization(name) {
        return this.request('REQ_CREATE_ORGANIZATION', { name })
    }

    acceptTerms() {
        return this.request('REQ_ACCEPT_TERMS', {})
    }

    joinOrganization(inviteCode) {
        return this.request('REQ_JOIN_ORGANIZATION', { invite_code: inviteCode })
    }

    changePassword(newPassword, oldPassword = '', targetUserId = '') {
        return this.request('REQ_CHANGE_PASSWORD', {
            new_password: newPassword,
            old_password: oldPassword,
            id: targetUserId
        })
    }

    // Event handling
    on(event, callback) {
        if (!this.listeners.has(event)) {
            this.listeners.set(event, [])
        }
        this.listeners.get(event).push(callback)
    }

    off(event, callback) {
        if (!this.listeners.has(event)) return
        const callbacks = this.listeners.get(event)
        const index = callbacks.indexOf(callback)
        if (index > -1) callbacks.splice(index, 1)
    }

    _emit(event, data) {
        if (!this.listeners.has(event)) return
        for (const callback of this.listeners.get(event)) {
            try {
                callback(data)
            } catch (e) {
                console.error('[WS] Callback error:', e)
            }
        }
    }
}

export const wsService = new WebSocketService()
export default wsService
