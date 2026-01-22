import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
    const env = loadEnv(mode, process.cwd(), '')
    const hmrPortRaw = env.VITE_HMR_CLIENT_PORT ? Number(env.VITE_HMR_CLIENT_PORT) : 3010
    const hmrPort = Number.isFinite(hmrPortRaw) && hmrPortRaw > 0 ? hmrPortRaw : 3010
    const disableHmr = env.VITE_DISABLE_HMR === '1'
    const hmrHost = env.VITE_HMR_HOST || undefined
    const hmrProtocol = env.VITE_HMR_PROTOCOL || undefined
    const apiUrl = env.VITE_API_URL || 'http://go-api:8080'
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
            port: 5173,
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
                    port: 5173,
                    host: hmrHost,
                    protocol: hmrProtocol
                }
        }
    }
})
