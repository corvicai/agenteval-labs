import { resolveRetryStatusItems as resolveRetryStatusItemsUtil } from '../utils/arena/runs.js'

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

  function applyRetryLoadingState(item) {
    const agentId = item?.agent_id
    const questionId = item?.question_id != null ? String(item.question_id) : ''
    if (!agentId || !questionId) return

    if (!runResults.value[agentId]) {
      runResults.value[agentId] = {}
    }

    runResults.value[agentId][questionId] = {
      ...(runResults.value[agentId][questionId] || {}),
      loading: true,
      queued: item?.status === 'queued',
      error: null
    }
  }

  async function reconcileRetriesFromServer() {
    if (!wsState.isConnected) return
    loadRetryRegistry()

    const retryIds = Object.keys(retryRegistry.value)
    if (retryIds.length === 0) return

    retryIds.forEach((retryId) => {
      const item = retryRegistry.value[retryId]
      if (item?.status === 'queued' || item?.status === 'running') {
        markRetryStarted(item.question_id, retryId, {
          runId: item.run_id,
          agentId: item.agent_id,
          questionSetId: item.question_set_id,
          status: item.status
        })
        applyRetryLoadingState(item)
      }
    })

    try {
      const response = await wsService.getRetryStatus(retryIds)
      const items = resolveRetryStatusItemsUtil(response)
      const known = new Set()
      let shouldRefreshResults = false

      for (const item of items) {
        if (!item?.retry_id) continue
        const retryId = item.retry_id
        const qIdStr = item?.question_id != null ? String(item.question_id) : ''
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
          markRetryFinished(entry.question_id, retryId, 'not_found')
        } else {
          delete retryRegistry.value[retryId]
        }
      })

      persistRetryRegistry()

      if (!hasActiveRetryEntries()) {
        maybeStopRunningWhenIdle()
      }

      if (shouldRefreshResults && currentQuestionSet.value?.id) {
        fetchLatestResultsForQS(currentQuestionSet.value.id)
      }
    } catch (e) {
      console.warn('[Arena] Failed to reconcile retries:', e)
    }
  }

  return {
    reconcileRetriesFromServer
  }
}
