import { describe, expect, it } from 'vitest'
import { ref } from 'vue'

import { useArenaRunExecution } from './useArenaRunExecution.js'

// The "Retry Failed" and "Retry Missing" buttons must select disjoint sets:
//   - getFailedRetryTargets  -> results that ran and errored
//   - getIncompleteRetryTargets -> results that never ran (missing), plus stale
//     evaluations; it must NOT re-include errored results (those belong to
//     "Retry Failed"). Together they cover every result that still needs a run,
//     with no overlap and no double-counting.

const UUID_A = '11111111-1111-1111-1111-111111111111'
const UUID_E = '22222222-2222-2222-2222-222222222222'

function primaryAgent(id, name = 'Agent') {
  return { id, name, enabled: true, provider_type: 'openai', config: {} }
}

function evaluatorAgent(id, targetAgentId) {
  return { id, name: 'Evaluator', enabled: true, provider_type: 'evaluator', config: { target_agent_id: targetAgentId } }
}

function setup({ agents, questions, runResults }) {
  return useArenaRunExecution({
    currentRun: ref({ id: 'run-1', agentIds: agents.map((a) => a.id) }),
    currentQuestionSet: ref({ id: 'qs-1' }),
    runResults: ref(runResults),
    getFlatQuestions: () => questions,
    getMergedAgents: () => agents
  })
}

const targetKey = (t) => `${t.agentId}::${t.resultKey}::${t.targetAgentId || ''}`

describe('retry target selectors — primary agents', () => {
  const agents = [primaryAgent(UUID_A, 'Agent A')]
  const questions = [{ id: 'q-missing' }, { id: 'q-error' }, { id: 'q-ok' }, { id: 'q-loading' }]
  const runResults = {
    [UUID_A]: {
      // q-missing: no result object at all
      'q-error': { error: 'boom' },
      'q-ok': { answer: 'hello' },
      'q-loading': { loading: true }
    }
  }

  it('Retry Failed selects only the errored result', () => {
    const { getFailedRetryTargets } = setup({ agents, questions, runResults })
    expect(getFailedRetryTargets('primary').map((t) => t.questionId)).toEqual(['q-error'])
  })

  it('Retry Missing selects only the never-run result (excludes the errored one)', () => {
    const { getIncompleteRetryTargets } = setup({ agents, questions, runResults })
    expect(getIncompleteRetryTargets('primary').map((t) => t.questionId)).toEqual(['q-missing'])
  })

  it('Failed and Missing primary sets are disjoint', () => {
    const { getFailedRetryTargets, getIncompleteRetryTargets } = setup({ agents, questions, runResults })
    const failedKeys = new Set(getFailedRetryTargets('primary').map(targetKey))
    const overlap = getIncompleteRetryTargets('primary').filter((t) => failedKeys.has(targetKey(t)))
    expect(overlap).toEqual([])
  })
})

describe('retry target selectors — evaluators', () => {
  const agents = [primaryAgent(UUID_A, 'Agent A'), evaluatorAgent(UUID_E, UUID_A)]
  const questions = [{ id: 'q1' }, { id: 'q2' }, { id: 'q3' }]
  const evalKey = (qid) => `eval-${UUID_A}-${qid}`
  const runResults = {
    [UUID_A]: {
      q1: { answer: 'A1', id: 'p-q1' }, // primary good, evaluation missing
      q2: { answer: 'A2', id: 'p-q2' }, // primary good, evaluation errored
      q3: { answer: 'A3', id: 'p-q3' } // primary good, evaluation stale
    },
    [UUID_E]: {
      [evalKey('q2')]: { error: 'eval boom' }, // errored eval -> Retry Failed
      [evalKey('q3')]: { score: 7, targetRunResultId: 'p-q3-OLD' } // stale eval -> Retry Missing
      // q1 evaluation absent -> Retry Missing
    }
  }

  it('Retry Failed Evaluations selects only the errored evaluation', () => {
    const { getFailedRetryTargets } = setup({ agents, questions, runResults })
    expect(getFailedRetryTargets('evaluator').map((t) => t.resultKey)).toEqual([evalKey('q2')])
  })

  it('Retry Missing Evaluations selects the missing and stale evals, never the errored one', () => {
    const { getIncompleteRetryTargets } = setup({ agents, questions, runResults })
    const keys = getIncompleteRetryTargets('evaluator').map((t) => t.resultKey).sort()
    expect(keys).toEqual([evalKey('q1'), evalKey('q3')].sort())
  })

  it('Failed and Missing evaluator sets are disjoint', () => {
    const { getFailedRetryTargets, getIncompleteRetryTargets } = setup({ agents, questions, runResults })
    const failedKeys = new Set(getFailedRetryTargets('evaluator').map(targetKey))
    const overlap = getIncompleteRetryTargets('evaluator').filter((t) => failedKeys.has(targetKey(t)))
    expect(overlap).toEqual([])
  })
})
