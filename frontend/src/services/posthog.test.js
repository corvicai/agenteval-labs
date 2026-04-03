import { beforeEach, describe, expect, it, vi } from 'vitest'

const posthogMock = {
    init: vi.fn(),
    register: vi.fn(),
    identify: vi.fn(),
    group: vi.fn(),
    capture: vi.fn(),
    reset: vi.fn()
}

const mockConfig = {
    POSTHOG_KEY: '',
    POSTHOG_HOST: '',
    POSTHOG_ENABLED: '',
    POSTHOG_ENVIRONMENT: '',
    PROD: false,
    MODE: 'development',
    APP_REVISION: 'rev-123',
    GIT_COMMIT: 'rev-123'
}

vi.mock('posthog-js', () => ({
    default: posthogMock
}))

vi.mock('../config.js', () => ({
    config: mockConfig
}))

function setWindowLocation(url) {
    delete window.location
    window.location = new URL(url)
}

async function loadService() {
    const service = await import('./posthog.js')
    service.__resetPostHogForTests()
    return service
}

describe('posthog service', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        mockConfig.POSTHOG_KEY = ''
        mockConfig.POSTHOG_HOST = ''
        mockConfig.POSTHOG_ENABLED = ''
        mockConfig.POSTHOG_ENVIRONMENT = ''
        mockConfig.PROD = false
        mockConfig.MODE = 'development'
        mockConfig.APP_REVISION = 'rev-123'
        mockConfig.GIT_COMMIT = 'rev-123'
    })

    it('never enables analytics on localhost', async () => {
        setWindowLocation('http://localhost:3010/')
        mockConfig.POSTHOG_KEY = 'phc_test'
        mockConfig.POSTHOG_HOST = 'https://us.i.posthog.com'
        mockConfig.POSTHOG_ENABLED = 'true'

        const service = await loadService()

        expect(service.isPostHogEnabled()).toBe(false)
        expect(service.initPostHog()).toBe(false)
        expect(posthogMock.init).not.toHaveBeenCalled()
    })

    it('initializes analytics in production with explicit runtime config', async () => {
        setWindowLocation('https://agenteval.corviclabs.ai/')
        mockConfig.POSTHOG_KEY = 'phc_test'
        mockConfig.POSTHOG_HOST = 'https://us.i.posthog.com'
        mockConfig.PROD = true
        mockConfig.MODE = 'production'

        const service = await loadService()

        expect(service.isPostHogEnabled()).toBe(true)
        expect(service.initPostHog()).toBe(true)
        expect(posthogMock.init).toHaveBeenCalledWith('phc_test', expect.objectContaining({
            api_host: 'https://us.i.posthog.com',
            autocapture: false,
            capture_pageview: false,
            disable_session_recording: true
        }))
        expect(posthogMock.register).toHaveBeenCalledWith(expect.objectContaining({
            app_environment: 'production',
            app_revision: 'rev-123'
        }))
    })

    it('initializes analytics in production with the built-in public fallback', async () => {
        setWindowLocation('https://agenteval.corviclabs.ai/')
        mockConfig.POSTHOG_KEY = 'phc_g5TY15YtOI4fvazYarJmuwTvqEAfl8KDyXh3HFjv0HV'
        mockConfig.POSTHOG_HOST = 'https://us.i.posthog.com'
        mockConfig.PROD = true
        mockConfig.MODE = 'production'

        const service = await loadService()

        expect(service.isPostHogEnabled()).toBe(true)
        expect(service.initPostHog()).toBe(true)
        expect(posthogMock.init).toHaveBeenCalledWith(
            'phc_g5TY15YtOI4fvazYarJmuwTvqEAfl8KDyXh3HFjv0HV',
            expect.objectContaining({ api_host: 'https://us.i.posthog.com' })
        )
    })

    it('identifies the current user and workspace context', async () => {
        setWindowLocation('https://agenteval.corviclabs.ai/')
        mockConfig.POSTHOG_KEY = 'phc_test'
        mockConfig.POSTHOG_HOST = 'https://us.i.posthog.com'
        mockConfig.PROD = true

        const service = await loadService()

        service.identifyPostHogUser({
            userId: 'user-1',
            email: 'user@example.com',
            name: 'User',
            workspaceId: 'ws-1',
            workspaceName: 'Main',
            isAdmin: true
        })

        expect(posthogMock.identify).toHaveBeenCalledWith('user-1', expect.objectContaining({
            email: 'user@example.com',
            workspace_id: 'ws-1',
            is_admin: true
        }))
        expect(posthogMock.group).toHaveBeenCalledWith('workspace', 'ws-1', { name: 'Main' })
    })

    it('captures explicit frontend events with shared app properties', async () => {
        setWindowLocation('https://agenteval.corviclabs.ai/')
        mockConfig.POSTHOG_KEY = 'phc_test'
        mockConfig.POSTHOG_HOST = 'https://us.i.posthog.com'
        mockConfig.PROD = true

        const service = await loadService()

        service.capturePostHogEvent('workspace_switched', { workspace_id: 'ws-1' })

        expect(posthogMock.capture).toHaveBeenCalledWith('workspace_switched', expect.objectContaining({
            workspace_id: 'ws-1',
            app_environment: 'production',
            app_revision: 'rev-123'
        }))
    })
})
