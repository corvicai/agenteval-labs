export function isLegacyEvaluatorConfig(config) {
  if (!config || typeof config !== 'object') return false
  const hasTargetField = Object.prototype.hasOwnProperty.call(config, 'target_agent_id')
  const hasOpenAIMode = typeof config.openai_mode === 'string' && config.openai_mode.trim() !== ''
  const hasSystemPrompt = typeof config.system_prompt === 'string' && config.system_prompt.trim() !== ''
  return hasTargetField || hasOpenAIMode || hasSystemPrompt
}

export function isEvaluatorAgentObject(agent) {
  if (!agent) return false
  if (agent.provider_type === 'evaluator') return true
  if (agent.provider_type !== 'openai') return false
  if (isLegacyEvaluatorConfig(agent.config || {})) return true
  const name = String(agent.name || '').toLowerCase()
  return name.includes('evaluator')
}

export function toAgentID(entry) {
  if (!entry || typeof entry !== 'object') return ''
  return String(entry.agent_id || entry.agentID || entry.id || '')
}

export function uniqueStringIDs(ids = []) {
  return [...new Set((ids || []).map((id) => String(id)).filter(Boolean))]
}

export function mergeAgentIDs(baseIDs = [], extraIDs = []) {
  return uniqueStringIDs([...(baseIDs || []), ...(extraIDs || [])])
}
