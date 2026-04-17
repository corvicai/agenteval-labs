import { uniqueStringIDs } from './agents.js'

export function splitSelectedAgents(payload = {}, isEvaluatorAgentID) {
  const requested = uniqueStringIDs(payload.agentIds || [])

  let primary = uniqueStringIDs(payload.primaryAgentIds || [])
  let evaluators = uniqueStringIDs(payload.evaluatorAgentIds || [])

  if (primary.length === 0 && evaluators.length === 0 && requested.length > 0) {
    requested.forEach((agentId) => {
      if (isEvaluatorAgentID(agentId)) {
        evaluators.push(agentId)
      } else {
        primary.push(agentId)
      }
    })
  }

  return {
    primary: uniqueStringIDs(primary),
    evaluators: uniqueStringIDs(evaluators)
  }
}

export function getEvaluatorIdsForRun(runLike, isEvaluatorAgentID) {
  return uniqueStringIDs(runLike?.agentIds || []).filter((agentId) => isEvaluatorAgentID(agentId))
}

export function hasEvaluatorResultsLoaded(resultMap, isEvaluatorAgentID) {
  const map = resultMap || {}
  for (const agentId in map) {
    const agentResults = map[agentId] || {}
    if (isEvaluatorAgentID(agentId) && Object.keys(agentResults).length > 0) {
      return true
    }
    for (const questionId in agentResults) {
      if (String(questionId).startsWith('eval-')) {
        return true
      }
    }
  }
  return false
}

export function resolveRunAgentIds(data) {
  const ids = new Set()

  const runAgentIds = data?.run?.agent_ids || data?.run?.agentIds
  if (Array.isArray(runAgentIds)) {
    runAgentIds.forEach((id) => {
      if (id) ids.add(id)
    })
  }

  if (Array.isArray(data?.results)) {
    data.results.forEach((res) => {
      if (res?.agent_id) ids.add(res.agent_id)
    })
  }

  const qsAgents = data?.question_set?.agents
  if (Array.isArray(qsAgents)) {
    qsAgents.forEach((agent) => {
      const id = agent?.agent_id || agent?.id
      const enabled = agent?.enabled
      if (id && enabled !== false) ids.add(id)
    })
  }

  if (data?.agents && typeof data.agents === 'object' && !Array.isArray(data.agents)) {
    Object.keys(data.agents).forEach((id) => {
      if (id) ids.add(id)
    })
  }

  return Array.from(ids).filter(Boolean)
}

// Strict variant for (completed/historical) runs: only sources that reflect
// what actually participated. Intentionally skips data.question_set.agents
// (the CURRENT QS config) so a newly-added agent does not leak into a past
// run's agentIds and mutate history via the retry flow.
export function resolveRunAgentIdsStrict(data) {
  const ids = new Set()

  const runAgentIds = data?.run?.agent_ids || data?.run?.agentIds
  if (Array.isArray(runAgentIds)) {
    runAgentIds.forEach((id) => {
      if (id) ids.add(String(id))
    })
  }

  if (Array.isArray(data?.results)) {
    data.results.forEach((res) => {
      if (res?.agent_id) ids.add(String(res.agent_id))
    })
  }

  if (data?.agents && typeof data.agents === 'object' && !Array.isArray(data.agents)) {
    Object.keys(data.agents).forEach((id) => {
      if (id) ids.add(String(id))
    })
  }

  return Array.from(ids).filter(Boolean)
}

export function resolveRetryStatusItems(response) {
  if (Array.isArray(response?.items)) return response.items
  if (Array.isArray(response)) return response
  return []
}
