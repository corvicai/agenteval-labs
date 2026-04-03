/**
 * Centralized environment configuration.
 *
 * This utility merges build-time environment variables (import.meta.env)
 * with runtime environment variables injected into window._env_ via env-config.js.
 */

const env = (typeof window !== 'undefined' && window._env_) || {};
const POSTHOG_PUBLIC_DEFAULT_KEY = 'phc_g5TY15YtOI4fvazYarJmuwTvqEAfl8KDyXh3HFjv0HV';
const POSTHOG_PUBLIC_DEFAULT_HOST = 'https://us.i.posthog.com';

export const config = {
    // Firebase Config
    FIREBASE_API_KEY: env.VITE_FIREBASE_API_KEY || import.meta.env.VITE_FIREBASE_API_KEY,
    FIREBASE_AUTH_DOMAIN: env.VITE_FIREBASE_AUTH_DOMAIN || import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
    FIREBASE_PROJECT_ID: env.VITE_FIREBASE_PROJECT_ID || import.meta.env.VITE_FIREBASE_PROJECT_ID,
    FIREBASE_STORAGE_BUCKET: env.VITE_FIREBASE_STORAGE_BUCKET || import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
    FIREBASE_MESSAGING_SENDER_ID: env.VITE_FIREBASE_MESSAGING_SENDER_ID || import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
    FIREBASE_APP_ID: env.VITE_FIREBASE_APP_ID || import.meta.env.VITE_FIREBASE_APP_ID,

    // WebSocket / API Config
    WS_URL: env.VITE_WS_URL || import.meta.env.VITE_WS_URL,
    WS_HOST: env.VITE_WS_HOST || import.meta.env.VITE_WS_HOST,
    WS_PORT: env.VITE_WS_PORT || import.meta.env.VITE_WS_PORT,
    API_URL: env.VITE_API_URL || import.meta.env.VITE_API_URL,

    // Feature Flags / UI Config
    ENABLE_LEGACY_AUTH: env.VITE_ENABLE_LEGACY_AUTH || import.meta.env.VITE_ENABLE_LEGACY_AUTH,
    AFK_TIMEOUT_MS: env.VITE_AFK_TIMEOUT_MS || import.meta.env.VITE_AFK_TIMEOUT_MS,
    POSTHOG_KEY: env.VITE_POSTHOG_KEY || import.meta.env.VITE_POSTHOG_KEY || POSTHOG_PUBLIC_DEFAULT_KEY,
    POSTHOG_HOST: env.VITE_POSTHOG_HOST || import.meta.env.VITE_POSTHOG_HOST || POSTHOG_PUBLIC_DEFAULT_HOST,
    POSTHOG_ENABLED: env.VITE_POSTHOG_ENABLED || import.meta.env.VITE_POSTHOG_ENABLED,
    POSTHOG_ENVIRONMENT: env.VITE_POSTHOG_ENVIRONMENT || import.meta.env.VITE_POSTHOG_ENVIRONMENT,

    // Meta
    APP_REVISION: env.VITE_APP_REVISION || env.APP_REVISION || import.meta.env.VITE_APP_REVISION || env.VITE_GIT_COMMIT || import.meta.env.VITE_GIT_COMMIT,
    APP_REVISION_BRANCH: env.VITE_APP_REVISION_BRANCH || env.APP_REVISION_BRANCH || import.meta.env.VITE_APP_REVISION_BRANCH,
    APP_REVISION_DIRTY: env.VITE_APP_REVISION_DIRTY || env.APP_REVISION_DIRTY || import.meta.env.VITE_APP_REVISION_DIRTY,
    APP_REVISION_UPDATED_AT: env.VITE_APP_REVISION_UPDATED_AT || env.APP_REVISION_UPDATED_AT || import.meta.env.VITE_APP_REVISION_UPDATED_AT,
    GIT_COMMIT: env.VITE_GIT_COMMIT || import.meta.env.VITE_GIT_COMMIT,
    DEV: import.meta.env.DEV,
    PROD: import.meta.env.PROD,
    MODE: import.meta.env.MODE,
};

export default config;
