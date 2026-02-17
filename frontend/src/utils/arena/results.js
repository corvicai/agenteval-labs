const HUMAN_VALIDATION_MAP = {
  like: 'positive',
  dislike: 'negative',
  valid: 'alternative',
  wrong: 'partial'
}

export function getAgentResults({
  runResults,
  agentId,
  includeAllQuestions = false,
  selectedQuestionId,
  flatQuestions,
  isEvaluatorAgentID,
  isRunning,
  activeRunQuestionSetId,
  currentQuestionSetId,
  currentRunAgentIds,
  taskProgress
}) {
  const results = runResults?.[agentId] || {}
  const isEvaluator = isEvaluatorAgentID(agentId)

  const targetQuestions = (includeAllQuestions || !selectedQuestionId)
    ? flatQuestions
    : flatQuestions.filter((q) => String(q.id) === String(selectedQuestionId))

  const isAgentRunning = isRunning &&
    activeRunQuestionSetId === currentQuestionSetId &&
    (currentRunAgentIds || []).includes(agentId)

  return targetQuestions.map((q) => {
    const qIdStr = String(q.id)
    let res = results[qIdStr]
    if (!res && isEvaluator) {
      const evalKey = Object.keys(results).find((key) => String(key).endsWith(`-${qIdStr}`))
      if (evalKey) {
        res = results[evalKey]
      }
    }

    return {
      question: q,
      answer: res ? res.answer : null,
      loading: res ? res.loading : isAgentRunning,
      queued: res ? res.queued : false,
      duration: res ? res.duration : null,
      timestamp: res ? res.timestamp : null,
      id: res ? res.id : null,
      error: res ? res.error : null,
      metadata: res ? res.metadata : null,
      progress: taskProgress?.[agentId]?.[qIdStr] || null,
      evaluations: res ? (res.evaluations || []) : [],
      humanValidation: res ? (HUMAN_VALIDATION_MAP[res.humanValidation] || res.humanValidation) : null
    }
  })
}

export function collectResultIDsForQuestion(runResults, questionId) {
  if (!runResults) return []
  const ids = []
  const qIdStr = String(questionId)
  for (const agentId in runResults) {
    const result = runResults[agentId]?.[qIdStr]
    if (result?.id) ids.push(result.id)
  }
  return ids
}
