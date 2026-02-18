export function calculateStats(results) {
  if (!results || !Array.isArray(results)) return {}

  let answered = 0
  let errors = 0
  const total = results.length
  let totalDuration = 0
  const validations = { positive: 0, negative: 0, alternative: 0, partial: 0, notEvaluated: 0 }

  results.forEach((r) => {
    const hasAnswer = r.answer
    if (hasAnswer) answered++
    if (r.error) errors++
    if (r.duration) totalDuration += parseFloat(r.duration) || 0

    // Only count validations for questions that have been answered.
    if (hasAnswer) {
      if (r.humanValidation) {
        const v = r.humanValidation.toLowerCase()
        validations[v] = (validations[v] || 0) + 1
      } else {
        validations.notEvaluated++
      }
    }
  })

  const totalValidations = validations.positive + validations.negative + validations.alternative + validations.partial + validations.notEvaluated

  return {
    answered,
    totalQuestions: total,
    errors,
    avgDuration: answered ? (totalDuration / answered).toFixed(2) : 0,
    validations,
    percentages: {
      positive: totalValidations > 0 ? Math.round((validations.positive || 0) / totalValidations * 100) : 0,
      negative: totalValidations > 0 ? Math.round((validations.negative || 0) / totalValidations * 100) : 0,
      alternative: totalValidations > 0 ? Math.round((validations.alternative || 0) / totalValidations * 100) : 0,
      partial: totalValidations > 0 ? Math.round((validations.partial || 0) / totalValidations * 100) : 0
    }
  }
}

export function calculateAverageEvaluationScore(results) {
  if (!Array.isArray(results) || results.length === 0) {
    return { count: 0, avgScore10: 0 }
  }

  let total = 0
  let count = 0

  results.forEach((item) => {
    const evals = Array.isArray(item?.evaluations) ? item.evaluations : []
    if (evals.length === 0) return

    const userEval = evals.find((ev) => ev?.rater_type === 'user' && ev?.score !== null && ev?.score !== undefined)
    const agentEval = evals.find((ev) => ev?.rater_type === 'agent' && ev?.score !== null && ev?.score !== undefined)
    const selected = userEval || agentEval
    if (!selected) return

    const score = Number(selected.score)
    if (!Number.isFinite(score)) return

    total += score
    count += 1
  })

  if (count === 0) return { count: 0, avgScore10: 0 }
  return { count, avgScore10: (total / count) / 10 }
}

export function formatDuration(value) {
  const seconds = parseFloat(value)
  if (Number.isFinite(seconds)) {
    return seconds >= 60 ? `${(seconds / 60).toFixed(1)} min` : `${seconds.toFixed(1)} s`
  }
  return '0 s'
}
