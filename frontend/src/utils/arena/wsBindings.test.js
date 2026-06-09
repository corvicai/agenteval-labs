import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

import { registerArenaWsEvents } from './wsBindings.js'

function fakeWsService() {
  const handlers = {}
  return {
    handlers,
    on(type, cb) {
      ;(handlers[type] || (handlers[type] = [])).push(cb)
    },
    off(type, cb) {
      handlers[type] = (handlers[type] || []).filter((h) => h !== cb)
    },
    emit(type, data) {
      ;(handlers[type] || []).forEach((cb) => cb(data))
    }
  }
}

describe('registerArenaWsEvents EVT_RUN_ERROR', () => {
  it('surfaces a run-level error to the user via setRunError', () => {
    const wsService = fakeWsService()
    const setRunError = vi.fn()
    const currentRun = ref({ id: 'run-1' })

    const cleanup = registerArenaWsEvents({ wsService, currentRun, setRunError })

    wsService.emit('EVT_RUN_ERROR', {
      run_id: 'run-1',
      error: 'Evaluator auto-run failed: boom'
    })

    expect(setRunError).toHaveBeenCalledWith('Evaluator auto-run failed: boom')
    cleanup()
  })

  it('ignores an error addressed to a different run', () => {
    const wsService = fakeWsService()
    const setRunError = vi.fn()
    const currentRun = ref({ id: 'run-1' })

    const cleanup = registerArenaWsEvents({ wsService, currentRun, setRunError })

    wsService.emit('EVT_RUN_ERROR', { run_id: 'run-2', error: 'not mine' })

    expect(setRunError).not.toHaveBeenCalled()
    cleanup()
  })
})
