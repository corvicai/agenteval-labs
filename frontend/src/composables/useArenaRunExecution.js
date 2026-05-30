import { isEvaluatorAgentObject } from '../utils/arena/agents.js'
import { parseEvaluatorTaskQuestionID } from '../utils/arena/parsing.js'
import { capturePostHogEvent } from '../services/posthog.js'

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
    getAgentResults,
    onTaskCompleted
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
      capturePostHogEvent('benchmark_run_started', {
        question_set_id: questionSetId,
        run_id: result.run_id || result.id || '',
        primary_agent_count: primaryAgentIds.length,
        evaluator_agent_count: evaluatorAgentIds.length,
        question_count: flatQuestions.length
      })
    } catch (e) {
      console.error('Failed to start run:', e)

      if (isTransientStartRunError(e)) {
        const recovered = await tryRecoverStartedRun(questionSetId, primaryAgentIds, evaluatorAgentIds)
        if (recovered) {
          capturePostHogEvent('benchmark_run_started', {
            question_set_id: questionSetId,
            run_id: currentRun.value?.id || '',
            primary_agent_count: primaryAgentIds.length,
            evaluator_agent_count: evaluatorAgentIds.length,
            question_count: flatQuestions.length,
            recovered_from_disconnect: true
          })
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
    if (typeof onTaskCompleted === 'function') {
      onTaskCompleted(data)
    }
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
        metadata: data.metadata || null,
        // Evaluator results carry the ID of the primary result they evaluated.
        // Stored here so getIncompleteRetryTargets can detect stale evaluations
        // after a primary-answer retry (auto-eval won't re-trigger when prior
        // eval results already exist for the run).
        targetRunResultId: data.target_run_result_id || null
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
      const retryQuestionId = qIdStr.startsWith('eval-')
        ? (parseEvaluatorTaskQuestionID(qIdStr)?.questionId || qIdStr)
        : qIdStr
      markRetryFinished(retryQuestionId, data.retry_id)
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
      capturePostHogEvent('benchmark_run_completed', {
        run_id: completedRunId,
        question_set_id: resolveQuestionSetIdForRun(completedRunId) || currentQuestionSet.value?.id || ''
      })
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
      capturePostHogEvent('benchmark_run_cancelled', {
        run_id: currentRun.value.id,
        question_set_id: currentQuestionSet.value?.id || ''
      })
      isRunning.value = false
      currentRun.value.status = 'cancelled'
      wsStore.setRunningQuestionSetId(null)
      taskProgress.value = {}
      clearRunProgress(currentRun.value.id)
    } catch (e) {
      console.error('Failed to cancel run:', e)
    }
  }

  async function rerunQuestion(agentId, questionId, localRetryId = null, rerunOptions = {}) {
    if (!currentRun.value) return

    const qIdStr = String(questionId)
    const resultKey = String(rerunOptions?.resultKey || qIdStr)
    if (runResults.value[agentId] && runResults.value[agentId][resultKey]) {
      runResults.value[agentId][resultKey].loading = true
      runResults.value[agentId][resultKey].error = null
    }

    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
    const mergedAgents = typeof getMergedAgents === 'function' ? getMergedAgents() : []
    const question = flatQuestions.find((item) => String(item.id) === qIdStr)
    const resultItem = runResults.value[agentId]?.[resultKey] || runResults.value[agentId]?.[qIdStr]
    const explicitTargetAgentId = String(rerunOptions?.targetAgentId || '')

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
      if (explicitTargetAgentId) {
        targetMatch = candidates.find((candidate) => candidate.agent_id === explicitTargetAgentId)
      } else if (targetAgentId) {
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
        originalQuestion: question?.question || question?.text || '',
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
      if (runResults.value[agentId] && runResults.value[agentId][resultKey]) {
        runResults.value[agentId][resultKey].loading = false
      }
      if (localRetryId) {
        markRetryFinished(qIdStr, localRetryId)
      }
    }
  }

  function getRetryEligibleAgents(kind = 'primary') {
    const mergedAgents = typeof getMergedAgents === 'function' ? getMergedAgents() : []
    const runAgentIds = currentRun.value?.agentIds
    const runAgentSet = runAgentIds?.length > 0 ? new Set(runAgentIds) : null
    return mergedAgents.filter((agent) => {
      if (!agent.enabled) return false
      // Restrict to agents that actually participated in this run so that a
      // newly-added agent does not appear as "missing" and get queued into an
      // already-completed historical run (history mutation).
      if (runAgentSet && !runAgentSet.has(agent.id)) return false
      const isEvaluator = isEvaluatorAgentObject(agent)
      if (kind === 'evaluator') return isEvaluator
      if (kind === 'all') return true
      return !isEvaluator
    })
  }

  function markLocalRetryPending(agentId, questionId, localRetryId, options = {}) {
    const qIdStr = String(questionId)
    const resultKey = String(options?.resultKey || qIdStr)
    markRetryStarted(qIdStr, localRetryId)
    if (!runResults.value[agentId]) runResults.value[agentId] = {}
    runResults.value[agentId][resultKey] = {
      ...(runResults.value[agentId][resultKey] || {}),
      loading: true,
      queued: false,
      error: null
    }
  }

  function getFailedRetryTargets(kind = 'primary') {
    if (!currentRun.value) return []

    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
    const knownQuestionIds = new Set(flatQuestions.map((question) => String(question.id)))
    const enabledAgents = getRetryEligibleAgents(kind)
    const targets = []

    flatQuestions.forEach((question) => {
      const qIdStr = String(question.id)
      enabledAgents.forEach((agent) => {
        if (isEvaluatorAgentObject(agent)) {
          const agentResults = runResults.value[agent.id] || {}
          Object.entries(agentResults).forEach(([resultKey, result]) => {
            const parsed = parseEvaluatorTaskQuestionID(String(resultKey))
            const originalQuestionId = String(parsed?.questionId || resultKey)
            if (originalQuestionId !== qIdStr) return
            if (!knownQuestionIds.has(originalQuestionId)) return
            if (!result?.error) return
            if (result.loading || result.queued) return
            targets.push({
              agentId: agent.id,
              questionId: originalQuestionId,
              resultKey: String(resultKey),
              targetAgentId: String(parsed?.targetAgentId || agent.config?.target_agent_id || '')
            })
          })
          return
        }

        const result = runResults.value[agent.id]?.[qIdStr]
        if (!result?.error) return
        if (result.loading || result.queued) return
        targets.push({
          agentId: agent.id,
          questionId: qIdStr,
          resultKey: qIdStr,
          targetAgentId: ''
        })
      })
    })

    return targets
  }

  function getQuestionEvaluatorTargets(questionId) {
    if (!currentRun.value || !questionId) return []

    const qIdStr = String(questionId)
    const evaluatorAgents = getRetryEligibleAgents('evaluator')
    if (evaluatorAgents.length === 0) return []

    const primaryCandidates = []
    getRetryEligibleAgents('primary').forEach((agent) => {
      const result = runResults.value[agent.id]?.[qIdStr]
      if (!result?.answer) return
      if (result.error || result.loading || result.queued) return
      primaryCandidates.push({
        agentId: agent.id,
        questionId: qIdStr,
        resultId: result.id || ''
      })
    })

    if (primaryCandidates.length === 0) return []

    const targets = []
    evaluatorAgents.forEach((agent) => {
      const configuredTargetAgentId = String(agent.config?.target_agent_id || '')
      const applicableCandidates = configuredTargetAgentId
        ? primaryCandidates.filter((candidate) => candidate.agentId === configuredTargetAgentId)
        : primaryCandidates

      applicableCandidates.forEach((candidate) => {
        const canonicalResultKey = `eval-${candidate.agentId}-${qIdStr}`
        const existingEvalResult = runResults.value[agent.id]?.[canonicalResultKey]
        targets.push({
          agentId: agent.id,
          questionId: qIdStr,
          resultKey: canonicalResultKey,
          targetAgentId: candidate.agentId,
          resultId: existingEvalResult?.id || ''
        })
      })
    })

    const deduped = new Map()
    targets.forEach((target) => {
      deduped.set(`${target.agentId}::${target.resultKey}`, target)
    })

    return [...deduped.values()]
  }

  async function retryQuestionForAllAgents(questionId) {
    if (!currentRun.value || !questionId) {
      showAlert('No active run. Please start a benchmark first.')
      return
    }

    const qIdStr = String(questionId)
    const enabledAgents = getRetryEligibleAgents('primary')

    if (enabledAgents.length === 0) {
      showAlert('No enabled agents found. Please enable at least one agent.')
      return
    }

    const localRetryIds = {}
    enabledAgents.forEach((agent) => {
      const localRetryId = `local-${agent.id}-${Date.now()}`
      localRetryIds[agent.id] = localRetryId
      markLocalRetryPending(agent.id, qIdStr, localRetryId, { resultKey: qIdStr })
    })

    for (const agent of enabledAgents) {
      await rerunQuestion(agent.id, questionId, localRetryIds[agent.id])
    }
  }

  async function retryQuestionForEvaluators(questionId) {
    if (!currentRun.value || !questionId) {
      showAlert('No active run. Please start a benchmark first.')
      return
    }

    const evaluatorTargets = getQuestionEvaluatorTargets(questionId)
    if (evaluatorTargets.length === 0) {
      showAlert('No evaluator targets available for this question.')
      return
    }

    const batchStamp = Date.now()
    const localRetryIds = new Map()

    evaluatorTargets.forEach((target, index) => {
      const localRetryId = `local-eval-${target.agentId}-${target.resultKey}-${batchStamp}-${index}`
      localRetryIds.set(`${target.agentId}::${target.resultKey}`, localRetryId)
      markLocalRetryPending(target.agentId, target.questionId, localRetryId, {
        resultKey: target.resultKey
      })
    })

    for (const target of evaluatorTargets) {
      const localRetryId = localRetryIds.get(`${target.agentId}::${target.resultKey}`)
      await rerunQuestion(target.agentId, target.questionId, localRetryId, {
        resultKey: target.resultKey,
        targetAgentId: target.targetAgentId
      })
    }
  }

  async function retryFailedResults(kind = 'primary') {
    if (!currentRun.value) {
      showAlert('No active run. Please start a benchmark first.')
      return
    }

    const failedTargets = getFailedRetryTargets(kind)
    if (failedTargets.length === 0) {
      if (kind === 'evaluator') {
        showAlert('No failed evaluator results found in the current run.')
      } else {
        showAlert('No failed primary results found in the current run.')
      }
      return
    }

    const batchStamp = Date.now()
    const localRetryIds = new Map()

    failedTargets.forEach((target, index) => {
      const localRetryId = `local-${target.agentId}-${target.resultKey}-${batchStamp}-${index}`
      const targetKey = `${target.agentId}::${target.resultKey}::${target.targetAgentId || ''}`
      localRetryIds.set(targetKey, localRetryId)
      markLocalRetryPending(target.agentId, target.questionId, localRetryId, {
        resultKey: target.resultKey
      })
    })

    for (const target of failedTargets) {
      const targetKey = `${target.agentId}::${target.resultKey}::${target.targetAgentId || ''}`
      const localRetryId = localRetryIds.get(targetKey)
      await rerunQuestion(target.agentId, target.questionId, localRetryId, {
        resultKey: target.resultKey,
        targetAgentId: target.targetAgentId
      })
    }
  }

  async function retryFailedPrimaryResults() {
    await retryFailedResults('primary')
  }

  async function retryFailedEvaluatorResults() {
    await retryFailedResults('evaluator')
  }

  // Returns targets that still need a run but are NOT outright failures — those
  // are handled separately by getFailedRetryTargets ("Retry Failed"). Keeping the
  // two sets disjoint is what makes "Retry Missing" and "Retry Failed" non-overlapping.
  // For primary: question/agent combos with no result yet (never run).
  // For evaluator: questions with a primary answer but no evaluator result, plus
  // stale evaluations (the eval points at a superseded primary answer).
  function getIncompleteRetryTargets(kind = 'primary') {
    if (!currentRun.value) return []

    const flatQuestions = typeof getFlatQuestions === 'function' ? getFlatQuestions() : []
    const enabledAgents = getRetryEligibleAgents(kind)
    const targets = []

    if (kind !== 'evaluator') {
      flatQuestions.forEach((question) => {
        const qIdStr = String(question.id)
        enabledAgents.forEach((agent) => {
          if (isEvaluatorAgentObject(agent)) return
          const result = runResults.value[agent.id]?.[qIdStr]
          if (result && !result.error) return  // completed successfully — skip
          if (result?.error) return  // errored — handled by "Retry Failed", not here
          if (result?.loading || result?.queued) return  // in-flight — skip
          targets.push({ agentId: agent.id, questionId: qIdStr, resultKey: qIdStr, targetAgentId: '' })
        })
      })
      return targets
    }

    // Evaluator: questions with primary answer but missing or errored evaluator result
    const primaryAgents = getRetryEligibleAgents('primary')
    const deduped = new Map()

    enabledAgents.forEach((agent) => {
      if (!isEvaluatorAgentObject(agent)) return
      const configuredTargetAgentId = String(agent.config?.target_agent_id || '')
      const agentResults = runResults.value[agent.id] || {}

      flatQuestions.forEach((question) => {
        const qIdStr = String(question.id)

        const applicablePrimaries = primaryAgents.filter((primary) => {
          if (isEvaluatorAgentObject(primary)) return false
          if (configuredTargetAgentId && primary.id !== configuredTargetAgentId) return false
          const primaryResult = runResults.value[primary.id]?.[qIdStr]
          return primaryResult?.answer && !primaryResult.error && !primaryResult.loading
        })

        applicablePrimaries.forEach((primary) => {
          const canonicalResultKey = `eval-${primary.id}-${qIdStr}`
          const existingEvalResult = agentResults[canonicalResultKey]
          if (existingEvalResult && !existingEvalResult.error) {
            // Only skip if the eval actually targets the *current* primary result.
            // After a primary-answer retry the primary result gets a new ID; if
            // targetRunResultId is known and doesn't match, the eval is stale and
            // must be re-run (auto-eval won't trigger because evalResultCount > 0).
            const knownTarget = existingEvalResult.targetRunResultId
            const currentPrimary = runResults.value[primary.id]?.[qIdStr]?.id
            if (!knownTarget || !currentPrimary || knownTarget === currentPrimary) {
              return  // eval done and up-to-date (or staleness is undetectable) — skip
            }
            // fall through: eval is stale
          }
          if (existingEvalResult?.loading || existingEvalResult?.queued) return  // in-flight — skip
          if (existingEvalResult?.error) return  // errored — handled by "Retry Failed", not here
          deduped.set(`${agent.id}::${canonicalResultKey}`, {
            agentId: agent.id,
            questionId: qIdStr,
            resultKey: canonicalResultKey,
            targetAgentId: primary.id
          })
        })
      })
    })

    return [...deduped.values()]
  }

  async function retryIncompleteResults(kind = 'primary') {
    if (!currentRun.value) {
      showAlert('No active run. Please start a benchmark first.')
      return
    }

    const targets = getIncompleteRetryTargets(kind)
    if (targets.length === 0) {
      showAlert(kind === 'evaluator'
        ? 'No missing evaluator results found.'
        : 'No missing results found.')
      return
    }

    const batchStamp = Date.now()
    const localRetryIds = new Map()

    targets.forEach((target, index) => {
      const localRetryId = `local-${target.agentId}-${target.resultKey}-${batchStamp}-${index}`
      const targetKey = `${target.agentId}::${target.resultKey}::${target.targetAgentId || ''}`
      localRetryIds.set(targetKey, localRetryId)
      markLocalRetryPending(target.agentId, target.questionId, localRetryId, { resultKey: target.resultKey })
    })

    for (const target of targets) {
      const targetKey = `${target.agentId}::${target.resultKey}::${target.targetAgentId || ''}`
      const localRetryId = localRetryIds.get(targetKey)
      await rerunQuestion(target.agentId, target.questionId, localRetryId, {
        resultKey: target.resultKey,
        targetAgentId: target.targetAgentId
      })
    }
  }

  async function retryIncompletePrimaryResults() {
    await retryIncompleteResults('primary')
  }

  async function retryIncompleteEvaluatorResults() {
    await retryIncompleteResults('evaluator')
  }

  // Retry an explicit, pre-computed list of targets (same mechanics as
  // retryIncompleteResults but works with any externally-filtered target set).
  // Used by "Retry Filtered" when the UI question filter is active.
  async function retrySpecificTargets(targets) {
    if (!currentRun.value || !targets || targets.length === 0) return

    const batchStamp = Date.now()
    const localRetryIds = new Map()

    targets.forEach((target, index) => {
      const localRetryId = `local-filtered-${target.agentId}-${target.resultKey}-${batchStamp}-${index}`
      const targetKey = `${target.agentId}::${target.resultKey}::${target.targetAgentId || ''}`
      localRetryIds.set(targetKey, localRetryId)
      markLocalRetryPending(target.agentId, target.questionId, localRetryId, { resultKey: target.resultKey })
    })

    for (const target of targets) {
      const targetKey = `${target.agentId}::${target.resultKey}::${target.targetAgentId || ''}`
      const localRetryId = localRetryIds.get(targetKey)
      await rerunQuestion(target.agentId, target.questionId, localRetryId, {
        resultKey: target.resultKey,
        targetAgentId: target.targetAgentId
      })
    }
  }

  // Build retry targets for an explicit set of question IDs regardless of
  // whether those questions already have results (force-retry). Used when the
  // user has applied a question filter and wants to re-run everything visible.
  //
  //   kind='primary'   → one target per (primaryAgent × question) — skips in-flight only
  //   kind='evaluator' → uses getQuestionEvaluatorTargets per question — skips in-flight only
  function getAllRetryTargetsForQuestions(questionIds, kind = 'primary') {
    if (!currentRun.value || !questionIds || questionIds.length === 0) return []
    const qIdSet = new Set(questionIds.map(String))
    const targets = []

    if (kind === 'evaluator') {
      qIdSet.forEach((qIdStr) => {
        const qTargets = getQuestionEvaluatorTargets(qIdStr)
        qTargets.forEach((t) => {
          const existing = runResults.value[t.agentId]?.[t.resultKey]
          if (existing?.loading || existing?.queued) return
          targets.push(t)
        })
      })
      return targets
    }

    const primaryAgents = getRetryEligibleAgents('primary')
    qIdSet.forEach((qIdStr) => {
      primaryAgents.forEach((agent) => {
        const result = runResults.value[agent.id]?.[qIdStr]
        if (result?.loading || result?.queued) return
        targets.push({ agentId: agent.id, questionId: qIdStr, resultKey: qIdStr, targetAgentId: '' })
      })
    })
    return targets
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
    retryQuestionForEvaluators,
    retryFailedPrimaryResults,
    retryFailedEvaluatorResults,
    getFailedRetryTargets,
    getIncompleteRetryTargets,
    retryIncompletePrimaryResults,
    retryIncompleteEvaluatorResults,
    retrySpecificTargets,
    getAllRetryTargetsForQuestions,
    getQuestionEvaluatorTargets,
    onValidation,
    onRetry
  }
}
