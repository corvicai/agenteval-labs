import { describe, it, expect, vi, beforeEach } from 'vitest'
import { wsService } from './websocket.js'

// Mock runtime utils
vi.mock('../utils/runtime.js', () => ({
    getWebSocketHost: () => 'localhost:3010',
    generateUUID: () => 'test-uuid'
}))

// Mock global WebSocket
class MockWebSocket {
    static CONNECTING = 0
    static OPEN = 1
    static CLOSING = 2
    static CLOSED = 3

    constructor(url, protocols) {
        this.url = url
        this.protocols = protocols
        this.readyState = MockWebSocket.CONNECTING
        setTimeout(() => {
            this.readyState = MockWebSocket.OPEN
            if (this.onopen) this.onopen()
        }, 0)
    }
    send(data) {
        this.lastSent = data
    }
    close() {
        this.readyState = MockWebSocket.CLOSED
        if (this.onclose) this.onclose()
    }
}

global.WebSocket = MockWebSocket

// Mock global fetch for IAP token
global.fetch = vi.fn(() =>
    Promise.resolve({
        ok: true,
        text: () => Promise.resolve('header.payload.signature')
    })
)

describe('WebSocketService', () => {
    beforeEach(() => {
        wsService.disconnect()
        vi.clearAllMocks()
        wsService.ws = null // Ensure fresh state
    })

    it('should establish a connection', async () => {
        await wsService.connectAnonymous()
        expect(wsService.isConnected()).toBe(true)
    })

    it('should handle request/response matching with correlation IDs', async () => {
        await wsService.connectAnonymous()

        const requestPromise = wsService.request('TEST_TYPE', { foo: 'bar' })

        // Simulate server response
        const correlationId = 'test-uuid'
        wsService.ws.onmessage({
            data: JSON.stringify({
                type: 'DATA_RESPONSE',
                correlation_id: correlationId,
                payload: { success: true }
            })
        })

        const result = await requestPromise
        expect(result.success).toBe(true)
    })

    it('should handle errors correctly', async () => {
        await wsService.connectAnonymous()

        const requestPromise = wsService.request('TEST_TYPE', {})

        wsService.ws.onmessage({
            data: JSON.stringify({
                type: 'EVT_ERROR',
                correlation_id: 'test-uuid',
                payload: { error: 'Something went wrong' }
            })
        })

        await expect(requestPromise).rejects.toThrow('Something went wrong')
    })

    it('asks for running runs when the recovery probe requests them', async () => {
        await wsService.connectAnonymous()

        const requestPromise = wsService.getLatestRunByQuestionSet('qs-1', 1000, true)
        const sent = JSON.parse(wsService.ws.lastSent)
        expect(sent.payload.include_running).toBe(true)
        expect(sent.payload.question_set_id).toBe('qs-1')

        wsService.ws.onmessage({
            data: JSON.stringify({
                type: 'DATA_RESPONSE',
                correlation_id: 'test-uuid',
                payload: { run: null }
            })
        })
        await requestPromise
    })

    it('should emit events to listeners', async () => {
        await wsService.connectAnonymous()
        const callback = vi.fn()
        wsService.on('CUSTOM_EVENT', callback)

        wsService.ws.onmessage({
            data: JSON.stringify({
                type: 'CUSTOM_EVENT',
                payload: { data: 'hello' }
            })
        })

        expect(callback).toHaveBeenCalledWith({ data: 'hello' })
    })

    it('should track event_id from broadcast envelopes', async () => {
        await wsService.connectAnonymous()
        expect(wsService.lastEventId).toBeNull()

        wsService.ws.onmessage({
            data: JSON.stringify({
                type: 'EVT_TASK_COMPLETED',
                event_id: 'abc123:42',
                payload: { run_id: 'r1' }
            })
        })

        expect(wsService.lastEventId).toBe('abc123:42')

        // Responses without event_id must not overwrite the cursor.
        wsService.ws.onmessage({
            data: JSON.stringify({
                type: 'DATA_RESPONSE',
                correlation_id: 'test-uuid',
                payload: {}
            })
        })
        expect(wsService.lastEventId).toBe('abc123:42')
    })

    it('getMissedEvents forces full sync when no cursor exists', async () => {
        await wsService.connectAnonymous()
        wsService.lastEventId = null
        const resp = await wsService.getMissedEvents(null)
        expect(resp).toEqual({ needs_full_sync: true, events: [] })
    })

    it('replayEvent re-routes envelopes through listeners', async () => {
        await wsService.connectAnonymous()
        const callback = vi.fn()
        wsService.on('EVT_TASK_COMPLETED', callback)

        wsService.replayEvent({
            type: 'EVT_TASK_COMPLETED',
            event_id: 'abc123:7',
            payload: { run_id: 'r1', success: true }
        })

        expect(callback).toHaveBeenCalledWith({ run_id: 'r1', success: true })
        expect(wsService.lastEventId).toBe('abc123:7')
    })

    it('disconnect with logout reason clears the replay cursor', async () => {
        await wsService.connectAnonymous()
        wsService.lastEventId = 'abc123:9'
        wsService.disconnect('logout')
        expect(wsService.lastEventId).toBeNull()
    })

    it('should pass IAP token in subprotocols if it is a valid JWT', async () => {
        // Force a non-localhost hostname for the test
        delete window.location
        window.location = { hostname: 'agenteval-dev.corviclabs.ai', protocol: 'https:' }

        await wsService.connectAnonymous()

        expect(global.fetch).toHaveBeenCalledWith('/_gcp_iap/identityToken', expect.any(Object))
        expect(wsService.ws.protocols).toEqual(['iap-bearer-token', 'header.payload.signature'])
    })
})
