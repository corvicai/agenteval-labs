import { isEvaluatorAgentObject } from '../utils/arena/agents.js'
import { parseEvaluatorTaskQuestionID } from '../utils/arena/parsing.js'

const START_RUN_RECOVERY_MAX_ATTEMPTS = 3
const START_RUN_RECOVERY_DELAY_MS = 1200
const START_RUN_RECOVERY_TIMEOUT_MS = 30000

function showAlert(message) {
  if (!message) return
  if (typeof window !== 'undefined' && typeof window.alert === 'function') {
    window.alert(message)
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function isTransientStartRunError(error) {
  const message = String(error?.message || error || '').toLowerCase()
  if (!message) return false
  return message.includes('request timeout: cmd_start_run') ||
    message.includes('websocket not connected') ||
    message.includes('websocket disconnected')
}

export function useArenaRunExecution(options = {}) {
  const {
    wsService,
    wsStore,
    currentQuestionSet,
    currentRun,
    runResults,
    taskProgress,
    isRunning,
    activeRunQuestionSetId,
    startedTasks,
    completedTasks,
    totalTasks,
    showRunSetup,
    pendingResultsBuffer,
    startRunError,
    canStartEvaluation,
    startEvaluationDisabledReason,
    enabledEvaluatorAgents,
    getFlatQuestions,
    getMergedAgents,
    splitSelectedAgents,
    resolveLatestRunIDForQuestionSet,
    triggerEvaluatorRun,
    setRunError,
    mergeAgentIDs,
    saveRunProgress,
    clearRunProgress,
    markRetryStarted,
    markRetryFinished,
    getPendingEvaluatorIds,
    popPendingEvaluators,
    resolveQuestionSetIdForRun,
    getAgentResults
  } = options

  function applyStartedRunState(runId, questionSetId, primaryAgentIds, evaluatorAgentIds, backendTotalTasks = 0) {
    const normalizedRunId = String(runId || '')
    if (!normalizedRunId) return false

    currentRun.value = {
      id: normalizedRunId,
      question_set_id: questionSetId,
      status: 'running',
      agentIds: mergeAgentIDs(primaryAgentIds, evaluatorAgentIds)
    }

    const parsedTotalTasks = Number(backendTotalTasks || 0)
    if (parsedTotalTasks > 0) {
      totalTasks.value = parsedTotalTasks
    }

    localStorage.setItem('activeRunId', currentRun.value.id)
    saveRunProgress(currentRun.value.id)

    if (pendingResultsBuffer.value.length > 0) {
      pendingResultsBuffer.value.forEach((data) => {
        if (data.run_id === currentRun.value.id) {
          processTaskCompleted(data)
        }
      })
      pendingResultsBuffer.value = []
    }

    if (startRunError?.value) {
      startRunError.value = null
    }
    return true
  }

  async function tryRecoverStartedRun(questionSetId, primaryAgentIds, evaluatorAgentIds) {
    const targetQuestionSetId = String(questionSetId || '')
    if (!targetQuestionSetId) return false

    for (let attempt = 0; attempt < START_RUN_RECOVERY_MAX_ATTEMPTS; attempt++) {
      if (attempt > 0) {
        await sleep(START_RUN_RECOVERY_DELAY_MS)
      }

      try {
        const latest = await wsService.getLatestRunByQuestionSet(targetQuestionSetId, START_RUN_RECOVERY_TIMEOUT_MS)
        const latestRun = latest?.run
        if (!latestRun?.id) continue

        const status = String(latestRun.status || '').toLowerCase()
        if (status !== 'running') continue

        const recovered = applyStartedRunState(
          latestRun.id,
          targetQuestionSetId,
          primaryAgentIds,
          evaluatorAgentIds,
          latestRun.total_tasks || latest?.total_tasks || 0
        )
        if (recovered) {
          console.warn('[Arena] Recovered run after start timeout/disconnect', {
            runId: String(latestRun.id),
            questionSetId: targetQuestionSetId,
            attempt: attempt + 1
          })
          return true
        }
      } catch (recoveryError) {
        console.warn('[Arena] Recovery probe failed after start error', {
          questionSetId: targetQuestionSetId,
          attempt: attempt + 1,
          error: recoveryError?.message || String(recoveryError)
        })
      }
    }

    return false
  }

  async function startEvaluationNow() {
    if (!canStartEvaluation.value) {
      if (startEvaluationDisabledReason.value) {
        showAlert(startEvaluationDisabledReason.value)
      }
      return
    }

    const questionSetId = currentQuestionSet.value?.id
    const evaluatorIds = (enabledEvaluatorAgents.value || []).map((agent) => agent.id)
    if (!questionSetId || evaluatorIds.length === 0) return

    try {
      const runId = await resolveLatestRunIDForQuestionSet(questionSetId)
      if (!runId) {
        showAlert('No previous run found for this question set. Run at least one benchmark agent first.')
        return
      }
      await triggerEvaluatorRun(runId, questionSetId, evaluatorIds)
    } catch (e) {
      console.error('[Arena] Manual evaluator run failed:', e)
      setRunError(e?.message || 'Failed to run evaluators.')
    }
  }

  async function handleStartRun(payload) {
    const { questionSetId } = payload || {}
    const { primary: primaryAgentIds, evaluators: evaluatorAgentIds } = splitSelectedAgents(payload || {})
    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []

    showRunSetup.value = false
    if (flatQuestions.length === 0) {
      showAlert('The current question set is empty.')
      return
    }

    if (primaryAgentIds.length === 0 && evaluatorAgentIds.length === 0) {
      showAlert('Select at least one agent to run.')
      return
    }

    if (primaryAgentIds.length === 0 && evaluatorAgentIds.length > 0) {
      showAlert('Evaluator-only run is not allowed in Run Setup. Select at least one primary agent.')
      return
    }

    const selectedPrimaryCount = primaryAgentIds.length
    isRunning.value = true
    activeRunQuestionSetId.value = questionSetId
    wsStore.setRunningQuestionSetId(questionSetId)
    startedTasks.value = 0
    completedTasks.value = 0
    totalTasks.value = selectedPrimaryCount * flatQuestions.length
    runResults.value = {}
    taskProgress.value = {}

    try {
      const result = await wsService.startRun(questionSetId, primaryAgentIds)
      applyStartedRunState(
        result.run_id || result.id,
        questionSetId,
        primaryAgentIds,
        evaluatorAgentIds,
        result.total_tasks || result.totalTasks || 0
      )
    } catch (e) {
      console.error('Failed to start run:', e)

      if (isTransientStartRunError(e)) {
        const recovered = await tryRecoverStartedRun(questionSetId, primaryAgentIds, evaluatorAgentIds)
        if (recovered) {
          return
        }
      }

      startRunError.value = e.message || 'Failed to start run. Please check your agent configurations.'
      setTimeout(() => {
        if (startRunError.value) startRunError.value = null
      }, 5000)

      isRunning.value = false
      pendingResultsBuffer.value = []
      taskProgress.value = {}
      activeRunQuestionSetId.value = null
      wsStore.setRunningQuestionSetId(null)
    }
  }

  function processTaskCompleted(data) {
    completedTasks.value++
    if (!runResults.value[data.agent_id]) runResults.value[data.agent_id] = {}

    runResults.value[data.agent_id] = {
      ...runResults.value[data.agent_id],
      [data.question_id]: {
        id: data.run_result_id,
        loading: false,
        success: data.success,
        answer: data.answer,
        error: data.error,
        duration: data.duration_ms / 1000,
        timestamp: new Date().toISOString(),
        evaluations: data.evaluations || [],
        metadata: data.metadata || null
      }
    }

    if (taskProgress.value[data.agent_id]) {
      delete taskProgress.value[data.agent_id][data.question_id]
      if (Object.keys(taskProgress.value[data.agent_id]).length === 0) {
        delete taskProgress.value[data.agent_id]
      }
    }

    if (currentRun.value?.id) {
      saveRunProgress(currentRun.value.id)
    }

    const qIdStr = String(data.question_id)
    if (data.retry_id) {
      markRetryFinished(qIdStr, data.retry_id)
    }

    if (qIdStr.startsWith('eval-')) {
      let targetResultId = String(data.target_run_result_id || data.targetRunResultID || '')
      if (!targetResultId) {
        const parsed = parseEvaluatorTaskQuestionID(qIdStr)
        if (parsed) {
          targetResultId = String(runResults.value?.[parsed.targetAgentId]?.[parsed.questionId]?.id || '')
        }
      }
      if (targetResultId) {
        void wsService.getResultDetails([targetResultId]).catch((err) => {
          console.warn('[Arena] Failed to refresh target result after evaluator completion:', err)
        })
      }
    }

    const completedRunId = String(currentRun.value?.id || data.run_id || '')
    const hasQueuedEvaluators = !!completedRunId && getPendingEvaluatorIds(completedRunId).length > 0
    if (totalTasks.value > 0 && completedTasks.value >= totalTasks.value && (isRunning.value || hasQueuedEvaluators)) {
      const queuedEvaluatorIDs = popPendingEvaluators(completedRunId)
      if (completedRunId && queuedEvaluatorIDs.length > 0) {
        const targetQuestionSetID = resolveQuestionSetIdForRun(completedRunId)
        void triggerEvaluatorRun(completedRunId, targetQuestionSetID, queuedEvaluatorIDs)
        return
      }

      isRunning.value = false
      if (currentRun.value) {
        currentRun.value.status = 'completed'
      }
      if (activeRunQuestionSetId.value === currentQuestionSet.value?.id) {
        wsStore.setRunningQuestionSetId(null)
      }
      activeRunQuestionSetId.value = null
      localStorage.removeItem('activeRunId')
      taskProgress.value = {}
      if (currentRun.value?.id) {
        clearRunProgress(currentRun.value.id)
      }
    }
  }

  async function cancelBenchmark() {
    if (!currentRun.value) return
    try {
      await wsService.cancelRun(currentRun.value.id)
      isRunning.value = false
      currentRun.value.status = 'cancelled'
      wsStore.setRunningQuestionSetId(null)
      taskProgress.value = {}
      clearRunProgress(currentRun.value.id)
    } catch (e) {
      console.error('Failed to cancel run:', e)
    }
  }

  async function rerunQuestion(agentId, questionId, localRetryId = null) {
    if (!currentRun.value) return

    const qIdStr = String(questionId)
    if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
      runResults.value[agentId][qIdStr].loading = true
      runResults.value[agentId][qIdStr].error = null
    }

    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
    const mergedAgents = typeof getMergedAgents === 'function' ? getMergedAgents() : []
    const question = flatQuestions.find((item) => String(item.id) === qIdStr)
    const resultItem = runResults.value[agentId]?.[qIdStr]

    let resultIdToUse = resultItem?.id
    const agent = mergedAgents.find((item) => item.id === agentId)

    if (agent && (isEvaluatorAgentObject(agent) || agent.config?.target_agent_id)) {
      const targetAgentId = agent.config?.target_agent_id
      const candidates = []

      for (const aid in runResults.value) {
        if (aid === agentId) continue
        const res = runResults.value[aid][qIdStr]
        if (res && res.answer) {
          candidates.push({ ...res, agent_id: aid })
        }
      }

      let targetMatch = null
      if (targetAgentId) {
        targetMatch = candidates.find((candidate) => candidate.agent_id === targetAgentId)
      } else if (candidates.length === 1) {
        targetMatch = candidates[0]
      } else if (candidates.length > 0) {
        targetMatch = candidates[0]
      }

      if (targetMatch) {
        resultIdToUse = targetMatch.id
      } else {
        resultIdToUse = ''
      }
    }

    try {
      const response = await wsService.rerunTask(currentRun.value.id, agentId, questionId, {
        questionSetId: currentQuestionSet.value?.id,
        resultId: resultIdToUse,
        originalQuestion: question?.question || '',
        expectedAnswer: question?.expected || question?.expected_answer || ''
      })
      const retryId = response?.retry_id || response?.retryId
      if (retryId) {
        markRetryStarted(qIdStr, retryId, {
          runId: currentRun.value?.id,
          agentId,
          questionSetId: currentQuestionSet.value?.id,
          status: 'queued'
        })
        if (localRetryId) {
          markRetryFinished(qIdStr, localRetryId)
        }
      }
    } catch (e) {
      console.error('Failed to rerun:', e)
      if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
        runResults.value[agentId][qIdStr].loading = false
      }
      if (localRetryId) {
        markRetryFinished(qIdStr, localRetryId)
      }
    }
  }

  async function retryQuestionForAllAgents(questionId) {
    if (!currentRun.value || !questionId) {
      showAlert('No active run. Please start a benchmark first.')
      return
    }

    const qIdStr = String(questionId)
    const mergedAgents = typeof getMergedAgents === 'function' ? getMergedAgents() : []
    const enabledAgents = mergedAgents.filter((agent) => agent.enabled && !isEvaluatorAgentObject(agent))

    if (enabledAgents.length === 0) {
      showAlert('No enabled agents found. Please enable at least one agent.')
      return
    }

    const localRetryIds = {}
    enabledAgents.forEach((agent) => {
      const localRetryId = `local-${agent.id}-${Date.now()}`
      localRetryIds[agent.id] = localRetryId
      markRetryStarted(qIdStr, localRetryId)
      if (!runResults.value[agent.id]) runResults.value[agent.id] = {}
      runResults.value[agent.id][qIdStr] = {
        ...(runResults.value[agent.id][qIdStr] || {}),
        loading: true,
        queued: false,
        error: null
      }
    })

    for (const agent of enabledAgents) {
      await rerunQuestion(agent.id, questionId, localRetryIds[agent.id])
    }
  }

  async function rateResult(resultId, rating) {
    try {
      return await wsService.createEvaluation(resultId, rating)
    } catch (e) {
      console.error('Failed to rate:', e)
      return null
    }
  }

  function onValidation(agentId, index, validation) {
    const results = getAgentResults(agentId)
    const result = results[index]
    if (!result || !result.id) return

    const qIdStr = String(result.question.id)
    rateResult(result.id, validation).then((newEval) => {
      if (runResults.value[agentId] && runResults.value[agentId][qIdStr]) {
        const item = runResults.value[agentId][qIdStr]
        item.humanValidation = validation
        if (newEval) {
          if (!item.evaluations) item.evaluations = []
          const existingIdx = item.evaluations.findIndex((evaluation) => evaluation.rater_type === 'user')
          if (existingIdx !== -1) {
            item.evaluations[existingIdx] = newEval
          } else {
            item.evaluations.push(newEval)
          }
        }
      }
    })
  }

  function onRetry(agentId, index) {
    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
    const question = flatQuestions[index]
    if (question && currentRun.value) {
      rerunQuestion(agentId, question.id)
    }
  }

  return {
    startEvaluationNow,
    handleStartRun,
    processTaskCompleted,
    cancelBenchmark,
    rerunQuestion,
    retryQuestionForAllAgents,
    onValidation,
    onRetry
  }
}
