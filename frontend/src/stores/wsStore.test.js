import { describe, expect, it } from 'vitest'
import { wsService } from '../services/websocket.js'
import useWSStore from './wsStore.js'

// useWSStore registers the EVT_DATA_CHANGED listener on first call.
const store = useWSStore()

function emitDataChanged(resource, action, data) {
    wsService._emit('EVT_DATA_CHANGED', { resource, action, data })
}

// A "created" broadcast can be delivered more than once for the same row:
// the creator receives its own workspace broadcast on top of the create
// response/refetch, and the reconnect replay re-delivers buffered events.
// A blind unshift renders the row twice (e.g. duplicated agent view).
describe('wsStore EVT_DATA_CHANGED created dedup', () => {
    it('does not duplicate an agent delivered twice', () => {
        const agent = { id: 'agent-dup-1', name: 'Agent' }
        emitDataChanged('agents', 'created', agent)
        emitDataChanged('agents', 'created', { ...agent })

        expect(store.state.agents.filter(a => a.id === agent.id)).toHaveLength(1)
    })

    it('does not duplicate a question set delivered twice', () => {
        const qs = { id: 'qs-dup-1', name: 'QS' }
        emitDataChanged('question_sets', 'created', qs)
        emitDataChanged('question_sets', 'created', { ...qs })

        expect(store.state.questionSets.filter(q => q.id === qs.id)).toHaveLength(1)
    })

    it('does not duplicate a run delivered twice', () => {
        const run = { id: 'run-dup-1', question_set_id: 'qs-x' }
        emitDataChanged('runs', 'created', run)
        emitDataChanged('runs', 'created', { ...run })

        expect(store.state.recentRuns.filter(r => r.id === run.id)).toHaveLength(1)
    })

    it('still inserts distinct rows', () => {
        emitDataChanged('agents', 'created', { id: 'agent-a' })
        emitDataChanged('agents', 'created', { id: 'agent-b' })

        const ids = store.state.agents.map(a => a.id)
        expect(ids).toContain('agent-a')
        expect(ids).toContain('agent-b')
    })
})
