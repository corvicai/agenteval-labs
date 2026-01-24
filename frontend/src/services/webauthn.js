import { create, get } from '@github/webauthn-json';

/**
 * Helper to handle WebAuthn registration/login ceremony
 */
export const webauthnService = {
    /**
     * Register a new credential
     * @param {Object} options - Registration options from server (PublicCredentialCreationOptions)
     * @returns {Promise<Object>} - Credential creation response to be sent back to server
     */
    async createCredential(options) {
        try {
            console.log('[WebAuthn] Creating credential with options:', options);
            const credential = await create(options);
            return credential;
        } catch (err) {
            console.error('[WebAuthn] Registration error:', err);
            throw err;
        }
    },

    /**
     * Request an assertion for login
     * @param {Object} options - Login options from server (PublicCredentialRequestOptions)
     * @returns {Promise<Object>} - Assertion response to be sent back to server
     */
    async getAssertion(options) {
        try {
            console.log('[WebAuthn] Getting assertion with options:', options);
            const assertion = await get(options);
            return assertion;
        } catch (err) {
            console.error('[WebAuthn] Login error:', err);
            throw err;
        }
    },

    /**
     * Check if WebAuthn is supported in this browser
     * @returns {boolean}
     */
    isSupported() {
        return (
            window.PublicKeyCredential &&
            typeof window.PublicKeyCredential === "function"
        );
    }
};
