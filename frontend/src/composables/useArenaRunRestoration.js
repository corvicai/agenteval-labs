export function useArenaRunRestoration(options = {}) {
  const {
    wsService,
    wsStore,
    wsState,
    currentQuestionSet,
    currentRun,
    runResults,
    taskProgress,
    isRunning,
    activeRunQuestionSetId,
    isRestoringRun,
    startedTasks,
    completedTasks,
    totalTasks,
    latestRunCache,
    getFlatQuestions,
    clearRunProgress,
    loadRunProgress,
    saveRunProgress,
    fetchLatestResultsForQS,
    mergeQuestionSetForUI,
    resolveRunAgentIds,
    extractQuestionIdsFromQuestionSet
  } = options

  function getRunningRunForCurrentQS() {
    if (!currentQuestionSet.value) return null
    const runs = wsState.recentRuns || []
    const matches = runs.filter((run) => run.status === 'running' && run.question_set_id === currentQuestionSet.value.id)
    if (matches.length === 0) return null
    matches.sort((a, b) => new Date(b.created_at || 0) - new Date(a.created_at || 0))
    return matches[0]
  }

  async function restoreActiveRun(runId) {
    if (!runId || isRestoringRun.value || !wsState.isConnected) return

    isRestoringRun.value = true
    try {
      const data = await wsService.getRunDetails(runId)
      if (!data || !data.run) return

      if (data.run.status === 'running') {
        if (data.question_set && (!currentQuestionSet.value || currentQuestionSet.value.id !== data.question_set.id)) {
          currentQuestionSet.value = mergeQuestionSetForUI(data.question_set, currentQuestionSet.value)
        }

        const runAgentIds = resolveRunAgentIds(data)
        currentRun.value = { ...data.run, agentIds: runAgentIds }
        isRunning.value = true
        activeRunQuestionSetId.value = data.run.question_set_id || data.question_set?.id || null
        wsStore.setRunningQuestionSetId(activeRunQuestionSetId.value)
        localStorage.setItem('activeRunId', runId)

        const storedProgress = loadRunProgress(runId)
        const questionIds = extractQuestionIdsFromQuestionSet(data.question_set)
        const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
        const fallbackTotal = runAgentIds.length * (questionIds.length || flatQuestions.length || 0)
        totalTasks.value = data.run.total_tasks || storedProgress?.total || fallbackTotal

        const baseResults = {}
        if (questionIds.length > 0 && runAgentIds.length > 0) {
          runAgentIds.forEach((agentId) => {
            baseResults[agentId] = {}
            questionIds.forEach((questionId) => {
              baseResults[agentId][questionId] = {
                id: null,
                loading: true,
                success: null,
                answer: '',
                error: null,
                duration: null,
                timestamp: null,
                evaluations: [],
                metadata: null,
                queued: false
              }
            })
          })
        }

        if (data.results) {
          const restored = { ...baseResults }
          data.results.forEach((res) => {
            const agentId = res.agent_id
            const qIdStr = String(res.question_id)
            if (!restored[agentId]) restored[agentId] = {}
            restored[agentId][qIdStr] = {
              id: res.id,
              loading: false,
              success: res.status === 'success',
              answer: res.answer,
              error: res.status === 'error' ? (res.error || 'Error') : null,
              duration: res.duration_ms / 1000,
              timestamp: res.created_at,
              evaluations: res.evaluations || [],
              metadata: res.metadata || null,
              humanValidation: res.evaluations?.find((e) => e.rater_type === 'user')?.rating
            }
          })
          runResults.value = restored
          completedTasks.value = Math.max(data.results.length, storedProgress?.completed || 0)
        } else {
          runResults.value = baseResults
          completedTasks.value = storedProgress?.completed || 0
        }

        startedTasks.value = Math.max(completedTasks.value, storedProgress?.started || completedTasks.value)
        saveRunProgress(runId)
        return
      }

      if (runId) {
        if (data.question_set && (!currentQuestionSet.value || currentQuestionSet.value.id !== data.question_set.id)) {
          currentQuestionSet.value = mergeQuestionSetForUI(data.question_set, currentQuestionSet.value)
        }

        const runAgentIds = resolveRunAgentIds(data)
        const questionIds = extractQuestionIdsFromQuestionSet(data.question_set)
        const restored = {}

        if (questionIds.length > 0 && runAgentIds.length > 0) {
          runAgentIds.forEach((agentId) => {
            restored[agentId] = {}
            questionIds.forEach((questionId) => {
              restored[agentId][questionId] = {
                id: null,
                loading: false,
                queued: false,
                success: null,
                answer: '',
                error: null,
                duration: null,
                timestamp: null,
                evaluations: [],
                metadata: null
              }
            })
          })
        }

        if (Array.isArray(data.results)) {
          data.results.forEach((res) => {
            const agentId = res.agent_id
            const qIdStr = String(res.question_id)
            if (!restored[agentId]) restored[agentId] = {}
            restored[agentId][qIdStr] = {
              id: res.id,
              loading: false,
              queued: false,
              success: res.status === 'success',
              answer: res.answer,
              error: res.status === 'error' ? (res.error || 'Error') : null,
              duration: res.duration_ms / 1000,
              timestamp: res.created_at,
              evaluations: res.evaluations || [],
              metadata: res.metadata || null,
              humanValidation: res.evaluations?.find((e) => e.rater_type === 'user')?.rating
            }
          })
        }

        localStorage.removeItem('activeRunId')
        clearRunProgress(runId)
        isRunning.value = false
        activeRunQuestionSetId.value = null
        wsStore.setRunningQuestionSetId(null)
        taskProgress.value = {}

        currentRun.value = { ...data.run, agentIds: runAgentIds }
        runResults.value = restored
        totalTasks.value = data.run.total_tasks || (Array.isArray(data.results) ? data.results.length : 0)
        completedTasks.value = Array.isArray(data.results) ? data.results.length : 0
        startedTasks.value = completedTasks.value

        const finishedQuestionSetID = String(data.run.question_set_id || data.question_set?.id || '')
        if (finishedQuestionSetID) {
          latestRunCache.delete(finishedQuestionSetID)
        }

        if (completedTasks.value === 0 && currentQuestionSet.value?.id) {
          await fetchLatestResultsForQS(currentQuestionSet.value.id)
        }
      }
    } catch (e) {
      console.error('Failed to restore active run:', e)
    } finally {
      isRestoringRun.value = false
    }
  }

  return {
    getRunningRunForCurrentQS,
    restoreActiveRun
  }
}
