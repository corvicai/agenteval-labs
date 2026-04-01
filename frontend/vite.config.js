import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'
import { execSync } from 'node:child_process'

// Prefer repo root .env when present; fall back to frontend/.env in container builds.
const configDir = fileURLToPath(new URL('.', import.meta.url))
const parentDir = path.resolve(configDir, '..')
const envDir = fs.existsSync(path.join(parentDir, '.env')) ? parentDir : configDir

function resolveGitCommitHash() {
    const explicitCommit =
        process.env.VITE_APP_REVISION?.trim() ||
        process.env.APP_REVISION?.trim() ||
        process.env.VITE_GIT_COMMIT?.trim() ||
        process.env.GIT_COMMIT?.trim()
    if (explicitCommit) {
        return explicitCommit
    }

    try {
        return execSync('git rev-parse --short HEAD', {
            cwd: parentDir,
            stdio: ['ignore', 'pipe', 'ignore']
        })
            .toString()
            .trim()
    } catch {
        return 'unknown'
    }
}

export default defineConfig(({ mode }) => {
    const env = loadEnv(mode, envDir, '')
    env.VITE_APP_REVISION =
        env.VITE_APP_REVISION ||
        process.env.VITE_APP_REVISION ||
        process.env.APP_REVISION ||
        env.VITE_GIT_COMMIT ||
        process.env.VITE_GIT_COMMIT ||
        process.env.GIT_COMMIT ||
        resolveGitCommitHash()
    env.VITE_APP_REVISION_BRANCH =
        env.VITE_APP_REVISION_BRANCH ||
        process.env.VITE_APP_REVISION_BRANCH ||
        process.env.APP_REVISION_BRANCH ||
        ''
    env.VITE_APP_REVISION_DIRTY =
        env.VITE_APP_REVISION_DIRTY ||
        process.env.VITE_APP_REVISION_DIRTY ||
        process.env.APP_REVISION_DIRTY ||
        ''
    env.VITE_APP_REVISION_UPDATED_AT =
        env.VITE_APP_REVISION_UPDATED_AT ||
        process.env.VITE_APP_REVISION_UPDATED_AT ||
        process.env.APP_REVISION_UPDATED_AT ||
        ''
    env.VITE_GIT_COMMIT = env.VITE_GIT_COMMIT || resolveGitCommitHash()

    // Explicitly expose VITE_ variables to the client
    // This is a robust fallback if auto-detection fails
    const processEnv = {}
    for (const key in env) {
        if (key.startsWith('VITE_')) {
            processEnv[`import.meta.env.${key}`] = JSON.stringify(env[key])
        }
    }
    const disableHmr = env.VITE_DISABLE_HMR === '1'
    const apiUrl = env.VITE_API_URL || 'http://go-api:8080'
    let hmrHost = env.VITE_HMR_HOST || undefined

    // Intelligent HMR Inference:
    if (!hmrHost && apiUrl.startsWith('https://')) {
        try {
            const url = new URL(apiUrl)
            hmrHost = url.hostname
            console.log(`[Vite] Inferred HMR Host from API URL: ${hmrHost}`)
        } catch (e) { }
    }

    // Determine HMR port:
    // 1. Explicit VITE_HMR_CLIENT_PORT
    // 2. If we have a custom host (explicit or inferred), default to 443
    // 3. Otherwise undefined (Vite uses server.port or 5173)
    let hmrPort = undefined
    if (env.VITE_HMR_CLIENT_PORT) {
        hmrPort = Number(env.VITE_HMR_CLIENT_PORT)
    } else if (hmrHost) {
        hmrPort = 443
    }

    // Default protocol to wss if we have a custom host (likely https), otherwise undefined
    const hmrProtocol = env.VITE_HMR_PROTOCOL || (hmrHost ? 'wss' : undefined)
    const wsUrl = apiUrl.replace(/^http/, 'ws')

    return {
        envDir,
        define: processEnv,
        plugins: [vue()],
        build: {
            // Generate unique filenames with content hash for cache-busting
            rollupOptions: {
                output: {
                    entryFileNames: 'assets/[name]-[hash].js',
                    chunkFileNames: 'assets/[name]-[hash].js',
                    assetFileNames: 'assets/[name]-[hash].[ext]'
                }
            },
            // Generate source maps for debugging
            sourcemap: false,
            // Clear output directory on each build
            emptyOutDir: true
        },
        server: {
            port: 3010,
            host: true,
            allowedHosts: true,
            watch: {
                usePolling: true
            },
            proxy: {
                '/api': {
                    target: apiUrl,
                    changeOrigin: true,
                    rewrite: (path) => path.replace(/^\/api/, '')
                },
                '/ws': {
                    target: wsUrl,
                    ws: true
                }
            },
            hmr: disableHmr
                ? false
                : {
                    clientPort: hmrPort,
                    port: 3010,
                    host: hmrHost,
                    protocol: hmrProtocol
                }
        }
    }
})
