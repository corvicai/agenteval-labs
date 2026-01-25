import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
    const env = loadEnv(mode, process.cwd(), '')
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
