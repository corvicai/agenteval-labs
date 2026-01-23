const API_BASE = '/api'

// Helper for fetch with error handling
export async function request(url, options = {}) {
    const headers = {
        'Content-Type': 'application/json',
        ...options.headers
    }

    const response = await fetch(`${API_BASE}${url}`, {
        ...options,
        headers,
        credentials: 'include' // Important for HttpOnly cookies
    })

    if (!response.ok) {
        if (response.status === 401) {
            // Avoid forcing reloads on background refresh calls.
            if (!url.includes('/auth/refresh')) {
                // If we are already on the login page, don't trigger anything to avoid loops
                if (url.includes('/auth/login') || url.includes('/auth/register')) {
                    const error = await response.json().catch(() => ({}))
                    throw new Error(error.error || `Auth failed: ${response.status}`)
                }

                // Prevent multiple concurrent reloads
                if (window.__isReloading) return
                window.__isReloading = true

                // Unauthenticated - clear local state but cookies are handled by browser
                await logout()

                // Final check to avoid double-loading login screen or infinite loops
                window.location.href = '/'
            }
        }
        const error = await response.json().catch(() => ({}))
        throw new Error(error.error || `Request failed: ${response.status}`)
    }

    if (response.status === 204) return null
    return response.json()
}

// Auth
export async function register(name, email, password, organization_name, invite_code, role = 'user') {
    const result = await request('/auth/register', {
        method: 'POST',
        body: JSON.stringify({ name, email, password, organization_name, invite_code, role })
    })
    // Token is now in HttpOnly cookie
    localStorage.setItem('user', JSON.stringify(result.user))
    if (result.workspace) {
        localStorage.setItem('workspace', JSON.stringify(result.workspace))
    }
    return result
}

export async function login(email, password, organizationId = null) {
    const result = await request('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password, organization_id: organizationId })
    })

    // If multiple orgs found or invite code required, return result for UI to handle
    if (result.requires_org_selection || result.requires_invite_code) {
        return result
    }

    // Token is now in HttpOnly cookie
    localStorage.setItem('user', JSON.stringify(result.user))
    if (result.workspace) {
        localStorage.setItem('workspace', JSON.stringify(result.workspace))
    }
    return result
}

export async function joinOrganization(inviteCode) {
    const result = await request('/auth/join-organization', {
        method: 'POST',
        body: JSON.stringify({ invite_code: inviteCode })
    })

    localStorage.setItem('user', JSON.stringify(result.user))
    if (result.workspace) {
        localStorage.setItem('workspace', JSON.stringify(result.workspace))
    }
    return result
}

export async function selectOrganization(orgId) {
    const result = await request('/auth/select-organization', {
        method: 'POST',
        body: JSON.stringify({ org_id: orgId })
    })

    localStorage.setItem('user', JSON.stringify(result.user))
    if (result.workspace) {
        localStorage.setItem('workspace', JSON.stringify(result.workspace))
    }
    return result
}

export async function logout() {
    try {
        await fetch(`${API_BASE}/auth/logout`, { method: 'POST', credentials: 'include' })
    } catch (e) {
        console.warn('Logout request failed', e)
    }
    localStorage.removeItem('user')
    localStorage.removeItem('token') // Clear legacy token
    localStorage.removeItem('impersonation_token')
    localStorage.removeItem('is_impersonating')
    localStorage.removeItem('workspace')
    localStorage.removeItem('activeRunId')
    localStorage.removeItem('lastQuestionSetId')
}

export async function refreshToken() {
    try {
        return await request('/auth/refresh', { method: 'POST' })
    } catch (e) {
        // If refresh fails (e.g. network or expired), strictly don't force logout here
        // as the interval might just retry, or the next real request will fail and trigger logout.
        console.warn('Token refresh failed', e)
        return null
    }
}

// We still keep this for UI checks, but it's now a heuristic based on 'user' object
export function isLoggedIn() {
    return !!localStorage.getItem('user')
}

export function getStoredUser() {
    const user = localStorage.getItem('user')
    return user ? JSON.parse(user) : null
}

export function getStoredWorkspace() {
    const ws = localStorage.getItem('workspace')
    return ws ? JSON.parse(ws) : null
}

export async function bootstrapAdmin(name, email, password, organization_name) {
    const result = await request('/auth/bootstrap-admin', {
        method: 'POST',
        body: JSON.stringify({ name, email, password, organization_name })
    })
    return result
}

export async function checkAdminExists() {
    return await request('/auth/check-admin')
}
