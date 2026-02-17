export function useArenaEvaluatorRuns(options = {}) {
  const {
    wsService,
    wsStore,
    wsState,
    currentRun,
    currentQuestionSet,
    activeRunQuestionSetId,
    pendingEvaluatorRuns,
    isRunning,
    completedTasks,
    totalTasks,
    getFlatQuestions,
    startRunError,
    uniqueStringIDs,
    mergeAgentIDs,
    getRunQuestionSetID,
    applyRunLiteData
  } = options

  function setRunError(message) {
    if (!startRunError) return
    startRunError.value = message || 'Failed to run evaluators.'
    setTimeout(() => {
      if (startRunError.value) startRunError.value = null
    }, 5000)
  }

  function resolveQuestionSetIdForRun(runId = '') {
    const targetRunId = String(runId || '')
    if (targetRunId) {
      const recentRuns = wsState.recentRuns || []
      const recent = recentRuns.find((r) => String(r.id) === targetRunId)
      if (recent?.question_set_id) {
        return String(recent.question_set_id)
      }
    }

    return String(
      getRunQuestionSetID(currentRun.value) ||
      activeRunQuestionSetId.value ||
      currentQuestionSet.value?.id ||
      ''
    )
  }

  function queuePendingEvaluators(runId, evaluatorIds) {
    if (!runId) return
    const ids = uniqueStringIDs(evaluatorIds)
    if (ids.length === 0) return
    pendingEvaluatorRuns.value[String(runId)] = ids
  }

  function popPendingEvaluators(runId) {
    if (!runId) return []
    const key = String(runId)
    const ids = uniqueStringIDs(pendingEvaluatorRuns.value[key] || [])
    delete pendingEvaluatorRuns.value[key]
    return ids
  }

  function getPendingEvaluatorIds(runId) {
    if (!runId) return []
    return uniqueStringIDs(pendingEvaluatorRuns.value[String(runId)] || [])
  }

  async function resolveLatestRunIDForQuestionSet(questionSetId) {
    const targetQuestionSetID = String(questionSetId || '')
    const currentRunQuestionSetID = getRunQuestionSetID(currentRun.value)

    if (currentRun.value?.id && currentRunQuestionSetID === targetQuestionSetID) {
      return String(currentRun.value.id)
    }

    const latest = await wsService.getLatestRunByQuestionSet(targetQuestionSetID)
    if (!latest?.run?.id) return ''

    applyRunLiteData(latest)
    return String(latest.run.id)
  }

  async function triggerEvaluatorRun(runId, questionSetId, evaluatorAgentIds) {
    const evalIDs = uniqueStringIDs(evaluatorAgentIds)
    if (!runId || evalIDs.length === 0) return false

    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
    const estimatedEvalTasks = evalIDs.length * flatQuestions.length
    const baseDone = Math.max(completedTasks.value, totalTasks.value)
    if (estimatedEvalTasks > 0) {
      totalTasks.value = baseDone + estimatedEvalTasks
      if (completedTasks.value > totalTasks.value) {
        completedTasks.value = totalTasks.value
      }
    }

    isRunning.value = true
    activeRunQuestionSetId.value = questionSetId
    wsStore.setRunningQuestionSetId(questionSetId)
    currentRun.value = {
      ...(currentRun.value || {}),
      id: String(runId),
      question_set_id: questionSetId || getRunQuestionSetID(currentRun.value),
      status: 'running',
      agentIds: mergeAgentIDs(currentRun.value?.agentIds || [], evalIDs)
    }

    try {
      await wsService.runEvaluators(String(runId), evalIDs)
      return true
    } catch (e) {
      console.error('[Arena] Failed to run evaluators:', e)
      setRunError(e?.message || 'Failed to run evaluators.')
      isRunning.value = false
      activeRunQuestionSetId.value = null
      wsStore.setRunningQuestionSetId(null)
      return false
    }
  }

  async function maybeTriggerQueuedEvaluatorsIfRunAlreadyFinished(runId, questionSetId = '') {
    const targetRunId = String(runId || '')
    if (!targetRunId) return

    const pendingIDs = getPendingEvaluatorIds(targetRunId)
    if (pendingIDs.length === 0) return

    try {
      const runLite = await wsService.getRunLite(targetRunId)
      const status = String(runLite?.run?.status || runLite?.status || '').toLowerCase()
      if (!status || status === 'running') return

      const queuedEvaluatorIDs = popPendingEvaluators(targetRunId)
      if (queuedEvaluatorIDs.length === 0) return

      const targetQuestionSetID = String(questionSetId || resolveQuestionSetIdForRun(targetRunId))
      void triggerEvaluatorRun(targetRunId, targetQuestionSetID, queuedEvaluatorIDs)
    } catch (e) {
      console.warn('[Arena] Failed to verify run status for evaluator trigger:', e)
    }
  }

  return {
    resolveQuestionSetIdForRun,
    queuePendingEvaluators,
    popPendingEvaluators,
    getPendingEvaluatorIds,
    resolveLatestRunIDForQuestionSet,
    triggerEvaluatorRun,
    maybeTriggerQueuedEvaluatorsIfRunAlreadyFinished,
    setRunError
  }
}
