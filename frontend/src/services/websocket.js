// WebSocket connection manager for real-time updates
import { getWebSocketHost, generateUUID } from '../utils/runtime.js'

class WebSocketService {
    constructor() {
        this.ws = null
        this.workspaceId = null
        this.listeners = new Map()
        this.pendingRequests = new Map() // correlationId -> { resolve, reject, timeout }
        this.reconnectAttempts = 0
        this.maxReconnectAttempts = 5
        this.connectionPromise = null
        this.shouldReconnect = true
        this.suppressNextReconnect = false
        this._iapTokenPromise = null
        this.lastDisconnectReason = null
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

    connect(workspaceId, token = null) {
        this.shouldReconnect = true
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
        } else if (!newToken && legacyToken) {
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
                        this._rejectPendingRequests('WebSocket disconnected')
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

    _attemptReconnect() {
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
            this.reconnectAttempts++
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000)
            console.log(`[WS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`)
            setTimeout(() => this._establishConnection(), delay)
        }
    }

    disconnect(reason = 'manual') {
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
        this.workspaceId = null
        this.token = null
    }

    async authenticateWithFirebase(token) {
        this.firebaseToken = token
        if (!this.isConnected()) {
            await this.connectAnonymous()
        }
        return this.request('AUTH', { token })
    }

    _rejectPendingRequests(message) {
        if (this.pendingRequests.size === 0) return
        for (const [correlationId, pending] of this.pendingRequests.entries()) {
            clearTimeout(pending.timeout)
            pending.reject(new Error(message))
            this.pendingRequests.delete(correlationId)
        }
    }

    /**
     * Sends a request and waits for a response matched by correlation_id
     * @returns {Promise}
     */
    async request(type, payload, timeoutMs = 10000) {
        if (!this.isConnected()) {
            console.log(`[WS] Waiting for connection before request: ${type}`)
            try {
                await this.connectionPromise
            } catch (e) {
                throw new Error('WebSocket not connected and failed to connect')
            }
        }

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
    startRun(questionSetId, agentIds = []) {
        return this.request('CMD_START_RUN', { question_set_id: questionSetId, agent_ids: agentIds })
    }

    rerunTask(runId, agentId, questionId, options = {}) {
        return this.request('CMD_RERUN_TASK', {
            run_id: runId,
            agent_id: agentId,
            question_id: questionId,
            question_set_id: options.questionSetId || '',
            result_id: options.resultId || '',
            original_question: options.originalQuestion || '',
            expected_answer: options.expectedAnswer || ''
        })
    }

    cancelRun(runId) {
        return this.request('CMD_CANCEL_RUN', { run_id: runId })
    }

    runEvaluators(runId, evaluatorAgentIds = []) {
        return this.request('CMD_RUN_EVALUATORS', { run_id: runId, evaluator_agent_ids: evaluatorAgentIds })
    }

    updateAgent(agentId, data) {
        return this.request('REQ_UPDATE_AGENT', { id: agentId, ...data })
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

    updateQuestionSetAgents(questionSetId, agents) {
        return this.request('REQ_UPDATE_QUESTION_SET_AGENTS', { question_set_id: questionSetId, agents })
    }

    createAgent(workspaceId, data) {
        return this.request('REQ_CREATE_AGENT', { workspace_id: workspaceId, ...data })
    }

    deleteAgent(agentId) {
        return this.request('REQ_DELETE_AGENT', { id: agentId })
    }

    // Manager API via WS
    getManagerStats() {
        return this.request('REQ_GET_MANAGER_STATS', {})
    }

    getManagerUsers() {
        return this.request('REQ_GET_MANAGER_USERS', {})
    }

    getRunDetails(runId) {
        return this.request('REQ_GET_RUN_DETAILS', { run_id: runId })
    }

    deleteRun(runId) {
        return this.request('REQ_DELETE_RUN', { run_id: runId })
    }

    deleteAllRuns() {
        return this.request('REQ_DELETE_ALL_RUNS', {})
    }

    getRunLite(runId) {
        return this.request('REQ_GET_RUN_LITE', { run_id: runId })
    }

    getLatestRunByQuestionSet(questionSetId) {
        return this.request('REQ_GET_LATEST_RUN_BY_QS', { question_set_id: questionSetId })
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
