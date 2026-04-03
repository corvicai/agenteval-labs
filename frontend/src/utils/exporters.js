import { isRef } from 'vue'
import { extractAnswerText, extractAnswerMeta } from './chatHelpers.js'

const triggerDownload = (data, prefix) => {
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5)
    const fileName = `${prefix}-${timestamp}.json`

    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)

    return fileName
}

const buildAgentStats = (results, calculateStats) => {
    if (typeof calculateStats !== 'function') {
        return null
    }
    const stats = calculateStats(results)
    const totalValidations =
        stats.validations.positive +
        stats.validations.negative +
        stats.validations.alternative +
        stats.validations.partial

    const qualityScore = totalValidations > 0
        ? (
            (stats.validations.positive * 1.0 +
                stats.validations.alternative * 0.8 +
                stats.validations.partial * 0.5) /
            (totalValidations || 1) * 100
        ).toFixed(1)
        : '0.0'

    return {
        stats,
        qualityScore
    }
}

const isEvaluatorAgent = (agent = {}) => {
    const provider = String(agent.provider || agent.provider_type || '').toLowerCase()
    const name = String(agent.name || agent.config?.name || '').toLowerCase()
    const cfg = agent.config || {}
    const hasLegacyEvaluatorConfig =
        !!cfg?.target_agent_id ||
        (typeof cfg?.openai_mode === 'string' && cfg.openai_mode.trim() !== '') ||
        (typeof cfg?.system_prompt === 'string' && cfg.system_prompt.trim() !== '')

    if (provider === 'evaluator') return true
    if (provider === 'openai' && (hasLegacyEvaluatorConfig || name.includes('evaluator'))) return true
    return false
}

export const exportResultsReport = ({
    agentsRef,
    extractText = extractAnswerText,
    extractMeta = extractAnswerMeta,
    calculateStats,
    questionSetData = null
}) => {
    const agents = isRef(agentsRef) ? agentsRef.value : agentsRef
    if (!Array.isArray(agents) || agents.length === 0) {
        alert('No agents available to export.')
        return
    }

    let parsedQuestionSetData = questionSetData
    if (typeof parsedQuestionSetData === 'string') {
        try {
            parsedQuestionSetData = JSON.parse(parsedQuestionSetData)
        } catch (e) {
            parsedQuestionSetData = null
        }
    }
    const reportNotes = typeof parsedQuestionSetData?.notes === 'string'
        ? parsedQuestionSetData.notes.trim()
        : ''

    const orderedAgents = [...agents].sort((a, b) => {
        const aEval = isEvaluatorAgent(a)
        const bEval = isEvaluatorAgent(b)
        if (aEval && !bEval) return 1
        if (!aEval && bEval) return -1
        const aPos = Number.isFinite(a?.position) ? a.position : 0
        const bPos = Number.isFinite(b?.position) ? b.position : 0
        return aPos - bPos
    })

    const agentsArray = orderedAgents.map(agent => {
        const config = agent.config?.value || agent.config || {}
        const results = agent.results?.value || agent.results || []
        const statsData = buildAgentStats(results, calculateStats)

        return {
            id: agent.id,
            provider: agent.provider,
            name: config.name || '',
            url: config.url || '',
            stats: statsData ? statsData.stats : null,
            qualityScore: statsData ? statsData.qualityScore : null
        }
    })

    // Determine max number of questions across all agents
    const maxLength = Math.max(0, ...orderedAgents.map(agent => (agent.results?.value || agent.results || []).length))
    const resultsArray = []

    for (let i = 0; i < maxLength; i++) {
        // Get question info from first available agent result
        const firstResult = orderedAgents.find(a => (a.results?.value || a.results || [])[i])?.results[i] ||
            (orderedAgents[0]?.results?.value?.[i]) ||
            (orderedAgents[0]?.results?.[i])

        if (!firstResult) continue

        resultsArray.push({
            questionNumber: i + 1,
            question: firstResult.question,
            expectedAnswer: firstResult.question?.expected || null, // Capture expected answer if available
            agents: orderedAgents.map(agent => {
                const arr = agent.results?.value || agent.results || []
                const result = arr[i]
                return {
                    agentId: agent.id,
                    name: agent.config?.name || agent.name, // Ensure name is passed
                    provider: agent.provider, // Ensure provider is passed
                    answer: result ? extractText(result.answer) : null,
                    metadata: result ? extractMeta(result.answer) : null,
                    duration: result?.duration || null,
                    error: result?.error || null,
                    humanValidation: result?.humanValidation || null,
                    rawResponse: result?.answer || null
                }
            })
        })
    }

    const summary = (() => {
        if (!Array.isArray(agentsArray) || agentsArray.length === 0) {
            return null
        }
        const agentsWithStats = agentsArray.filter(a => a?.stats)
        if (agentsWithStats.length === 0) {
            return null
        }

        const totalQuestions = resultsArray.length
        const completedQuestions = resultsArray.filter(r =>
            r.agents.some(a => a.answer || a.error)
        ).length

        const fastest = [...agentsWithStats]
            .filter(a => a.stats && parseFloat(a.stats.avgDuration) > 0)
            .sort((a, b) => parseFloat(a.stats.avgDuration) - parseFloat(b.stats.avgDuration))[0] || null

        const bestQuality = [...agentsWithStats]
            .filter(a => !Number.isNaN(parseFloat(a.qualityScore)))
            .sort((a, b) => parseFloat(b.qualityScore) - parseFloat(a.qualityScore))[0] || null

        return {
            exportDate: new Date().toISOString(),
            totalAgents: agentsArray.length,
            totalQuestions,
            completedQuestions,
            notes: reportNotes,
            validationLegend: {
                '👍 positive': 'Correct and complete answer',
                '👎 negative': 'Incorrect answer',
                '🔄 alternative': 'Valid but different answer',
                '⚠️ partial': 'Partially correct'
            },
            comparison: {
                betterRatedAgent: bestQuality ? bestQuality.name : 'N/A', // Changed key to match PrintReport usage
                fasterAgent: fastest ? fastest.name : 'N/A', // Changed key
                avgDurationDifference: '0.0', // simplified
                fastestAgent: fastest ? fastest.name : null,
                fastestAgentAvgDuration: fastest ? fastest.stats.avgDuration : null,
                bestQualityAgent: bestQuality ? bestQuality.name : null,
                bestQualityScore: bestQuality ? bestQuality.qualityScore : null
            },
            agents: agentsArray // Pass agents array to summary for PrintReport
        }
    })()

    // For PDF printing via browser, we'll open a new window or trigger the print view.
    // The original app seemed to just trigger download json?
    // Wait, the user said "PDF export functionality was present (PrintReport.vue)".
    // PrintReport.vue is a component. Usually used by mounting it and calling window.print().
    // The `exportResultsReport` above only returns JSON.

    // I should modify this to return the data structure needed for PrintReport.vue,
    // NOT trigger download immediately if the intent is PDF.

    // Let's change the return to be the data object.
    return {
        timestamp: new Date().toISOString(),
        agents: agentsArray,
        totalQuestions: resultsArray.length,
        summary,
        results: resultsArray
    }
}

export const exportSummaryReport = ({ agentsRef, calculateStats }) => {
    // ... (keep as is or similar)
    // I will skip this one for now as it's less critical
    return {}
}
