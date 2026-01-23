/**
 * Formats a duration in seconds to a human-readable string
 * Examples:
 *   0.5 -> "0.5s"
 *   5.2 -> "5.2s"
 *   65 -> "1m 5s"
 *   125 -> "2m 5s"
 *   3665 -> "1h 1m 5s"
 * 
 * @param {number} seconds - Duration in seconds
 * @param {object} options - Formatting options
 * @param {boolean} options.showMs - Show milliseconds for durations < 1s (default: false)
 * @returns {string} Formatted duration string
 */
export function formatDuration(seconds, options = {}) {
    if (seconds === null || seconds === undefined || isNaN(seconds)) {
        return '-'
    }

    const { showMs = false } = options

    // For very small durations, show milliseconds
    if (seconds < 1) {
        if (showMs) {
            return `${Math.round(seconds * 1000)}ms`
        }
        return `${seconds.toFixed(2)}s`
    }

    // For durations less than 60 seconds, show with one decimal
    if (seconds < 60) {
        return `${seconds.toFixed(1)}s`
    }

    // For durations 60 seconds or more, show minutes and seconds
    const hours = Math.floor(seconds / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    const secs = Math.floor(seconds % 60)

    if (hours > 0) {
        return `${hours}h ${minutes}m ${secs}s`
    }

    return `${minutes}m ${secs}s`
}

/**
 * Shorthand for formatting duration from milliseconds
 * @param {number} ms - Duration in milliseconds
 * @returns {string} Formatted duration string
 */
export function formatDurationMs(ms) {
    return formatDuration(ms / 1000)
}
