import { reactive, readonly } from 'vue'
import wsService from '../services/websocket'

const state = reactive({
    agents: [],
    sharedAgents: [],
    questionSets: [],
    sharedQuestionSets: [],
    recentRuns: [],
    isConnected: false,
    // isSynced becomes true only after syncState() completes successfully;
    // resets to false on disconnect. Components that need fresh store data
    // (e.g. run restoration) should watch isSynced, not isConnected, so they
    // never read stale recentRuns.
    isSynced: false,
    isSyncing: false,
    lastError: null,
    onlineUsers: [],
    onlineCount: 0,
    isMaintenance: false,
    runningQuestionSetId: null,
    // Populated by syncState when the workspace has an active run; allows
    // BenchmarkArena to restore run results inline without a second request.
    activeRunHydration: null,
    // Populated for admin users only when the encryption key health is not "match".
    // Components can check hasEncryptionIssue to decide whether to surface a banner.
    encryptionKeyHealth: null,
    // Plan 24 Camada 3 — tracks the timestamp of the latest result received per
    // run so we can ask for a delta (REQ_GET_RUN_PROGRESS) on reconnect instead
    // of a full REQ_GET_RUN_DETAILS round-trip.
    // Map<runId: string, isoTimestamp: string>
    lastProgressTsPerRun: {},
})

const listSignature = (items = []) => {
    if (!Array.isArray(items)) return ''
    return items.map(item => {
        const id = item?.id || ''
        const updated = item?.updated_at || item?.updatedAt || item?.created_at || ''
        if (updated) return `${id}:${updated}`
        try {
            return `${id}:${JSON.stringify(item)}`
        } catch (e) {
            return `${id}:`
        }
    }).join('|')
}

const shouldReplaceList = (current, incoming) => {
    if (!Array.isArray(incoming)) return false
    if (!Array.isArray(current)) return true
    if (current.length !== incoming.length) return true
    return listSignature(current) !== listSignature(incoming)
}

// A "created" broadcast can be delivered more than once for the same row
// (own workspace broadcast on top of the create response/refetch, or the
// reconnect replay re-delivering buffered events). Inserting blindly renders
// the row twice. Returns true when the item was actually inserted.
const unshiftUnique = (list, item) => {
    if (item?.id != null && list.some(entry => entry?.id === item.id)) return false
    list.unshift(item)
    return true
}

