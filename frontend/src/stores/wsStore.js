import { reactive, readonly } from 'vue'
import wsService from '../services/websocket'

const state = reactive({
    agents: [],
    questionSets: [],
    recentRuns: [],
    isConnected: false,
    isSyncing: false,
    lastError: null,
    onlineUsers: [],
    onlineCount: 0,
    isMaintenance: false,
    runningQuestionSetId: null
})

// Action: Initialize WebSocket and listeners
export function useWSStore() {
    const connect = async (workspaceId, token) => {
        await wsService.connect(workspaceId, token)
    }

    const disconnect = (reason = 'manual') => {
        wsService.disconnect(reason)
        state.isConnected = false
        state.agents = []
        state.questionSets = []
        state.recentRuns = []
        state.lastError = null
        state.onlineUsers = []
        state.onlineCount = 0
    }

    const syncState = async () => {
        if (!state.isConnected) return

        state.isSyncing = true
        try {
            const data = await wsService.request('REQ_SYNC_STATE', {})
            console.log('[WS Store] Sync state received:', data)
            if (data && (data.agents || data.question_sets)) {
                state.agents = data.agents || []
                state.questionSets = data.question_sets || []
                state.recentRuns = data.recent_runs || []
                state.lastError = null
            } else {
                console.warn('[WS Store] Sync returned empty or invalid data, keeping current state')
            }
        } catch (err) {
            console.error('[WS Store] Sync failed:', err)
            state.lastError = err.message
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
                if (action === 'created') state.agents.unshift(data)
                else if (action === 'updated') {
                    const idx = state.agents.findIndex(a => a.id === data.id)
                    if (idx !== -1) state.agents[idx] = data
                } else if (action === 'deleted') {
                    state.agents = state.agents.filter(a => a.id !== data.id)
                }
                break

            case 'question_sets':
                if (action === 'created') state.questionSets.unshift(data)
                else if (action === 'updated') {
                    const idx = state.questionSets.findIndex(q => q.id === data.id)
                    if (idx !== -1) state.questionSets[idx] = data
                } else if (action === 'deleted') {
                    state.questionSets = state.questionSets.filter(q => q.id !== data.id)
                }
                break

            case 'runs':
                if (action === 'created') {
                    state.recentRuns.unshift(data)
                    if (state.recentRuns.length > 20) state.recentRuns.pop()
                }
                // Updates to runs (status changes) are usually handled by individual task events
                // but can be added here if needed.
                break
        }
    }

    // Setup listeners once
    if (wsService.listeners.size === 0) {
        wsService.on('connected', () => {
            state.isConnected = true
            syncState()
        })

        wsService.on('disconnected', () => {
            state.isConnected = false
            state.agents = []
            state.questionSets = []
            state.recentRuns = []
            state.lastError = null
            state.onlineUsers = []
            state.onlineCount = 0
        })

        wsService.on('EVT_DATA_CHANGED', handleDataChanged)

        // Optional: handle specific run events for granular updates
        wsService.on('EVT_TASK_COMPLETED', (payload) => {
            // Update progress in recentRuns if visible
            const run = state.recentRuns.find(r => r.id === payload.run_id)
            if (run) {
                // Increment completed tasks or similar logic
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
        setRunningQuestionSetId: (id) => { state.runningQuestionSetId = id }
    }
}

export default useWSStore
