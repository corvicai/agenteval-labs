// Stateless helpers for chat-corvic

// Extract plain text answer from MCP result
export function extractAnswerText(answer) {
    try {
        if (typeof answer === 'string') {
            return answer
        }

        if (answer?.content && Array.isArray(answer.content)) {
            let resultText = ''

            // Extract text content
            const textContent = answer.content.find(c => c.type === 'text')
            if (textContent?.text) {
                const text = textContent.text
                // Capture everything from ****Answer**** to the end, so we keep visual context and images
                const answerMatch = text.match(/\*\*\*\*Answer\*\*\*\*\s*:\s*([\s\S]*)$/)
                resultText = answerMatch ? answerMatch[1].trim() : text
            }

            // Extract and append images if present
            const imageContents = answer.content.filter(c => c.type === 'image')
            if (imageContents.length > 0) {
                imageContents.forEach((img) => {
                    // MCP image format: { type: 'image', data: 'base64...', mimeType: 'image/png' }
                    if (img.data) {
                        const mimeType = img.mimeType || 'image/png'
                        // Convert to our inline format: (data:image/TYPE;base64,DATA)
                        const inlineImage = `(data:${mimeType};base64,${img.data})`
                        // Append image to text with newlines
                        resultText += '\n\n' + inlineImage
                    }
                })
            }

            if (resultText) {
                return resultText
            }
        }

        return JSON.stringify(answer, null, 2)
    } catch (e) {
        return 'Error parsing answer'
    }
}

// Extract ONLY text without images - for evaluation purposes
export function extractTextOnly(answer) {
    try {
        if (typeof answer === 'string') {
            // Remove any inline base64 images from string
            return answer.replace(/\(data:image\/[^;]+;base64,[^)]+\)/g, '[Image removed]').trim()
        }

        if (answer?.content && Array.isArray(answer.content)) {
            // Only get text content, ignore images completely
            const textContent = answer.content.find(c => c.type === 'text')
            if (textContent?.text) {
                const text = textContent.text
                // Capture everything from ****Answer**** to the end
                const answerMatch = text.match(/\*\*\*\*Answer\*\*\*\*\s*:\s*([\s\S]*)$/)
                let resultText = answerMatch ? answerMatch[1].trim() : text

                // Remove any inline base64 images that might be in the text
                resultText = resultText.replace(/\(data:image\/[^;]+;base64,[^)]+\)/g, '[Image removed]').trim()

                return resultText
            }
        }

        // For other formats, stringify but remove any base64 image data
        const jsonStr = JSON.stringify(answer, null, 2)
        return jsonStr.replace(/"data":\s*"[A-Za-z0-9+/=]{100,}"/g, '"data": "[Image data removed]"')
    } catch (e) {
        return 'Error parsing answer'
    }
}

// Extract metadata (title/document) from MCP result
export function extractAnswerMeta(answer) {
    try {
        if (answer?.content && Array.isArray(answer.content)) {
            const textContent = answer.content.find(c => c.type === 'text')
            if (textContent?.text) {
                const text = textContent.text
                const titleMatch = text.match(/\*\*\*\*Content Title\*\*\*\*\s*:\s*([^\n]+)/)
                const docMatch = text.match(/\*\*\*\*Document Name\*\*\*\*\s*:\s*([^\n]+)/)

                if (titleMatch || docMatch) {
                    return {
                        title: titleMatch ? titleMatch[1].trim() : null,
                        document: docMatch ? docMatch[1].trim() : null
                    }
                }
            }
        }
        return null
    } catch (e) {
        return null
    }
}

// Compute statistics for a list of results
export function calculateStats(results) {
    const validResults = results.filter(r => r.answer && !r.error)
    const withDuration = results.filter(r => r.duration)

    const totalDuration = withDuration.reduce((sum, r) => sum + parseFloat(r.duration || 0), 0)
    const avgDuration = withDuration.length > 0 ? (totalDuration / withDuration.length).toFixed(1) : 0

    const validations = {
        positive: results.filter(r => r.humanValidation === 'positive').length,
        negative: results.filter(r => r.humanValidation === 'negative').length,
        alternative: results.filter(r => r.humanValidation === 'alternative').length,
        partial: results.filter(r => r.humanValidation === 'partial').length,
        notEvaluated: results.filter(r => !r.humanValidation && r.answer).length
    }

    const totalValidations = validations.positive + validations.negative + validations.alternative + validations.partial
    const percentages = totalValidations > 0 ? {
        positive: ((validations.positive / totalValidations) * 100).toFixed(1),
        negative: ((validations.negative / totalValidations) * 100).toFixed(1),
        alternative: ((validations.alternative / totalValidations) * 100).toFixed(1),
        partial: ((validations.partial / totalValidations) * 100).toFixed(1)
    } : { positive: 0, negative: 0, alternative: 0, partial: 0 }

    return {
        totalQuestions: results.length,
        answered: validResults.length,
        errors: results.filter(r => r.error).length,
        totalDuration: totalDuration.toFixed(1),
        avgDuration,
        minDuration: withDuration.length > 0 ? Math.min(...withDuration.map(r => parseFloat(r.duration))).toFixed(1) : 0,
        maxDuration: withDuration.length > 0 ? Math.max(...withDuration.map(r => parseFloat(r.duration))).toFixed(1) : 0,
        validations,
        percentages
    }
}

// Format a timestamp into a human-friendly date/time
export function formatFullDateTime(timestamp) {
    if (!timestamp) return ''
    const date = new Date(timestamp)
    return date.toLocaleString('en-US', {
        dateStyle: 'medium',
        timeStyle: 'short'
    })
}
