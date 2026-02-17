export function registerArenaWsEvents(options = {}) {
  const {
    wsService,
    wsStore,
    contentCache,
    runResults,
    taskProgress,
    isRunning,
    currentRun,
    pendingResultsBuffer,
    currentQuestionSet,
    activeRunQuestionSetId,
    startedTasks,
    totalTasks,
    retryingQuestions,
    saveRunProgress,
    markRetryStarted,
    processTaskCompleted,
    popPendingEvaluators,
    resolveQuestionSetIdForRun,
    triggerEvaluatorRun,
    getEvaluatorIdsForRun,
    hasEvaluatorResultsLoaded,
    clearRunProgress,
    clearRetryTrackingForRun,
    clearAllLoadingStates,
    maybeStopRunningWhenIdle,
    fetchLatestResultsForQS
  } = options

  const handlers = {
    EVT_TASK_QUEUED: (data) => {
      const runId = String(data.run_id || '')
      const currentRunId = String(currentRun.value?.id || '')
      if (currentRunId && runId && runId !== currentRunId) return

      const agentId = data.agent_id
      const qIdStr = String(data.question_id)
      const isEvaluatorTask = qIdStr.startsWith('eval-')

      if (isEvaluatorTask && currentRunId && runId === currentRunId) {
        if (!runResults.value[agentId]) runResults.value[agentId] = {}

        if (!runResults.value[agentId][qIdStr]) {
          runResults.value[agentId][qIdStr] = {
            id: null,
            loading: true,
            queued: true,
            success: false,
            answer: '',
            error: null,
            duration: null,
            timestamp: new Date().toISOString(),
            evaluations: [],
            metadata: null
          }
          totalTasks.value++
        }

        if (!isRunning.value) {
          const targetQuestionSetID = resolveQuestionSetIdForRun(currentRunId)
          isRunning.value = true
          activeRunQuestionSetId.value = targetQuestionSetID
          wsStore.setRunningQuestionSetId(targetQuestionSetID || null)
        }
      }

      if (data.retry_id) {
        markRetryStarted(qIdStr, data.retry_id, {
          runId: data.run_id,
          agentId: data.agent_id,
          questionSetId: currentQuestionSet.value?.id,
          status: 'queued'
        })
      }

      if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
        runResults.value[agentId][qIdStr].queued = true
        runResults.value[agentId][qIdStr].loading = true
        runResults.value[agentId][qIdStr].error = null
      }
    },

    EVT_TASK_STARTED: (data) => {
      if (isRunning.value) {
        startedTasks.value++
        if (currentRun.value?.id) {
          saveRunProgress(currentRun.value.id)
        }
      }
      if (data.retry_id) {
        const qIdStr = String(data.question_id)
        markRetryStarted(qIdStr, data.retry_id, {
          runId: data.run_id,
          agentId: data.agent_id,
          questionSetId: currentQuestionSet.value?.id,
          status: 'running'
        })
      }

      const agentId = data.agent_id
      const qIdStr = String(data.question_id)

      if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
        runResults.value[agentId][qIdStr].queued = false
        runResults.value[agentId][qIdStr].loading = true
        runResults.value[agentId][qIdStr].error = null
      }
    },

    EVT_TASK_PROGRESS: (data) => {
      if (!currentRun.value) return
      if (data.run_id !== currentRun.value.id) return
      const agentId = data.agent_id
      const qIdStr = String(data.question_id)
      if (data.retry_id) {
        markRetryStarted(qIdStr, data.retry_id, {
          runId: data.run_id,
          agentId: data.agent_id,
          questionSetId: currentQuestionSet.value?.id,
          status: 'running'
        })
      }
      if (!taskProgress.value[agentId]) taskProgress.value[agentId] = {}
      taskProgress.value[agentId][qIdStr] = {
        message: data.message || 'Runner still processing...',
        elapsed_ms: data.elapsed_ms || null,
        timestamp: new Date().toISOString()
      }
    },

    EVT_TASK_COMPLETED: (data) => {
      if (!currentRun.value) {
        if (isRunning.value) {
          console.log('[Arena] Buffering result for pending run:', data.run_id)
          pendingResultsBuffer.value.push(data)
        }
        return
      }
      if (data.run_id !== currentRun.value.id) return
      processTaskCompleted(data)
    },

    DATA_RESULT_DETAILS: (payload) => {
      if (!payload.results || !currentRun.value?.id) return
      const runId = String(currentRun.value.id)
      payload.results.forEach((res) => {
        if (String(res.run_id) !== runId) return
        const agentId = res.agent_id
        const qIdStr = String(res.question_id)
        if (!runResults.value[agentId]) return

        const skeleton = runResults.value[agentId][qIdStr]
        if (!skeleton || skeleton.id !== res.id) return

        if (skeleton.content_hash) {
          contentCache.set(skeleton.content_hash, {
            answer: res.answer,
            evaluations: res.evaluations
          })
        }
        runResults.value[agentId][qIdStr] = {
          id: res.id,
          content_hash: skeleton.content_hash,
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
    },

    EVT_RUN_FINISHED: (data) => {
      const finishedRunId = data?.run_id ? String(data.run_id) : ''
      const runIdForPending = finishedRunId || String(currentRun.value?.id || '')
      const queuedEvaluatorIDs = popPendingEvaluators(runIdForPending)
      if (runIdForPending && queuedEvaluatorIDs.length > 0) {
        const targetQuestionSetID = resolveQuestionSetIdForRun(runIdForPending)
        void triggerEvaluatorRun(runIdForPending, targetQuestionSetID, queuedEvaluatorIDs)
        return
      }

      if (finishedRunId && currentRun.value?.id && finishedRunId !== String(currentRun.value.id)) {
        return
      }

      const fallbackRunId = finishedRunId || String(currentRun.value?.id || '')
      const fallbackEvaluatorIDs = getEvaluatorIdsForRun(currentRun.value)
      if (fallbackRunId && fallbackEvaluatorIDs.length > 0 && !hasEvaluatorResultsLoaded()) {
        const targetQuestionSetID = resolveQuestionSetIdForRun(fallbackRunId)
        console.warn('[Arena] Run finished without evaluator results; triggering evaluator fallback', {
          runId: fallbackRunId,
          evaluatorCount: fallbackEvaluatorIDs.length
        })
        void triggerEvaluatorRun(fallbackRunId, targetQuestionSetID, fallbackEvaluatorIDs)
        return
      }

      isRunning.value = false
      localStorage.removeItem('activeRunId')
      taskProgress.value = {}
      activeRunQuestionSetId.value = null
      wsStore.setRunningQuestionSetId(null)

      if (currentRun.value) {
        currentRun.value.status = data?.status || 'completed'
      }
      if (currentRun.value?.id) {
        clearRunProgress(currentRun.value.id)
      }

      if (finishedRunId) {
        clearRetryTrackingForRun(finishedRunId)
      }
      retryingQuestions.value = {}
      clearAllLoadingStates()
      maybeStopRunningWhenIdle()

      if (currentQuestionSet.value?.id) {
        fetchLatestResultsForQS(currentQuestionSet.value.id)
      }
    }
  }

  wsService.on('EVT_TASK_QUEUED', handlers.EVT_TASK_QUEUED)
  wsService.on('EVT_TASK_STARTED', handlers.EVT_TASK_STARTED)
  wsService.on('EVT_TASK_PROGRESS', handlers.EVT_TASK_PROGRESS)
  wsService.on('EVT_TASK_COMPLETED', handlers.EVT_TASK_COMPLETED)
  wsService.on('DATA_RESULT_DETAILS', handlers.DATA_RESULT_DETAILS)
  wsService.on('EVT_RUN_FINISHED', handlers.EVT_RUN_FINISHED)

  return () => {
    wsService.off('EVT_TASK_QUEUED', handlers.EVT_TASK_QUEUED)
    wsService.off('EVT_TASK_STARTED', handlers.EVT_TASK_STARTED)
    wsService.off('EVT_TASK_PROGRESS', handlers.EVT_TASK_PROGRESS)
    wsService.off('EVT_TASK_COMPLETED', handlers.EVT_TASK_COMPLETED)
    wsService.off('DATA_RESULT_DETAILS', handlers.DATA_RESULT_DETAILS)
    wsService.off('EVT_RUN_FINISHED', handlers.EVT_RUN_FINISHED)
  }
}
