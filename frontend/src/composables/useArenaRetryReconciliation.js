import { resolveRetryStatusItems as resolveRetryStatusItemsUtil } from '../utils/arena/runs.js'
import { parseEvaluatorTaskQuestionID } from '../utils/arena/parsing.js'

export function useArenaRetryReconciliation(options = {}) {
  const {
    wsService,
    wsState,
    wsStore,
    retryRegistry,
    loadRetryRegistry,
    markRetryStarted,
    markRetryFinished,
    persistRetryRegistry,
    hasActiveRetryEntries,
    runResults,
    isRunning,
    activeRunQuestionSetId,
    currentQuestionSet,
    currentRun,
    getDisplayAgents,
    maybeStopRunningWhenIdle,
    fetchLatestResultsForQS
  } = options

  function resolveRetryQuestionRefs(item) {
    const rawQuestionId = item?.question_id != null ? String(item.question_id) : ''
    if (!rawQuestionId) {
      return {
        questionId: '',
        resultKey: ''
      }
    }

    const parsed = parseEvaluatorTaskQuestionID(rawQuestionId)
    return {
      questionId: String(parsed?.questionId || rawQuestionId),
      resultKey: rawQuestionId
    }
  }

  function applyRetryLoadingState(item) {
    const agentId = item?.agent_id
    const { resultKey } = resolveRetryQuestionRefs(item)
    if (!agentId || !resultKey) return

    if (!runResults.value[agentId]) {
      runResults.value[agentId] = {}
    }

    runResults.value[agentId][resultKey] = {
      ...(runResults.value[agentId][resultKey] || {}),
      loading: true,
      queued: item?.status === 'queued',
      error: null,
      placeholder: false
    }
  }

  async function reconcileRetriesFromServer() {
    if (!wsState.isConnected) return
    loadRetryRegistry()

    const retryIds = Object.keys(retryRegistry.value)
    if (retryIds.length === 0) return

    try {
      const response = await wsService.getRetryStatus(retryIds)
      const items = resolveRetryStatusItemsUtil(response)
      const known = new Set()
      let shouldRefreshResults = false

      for (const item of items) {
        if (!item?.retry_id) continue
        const retryId = item.retry_id
        const { questionId: qIdStr } = resolveRetryQuestionRefs(item)
        known.add(retryId)

        if (item.status === 'queued' || item.status === 'running') {
          markRetryStarted(qIdStr, retryId, {
            runId: item.run_id,
            agentId: item.agent_id,
            questionSetId: currentQuestionSet.value?.id,
            status: item.status
          })
          applyRetryLoadingState(item)
          if (!isRunning.value) {
            isRunning.value = true
          }
          if (!activeRunQuestionSetId.value && currentQuestionSet.value?.id) {
            activeRunQuestionSetId.value = currentQuestionSet.value.id
            wsStore.setRunningQuestionSetId(currentQuestionSet.value.id)
          }
          if (!currentRun.value?.id && item.run_id) {
            const agents = typeof getDisplayAgents === 'function' ? (getDisplayAgents() || []) : []
            currentRun.value = {
              id: item.run_id,
              status: 'running',
              agentIds: agents.map((a) => a.id).filter(Boolean)
            }
          }
        } else {
          if (qIdStr) {
            markRetryFinished(qIdStr, retryId, item.status)
          } else {
            delete retryRegistry.value[retryId]
          }
          shouldRefreshResults = true
        }
      }

      retryIds.forEach((retryId) => {
        if (known.has(retryId)) return
        const entry = retryRegistry.value[retryId]
        if (entry?.question_id) {
          const parsed = parseEvaluatorTaskQuestionID(String(entry.question_id))
          markRetryFinished(String(parsed?.questionId || entry.question_id), retryId, 'not_found')
        } else {
          delete retryRegistry.value[retryId]
        }
      })

      persistRetryRegistry()

      if (!hasActiveRetryEntries()) {
        maybeStopRunningWhenIdle()
      }

      if (shouldRefreshResults && currentQuestionSet.value?.id) {
        fetchLatestResultsForQS(currentQuestionSet.value.id, { force: true })
      }
    } catch (e) {
      console.warn('[Arena] Failed to reconcile retries:', e)
    }
  }

  return {
    reconcileRetriesFromServer
  }
}
