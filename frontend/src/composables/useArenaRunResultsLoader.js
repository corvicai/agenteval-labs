import { getRecentRunIdForQuestionSet, getCachedRunForQuestionSet, setCachedRunForQuestionSet } from '../utils/arena/cache.js'
import { collectResultIDsForQuestion } from '../utils/arena/results.js'

export function useArenaRunResultsLoader(options = {}) {
  const {
    wsService,
    wsState,
    workspaceId,
    isLoadingResults,
    latestRunCache,
    downloadManager,
    contentCache,
    runResults,
    taskProgress,
    currentRun,
    totalTasks,
    completedTasks,
    getSelectedQuestionId
  } = options

  function getResultSyncSignature(result = {}, agentId = '', questionId = '') {
    const id = String(result?.id || '')
    const status = result?.success === true ? 'success' : (result?.error ? 'error' : 'pending')
    const contentHash = String(result?.content_hash || '')
    return `${id}:${agentId}:${questionId}:${status}:${contentHash}`
  }

  function getCurrentRunLiteSignature() {
    const runId = String(currentRun.value?.id || '')
    if (!runId) return ''

    const runStatus = String(currentRun.value?.status || '')
    const total = String(totalTasks.value || '')
    const resultSignatures = []
    const agentResultsMap = runResults.value || {}

    Object.keys(agentResultsMap).forEach((agentId) => {
      const agentResults = agentResultsMap[agentId] || {}
      Object.keys(agentResults).forEach((questionId) => {
        resultSignatures.push(getResultSyncSignature(agentResults[questionId], agentId, questionId))
      })
    })

    resultSignatures.sort()
    return `${runId}:${runStatus}:${total}:${resultSignatures.join('|')}`
  }

  function getIncomingRunLiteSignature(data) {
    const runId = String(data?.run?.id || '')
    if (!runId) return ''

    const runStatus = String(data?.run?.status || '')
    const total = String(data?.run?.total_tasks || data?.run?.totalTasks || '')
    const results = Array.isArray(data?.results) ? data.results : []
    const resultSignatures = results.map((result) =>
      getResultSyncSignature(result, result?.agent_id, String(result?.question_id))
    )

    resultSignatures.sort()
    return `${runId}:${runStatus}:${total}:${resultSignatures.join('|')}`
  }

  function getRecentRunIdForQS(qsId) {
    return getRecentRunIdForQuestionSet(wsState.recentRuns, qsId)
  }

  function getCachedRunForQS(qsId) {
    return getCachedRunForQuestionSet(latestRunCache, wsState.recentRuns, qsId)
  }

  function setCachedRunForQS(qsId, data) {
    setCachedRunForQuestionSet(latestRunCache, qsId, data)
  }

  function prioritizeQuestionInQueue(questionId) {
    const idsToPrioritize = collectResultIDsForQuestion(runResults.value, questionId)
    if (idsToPrioritize.length > 0) {
      downloadManager.prioritize(idsToPrioritize[0])
    }
  }

  function applyRunLiteData(data) {
    const incomingSignature = getIncomingRunLiteSignature(data)
    if (incomingSignature && incomingSignature === getCurrentRunLiteSignature()) {
      return
    }

    // Snapshot existing in-memory results BEFORE wiping. During a live run,
    // EVT_TASK_COMPLETED fills runResults[agent][q].answer directly, but
    // those answers are never written to contentCache. If we blow the slate
    // clean we'd force a full re-download from the server, leaving the UI
    // showing "loading" for every cell until DownloadManager catches up —
    // which looks like "everything was erased" at EVT_RUN_FINISHED.
    const previousResults = runResults.value || {}

    taskProgress.value = {}
    currentRun.value = null
    totalTasks.value = 0
    completedTasks.value = 0

    if (!data || !data.run || !data.run.id) {
      runResults.value = {}
      return
    }

    currentRun.value = data.run
    const skeletonResults = {}
    const allResultIds = []

    data.results.forEach((res) => {
      const agentId = res.agent_id
      const qIdStr = String(res.question_id)

      if (!skeletonResults[agentId]) skeletonResults[agentId] = {}

      const cached = contentCache.get(res.content_hash)
      const existing = previousResults[agentId]?.[qIdStr]
      const hasInMemoryAnswer = !!existing
        && String(existing.id || '') === String(res.id || '')
        && (typeof existing.answer === 'string' && existing.answer.length > 0 || !!existing.error)

      if (hasInMemoryAnswer) {
        // Warm contentCache so subsequent selections/refreshes find the
        // answer without a round-trip.
        if (res.content_hash && typeof existing.answer === 'string' && existing.answer.length > 0) {
          contentCache.set(res.content_hash, {
            answer: existing.answer,
            evaluations: existing.evaluations || []
          })
        }

        const mergedEvaluations = Array.isArray(res.evaluations) && res.evaluations.length > 0
          ? res.evaluations
          : (existing.evaluations || [])

        skeletonResults[agentId][qIdStr] = {
          ...existing,
          id: res.id,
          content_hash: res.content_hash,
          loading: false,
          success: res.status === 'success',
          error: res.status === 'error' ? (res.error || existing.error || 'Error in run') : (existing.error || null),
          duration: (res.duration_ms != null) ? res.duration_ms / 1000 : existing.duration,
          timestamp: res.created_at || existing.timestamp,
          evaluations: mergedEvaluations,
          humanValidation: mergedEvaluations?.find((evaluation) => evaluation.rater_type === 'user')?.rating ?? existing.humanValidation ?? null,
          metadata: existing.metadata ?? null
        }
        return
      }

      skeletonResults[agentId][qIdStr] = {
        id: res.id,
        content_hash: res.content_hash,
        loading: !cached,
        success: res.status === 'success',
        answer: cached ? cached.answer : '',
        error: res.status === 'error' ? (res.error || 'Error in run') : null,
        duration: res.duration_ms / 1000,
        timestamp: res.created_at,
        evaluations: cached ? (cached.evaluations || []) : [],
        humanValidation: cached ? cached.evaluations?.find((evaluation) => evaluation.rater_type === 'user')?.rating : null,
        metadata: null
      }

      if (!cached) allResultIds.push(res.id)
    })

    runResults.value = skeletonResults
    totalTasks.value = data.run.total_tasks || 0
    completedTasks.value = data.run.total_tasks || 0

    if (allResultIds.length > 0) {
      downloadManager.enqueue(allResultIds)
    }

    const selectedQuestionId = typeof getSelectedQuestionId === 'function' ? getSelectedQuestionId() : ''
    if (selectedQuestionId) {
      prioritizeQuestionInQueue(selectedQuestionId)
    }
  }

  async function fetchLatestResultsForQS(qsId, options = {}) {
    if (!workspaceId()) return
    const force = !!options.force
    isLoadingResults.value = true
    downloadManager.cancelAll()

    try {
      if (!force) {
        const cached = getCachedRunForQS(qsId)
        if (cached) {
          applyRunLiteData(cached)
          return
        }
      } else {
        latestRunCache.delete(qsId)
      }

      const data = await wsService.getLatestRunByQuestionSet(qsId)
      setCachedRunForQS(qsId, data)
      applyRunLiteData(data)
    } catch (e) {
      console.error('[Arena] Failed to load latest results:', e)
    } finally {
      isLoadingResults.value = false
    }
  }

  return {
    getRecentRunIdForQS,
    getCachedRunForQS,
    setCachedRunForQS,
    prioritizeQuestionInQueue,
    applyRunLiteData,
    fetchLatestResultsForQS
  }
}
