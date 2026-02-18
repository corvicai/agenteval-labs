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
    runResults.value = {}
    taskProgress.value = {}
    currentRun.value = null
    totalTasks.value = 0
    completedTasks.value = 0

    if (!data || !data.run || !data.run.id) {
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
