export function getPrimaryResponseEntry({
  runResults,
  questionId,
  mergedAgents,
  isEvaluatorAgentObject,
  isEvaluatorAgentID,
  isQuestionRetrying
}) {
  if (!runResults || !questionId) return null
  const qIdStr = String(questionId)
  if (isQuestionRetrying(qIdStr)) return null

  const orderedPrimaryAgents = (mergedAgents || [])
    .filter((agent) => !isEvaluatorAgentObject(agent))
    .sort((a, b) => (a.position || 0) - (b.position || 0))

  const primaryAgentIDs = orderedPrimaryAgents.map((agent) => agent.id)
  if (primaryAgentIDs.length === 0) {
    for (const agentId in runResults) {
      if (isEvaluatorAgentID(agentId)) continue
      primaryAgentIDs.push(agentId)
    }
  }

  for (const agentId of primaryAgentIDs) {
    const result = runResults[agentId]?.[qIdStr]
    if (result && result.answer && !result.loading && !result.error) {
      return { agentId, answer: result.answer }
    }
  }

  return null
}

export function getQuestionResponse({
  runResults,
  questionId,
  mergedAgents,
  isEvaluatorAgentObject,
  isEvaluatorAgentID,
  isQuestionRetrying,
  truncatePreviewText,
  truncated = true
}) {
  const entry = getPrimaryResponseEntry({
    runResults,
    questionId,
    mergedAgents,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying
  })
  if (!entry) return null
  return truncated ? truncatePreviewText(entry.answer) : entry.answer
}

export function getQuestionEvaluation({
  runResults,
  questionId,
  mergedAgents,
  isEvaluatorAgentObject,
  isEvaluatorAgentID,
  isQuestionRetrying,
  truncatePreviewText,
  truncated = true
}) {
  if (!runResults || !questionId) return null

  const responseEntry = getPrimaryResponseEntry({
    runResults,
    questionId,
    mergedAgents,
    isEvaluatorAgentObject,
    isEvaluatorAgentID,
    isQuestionRetrying
  })
  if (!responseEntry?.agentId) return null

  const qIdStr = String(questionId)
  const expectedEvalKey = `eval-${responseEntry.agentId}-${qIdStr}`

  const evaluatorAgentIDs = (mergedAgents || [])
    .filter((agent) => isEvaluatorAgentObject(agent))
    .sort((a, b) => (a.position || 0) - (b.position || 0))
    .map((agent) => agent.id)

  if (evaluatorAgentIDs.length === 0) {
    for (const agentId in runResults) {
      if (isEvaluatorAgentID(agentId)) evaluatorAgentIDs.push(agentId)
    }
  }

  for (const evaluatorId of evaluatorAgentIDs) {
    const evaluatorResults = runResults[evaluatorId]
    if (!evaluatorResults) continue

    let result = evaluatorResults[expectedEvalKey]
    if (!result) {
      const fallbackKey = Object.keys(evaluatorResults).find((key) => String(key).endsWith(`-${qIdStr}`))
      if (fallbackKey) result = evaluatorResults[fallbackKey]
    }

    if (result && result.answer && !result.loading && !result.error) {
      return truncated ? truncatePreviewText(result.answer) : result.answer
    }
  }

  return null
}
