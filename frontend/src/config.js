/**
 * Centralized environment configuration.
 *
 * This utility merges build-time environment variables (import.meta.env)
 * with runtime environment variables injected into window._env_ via env-config.js.
 */

const env = (typeof window !== 'undefined' && window._env_) || {};

export const config = {
    // Firebase Config
    FIREBASE_API_KEY: env.VITE_FIREBASE_API_KEY || import.meta.env.VITE_FIREBASE_API_KEY,
    FIREBASE_AUTH_DOMAIN: env.VITE_FIREBASE_AUTH_DOMAIN || import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
    FIREBASE_PROJECT_ID: env.VITE_FIREBASE_PROJECT_ID || import.meta.env.VITE_FIREBASE_PROJECT_ID,
    FIREBASE_STORAGE_BUCKET: env.VITE_FIREBASE_STORAGE_BUCKET || import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
    FIREBASE_MESSAGING_SENDER_ID: env.VITE_FIREBASE_MESSAGING_SENDER_ID || import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
    FIREBASE_APP_ID: env.VITE_FIREBASE_APP_ID || import.meta.env.VITE_FIREBASE_APP_ID,

    // WebSocket / API Config
    WS_HOST: env.VITE_WS_HOST || import.meta.env.VITE_WS_HOST,
    WS_PORT: env.VITE_WS_PORT || import.meta.env.VITE_WS_PORT,

    // Feature Flags / UI Config
    ENABLE_LEGACY_AUTH: env.VITE_ENABLE_LEGACY_AUTH || import.meta.env.VITE_ENABLE_LEGACY_AUTH,

    // Meta
    DEV: import.meta.env.DEV,
    PROD: import.meta.env.PROD,
    MODE: import.meta.env.MODE,
};

export default config;