// Action: Initialize WebSocket and listeners
export function useWSStore() {
    const connect = async (workspaceId, token) => {
        await wsService.connect(workspaceId, token)
    }

    const disconnect = (reason = 'manual') => {
        wsService.disconnect(reason)
        state.isConnected = false
        state.isSynced = false
        state.activeRunHydration = null
        if (reason === 'logout' || reason === 'app-unmount') {
            state.agents = []
            state.sharedAgents = []
            state.questionSets = []
            state.sharedQuestionSets = []
            state.recentRuns = []
            state.lastError = null
            state.onlineUsers = []
            state.onlineCount = 0
        }
    }

    const syncState = async () => {
        if (!state.isConnected) return

        state.isSynced = false
        state.activeRunHydration = null
        state.isSyncing = true
        try {
            const data = await wsService.request('REQ_SYNC_STATE', {})
            console.log('[WS Store] Sync state received:', data)
            if (data && (data.agents || data.question_sets)) {
                const nextAgents = data.agents || []
                const nextSharedAgents = data.shared_agents || []
                const nextQuestionSets = data.question_sets || []
                const nextSharedQuestionSets = data.shared_question_sets || []
                const nextRuns = data.recent_runs || []
                const warnings = Array.isArray(data.warnings) ? data.warnings : []

                if (shouldReplaceList(state.agents, nextAgents)) {
                    state.agents = nextAgents
                }
                if (shouldReplaceList(state.sharedAgents, nextSharedAgents)) {
                    state.sharedAgents = nextSharedAgents
                }
                if (shouldReplaceList(state.questionSets, nextQuestionSets)) {
                    state.questionSets = nextQuestionSets
                }
                if (shouldReplaceList(state.sharedQuestionSets, nextSharedQuestionSets)) {
                    state.sharedQuestionSets = nextSharedQuestionSets
                }
                if (shouldReplaceList(state.recentRuns, nextRuns)) {
                    state.recentRuns = nextRuns
                }
                if (data.active_run_hydration?.run_id) {
                    state.activeRunHydration = data.active_run_hydration
                    console.log('[WS Store] Active run hydration received for run:', data.active_run_hydration.run_id,
                        '— results:', data.active_run_hydration.results?.length ?? 0)
                }
                state.encryptionKeyHealth = data.encryption_health || null
                if (warnings.length > 0) {
                    console.warn('[WS Store] Sync completed with warnings:', warnings)
                }
                state.lastError = null
            } else {
                console.warn('[WS Store] Sync returned empty or invalid data, keeping current state')
            }
            // Signal that all store data is fresh and ready to consume.
            state.isSynced = true
        } catch (err) {
            console.error('[WS Store] Sync failed:', err)
            state.lastError = err.message
            // isSynced stays false — components should not restore state from
            // a potentially stale store.
        } finally {
            state.isSyncing = false
        }
    }

    // Handle data changes from broadcasts
    const handleDataChanged = (payload) => {
        const { resource, action, data } = payload
        console.log(`[WS Store] Data changed: ${resource} ${action}`, data)

        switch (resource) {
            case 'agents':
                if (action === 'created') unshiftUnique(state.agents, data)
                else if (action === 'updated') {
                    const idx = state.agents.findIndex(a => a.id === data.id)
                    if (idx !== -1) state.agents[idx] = data
                } else if (action === 'deleted') {
                    state.agents = state.agents.filter(a => a.id !== data.id)
                }
                break

            case 'shared_agents':
                // Fan-out of an owner's agent update/delete to collaborators
                // with redacted config. Keep sharedAgents in sync so the
                // collaborator UI reflects the latest owner-facing state.
                if (action === 'updated') {
                    const idx = state.sharedAgents.findIndex(a => a.id === data.id)
                    if (idx !== -1) {
                        state.sharedAgents[idx] = { ...state.sharedAgents[idx], ...data }
                    }
                } else if (action === 'deleted') {
                    state.sharedAgents = state.sharedAgents.filter(a => a.id !== data.id)
                }
                break

            case 'question_sets':
                if (action === 'created') unshiftUnique(state.questionSets, data)
                else if (action === 'updated') {
                    const idx = state.questionSets.findIndex(q => q.id === data.id)
                    if (idx !== -1) state.questionSets[idx] = data
                } else if (action === 'deleted') {
                    state.questionSets = state.questionSets.filter(q => q.id !== data.id)
                    state.recentRuns = state.recentRuns.filter(run => String(run?.question_set_id || '') !== String(data?.id || ''))
                    if (String(state.runningQuestionSetId || '') === String(data?.id || '')) {
                        state.runningQuestionSetId = null
                    }
                }
                break

            case 'runs':
                if (action === 'created') {
                    if (unshiftUnique(state.recentRuns, data) && state.recentRuns.length > 20) {
                        state.recentRuns.pop()
                    }
                } else if (action === 'deleted') {
                    state.recentRuns = state.recentRuns.filter(run => run.id !== data.id)
                }
                // Updates to runs (status changes) are usually handled by individual task events
                // but can be added here if needed.
                break
        }
    }

    // Attempt to replay events missed during a transient disconnect. Returns
    // true when the buffer served a clean delta and a full sync is NOT
    // required, false otherwise (cold boot, workspace switch, server
    // restart, TTL expiry, or any error).
    const tryReplayMissedEvents = async () => {
        const cursor = wsService.lastEventId
        if (!cursor) return false
        try {
            const resp = await wsService.getMissedEvents(cursor)
            if (!resp || resp.needs_full_sync) {
                console.log('[WS Store] Replay skipped, server requests full sync')
                return false
            }
            const events = Array.isArray(resp.events) ? resp.events : []
            if (events.length === 0) {
                console.log('[WS Store] Replay empty — already in sync')
            } else {
                console.log(`[WS Store] Replaying ${events.length} missed event(s)`)
                for (const envelope of events) {
                    wsService.replayEvent(envelope)
                }
            }
            if (resp.last_event_id) {
                wsService.lastEventId = resp.last_event_id
            }
            return true
        } catch (e) {
            console.warn('[WS Store] Replay request failed, falling back to full sync:', e?.message || e)
            return false
        }
    }

    // Setup listeners once
    if (wsService.listeners.size === 0) {
        wsService.on('connected', async () => {
            state.isConnected = true
            const resumed = await tryReplayMissedEvents()
            if (resumed) {
                // Live deltas were applied on top of the existing store —
                // mark as synced so watchers waiting on isSynced wake up.
                state.isSynced = true
                // Plan 24 Layer 4: recover any in-flight requests that were
                // parked during the disconnect before marking sync complete.
                await wsService.drainPendingOnReconnect()
                return
            }
            await syncState()
            // Plan 24 Layer 4: recover parked in-flight requests now that
            // the store is fully synced and the server cache is accessible.
            await wsService.drainPendingOnReconnect()
        })

        wsService.on('disconnected', (payload) => {
            state.isConnected = false
            state.isSynced = false
            state.activeRunHydration = null
            const reason = payload?.disconnectReason || 'unknown'
            if (reason === 'logout' || reason === 'app-unmount') {
                state.agents = []
                state.sharedAgents = []
                state.questionSets = []
                state.sharedQuestionSets = []
                state.recentRuns = []
                state.lastError = null
                state.onlineUsers = []
                state.onlineCount = 0
            }
        })

        wsService.on('EVT_DATA_CHANGED', handleDataChanged)

        // Remove the revoked shared QS / Agent from the corresponding list.
        // The payload carries either `question_set_id` (QS collab revoke) or
        // `agent_id` + `resource: "agents"` (agent collab revoke / agent
        // deleted while shared).
        wsService.on('EVT_COLLABORATOR_REVOKED', (payload) => {
            const qsId = payload?.question_set_id
            if (qsId) {
                state.sharedQuestionSets = state.sharedQuestionSets.filter(q => q.id !== qsId)
                console.log('[WS Store] Collaborator revoked — removed shared QS:', qsId)
                return
            }
            const agentId = payload?.agent_id
            if (agentId && (payload?.resource === 'agents' || payload?.resource === 'shared_agents')) {
                state.sharedAgents = state.sharedAgents.filter(a => a.id !== agentId)
                console.log('[WS Store] Collaborator revoked — removed shared agent:', agentId)
            }
        })

        // Optional: handle specific run events for granular updates
        wsService.on('EVT_TASK_COMPLETED', (payload) => {
            // Update progress in recentRuns if visible
            const run = state.recentRuns.find(r => r.id === payload.run_id)
            if (run) {
                // Increment completed tasks or similar logic
            }
            // Plan 24 Camada 3 — stamp latest progress timestamp per run
            if (payload?.run_id) {
                state.lastProgressTsPerRun = {
                    ...state.lastProgressTsPerRun,
                    [payload.run_id]: new Date().toISOString()
                }
            }
        })

        wsService.on('EVT_TASK_PROGRESS', (payload) => {
            if (payload?.run_id) {
                state.lastProgressTsPerRun = {
                    ...state.lastProgressTsPerRun,
                    [payload.run_id]: new Date().toISOString()
                }
            }
        })

        wsService.on('EVT_ONLINE_STATUS', (payload) => {
            state.onlineUsers = payload.user_ids || []
            state.onlineCount = payload.total || 0
        })

        wsService.on('EVT_MAINTENANCE_STARTED', () => {
            console.log('[WS Store] Maintenance started signal received')
            state.isMaintenance = true
        })
    }

    return {
        state: readonly(state),
        connect,
        disconnect,
        syncState,
        startRun: wsService.startRun.bind(wsService),
        cancelRun: wsService.cancelRun.bind(wsService),
        rerunTask: wsService.rerunTask.bind(wsService),
        setRunningQuestionSetId: (id) => { state.runningQuestionSetId = id },
        // Plan 24 Camada 3 — get the stored progress cursor for a run
        getLastProgressTs: (runId) => state.lastProgressTsPerRun[runId] || null,
        // Delegate getRunProgress to websocket service
        getRunProgress: (runId, sinceTs) => wsService.getRunProgress(runId, sinceTs),
        // True when the server reports an encryption key issue (admin-only).
        get hasEncryptionIssue() {
            const s = state.encryptionKeyHealth?.state_status
            return s && s !== 'match' && s !== ''
        }
    }
}

export default useWSStore
