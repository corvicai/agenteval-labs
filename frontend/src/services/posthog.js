import posthog from 'posthog-js'

import { config } from '../config.js'
import { isLocalhost } from '../utils/runtime.js'

const POSTHOG_DEFAULTS = '2026-01-30'

let initialized = false
let appLoadCaptured = false

function parseBoolean(value) {
    if (value == null) return null
    const normalized = String(value).trim().toLowerCase()
    if (!normalized) return null
    if (['1', 'true', 'yes', 'on'].includes(normalized)) return true
    if (['0', 'false', 'no', 'off'].includes(normalized)) return false
    return null
}

function getHostname() {
    if (typeof window === 'undefined') return ''
    return String(window.location?.hostname || '').toLowerCase()
}

function trimString(value) {
    return String(value || '').trim()
}

function compactProperties(properties = {}) {
    return Object.fromEntries(
        Object.entries(properties).filter(([, value]) => value !== undefined && value !== null && value !== '')
    )
}

export function getPostHogEnvironment() {
    const explicitEnvironment = trimString(config.POSTHOG_ENVIRONMENT)
    if (explicitEnvironment) return explicitEnvironment
    if (isLocalhost(getHostname())) return 'local'
    return config.PROD ? 'production' : 'development'
}

export function isPostHogEnabled() {
    if (typeof window === 'undefined') return false
    if (isLocalhost(getHostname())) return false

    const key = trimString(config.POSTHOG_KEY)
    const host = trimString(config.POSTHOG_HOST)
    if (!key || !host) return false

    const explicitEnabled = parseBoolean(config.POSTHOG_ENABLED)
    if (explicitEnabled !== null) return explicitEnabled

    return Boolean(config.PROD)
}

function getSharedProperties() {
    return compactProperties({
        app_environment: getPostHogEnvironment(),
        app_revision: trimString(config.APP_REVISION || config.GIT_COMMIT),
        app_mode: trimString(config.MODE)
    })
}

export function initPostHog() {
    if (initialized || !isPostHogEnabled()) return false

    posthog.init(trimString(config.POSTHOG_KEY), {
        api_host: trimString(config.POSTHOG_HOST),
        defaults: POSTHOG_DEFAULTS,
        person_profiles: 'identified_only',
        autocapture: false,
        capture_pageview: false,
        capture_pageleave: false,
        disable_session_recording: true
    })

    posthog.register(getSharedProperties())
    initialized = true
    return true
}

function ensurePostHog() {
    if (initialized) return true
    return initPostHog()
}

export function identifyPostHogUser({
    userId,
    email,
    name,
    workspaceId,
    workspaceName,
    isAdmin
} = {}) {
    const distinctId = trimString(userId)
    if (!distinctId || !ensurePostHog()) return false

    posthog.identify(distinctId, compactProperties({
        email: trimString(email),
        name: trimString(name),
        workspace_id: trimString(workspaceId),
        workspace_name: trimString(workspaceName),
        is_admin: Boolean(isAdmin),
        ...getSharedProperties()
    }))

    posthog.register(compactProperties({
        user_id: distinctId,
        workspace_id: trimString(workspaceId),
        workspace_name: trimString(workspaceName),
        is_admin: Boolean(isAdmin)
    }))

    const groupKey = trimString(workspaceId)
    if (groupKey) {
        posthog.group('workspace', groupKey, compactProperties({
            name: trimString(workspaceName)
        }))
    }

    return true
}

export function resetPostHogUser() {
    if (!initialized) return
    posthog.reset()
}

export function capturePostHogEvent(eventName, properties = {}) {
    const name = trimString(eventName)
    if (!name || !ensurePostHog()) return false

    posthog.capture(name, compactProperties({
        ...getSharedProperties(),
        ...properties
    }))

    return true
}

export function capturePostHogAppLoaded() {
    if (appLoadCaptured) return false
    const captured = capturePostHogEvent('app_loaded')
    if (captured) {
        appLoadCaptured = true
    }
    return captured
}

export function __resetPostHogForTests() {
    initialized = false
    appLoadCaptured = false
}
