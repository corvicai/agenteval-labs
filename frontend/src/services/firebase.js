import { initializeApp } from "firebase/app";
import { getAuth, GoogleAuthProvider, GithubAuthProvider, signInWithPopup, signOut } from "firebase/auth";

// Firebase configuration from environment variables
// These should be set in .env or provided by the user
const firebaseConfig = {
    apiKey: import.meta.env.VITE_FIREBASE_API_KEY || "PLACEHOLDER",
    authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN || "PLACEHOLDER",
    projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID || "PLACEHOLDER",
    storageBucket: import.meta.env.VITE_FIREBASE_STORAGE_BUCKET || "PLACEHOLDER",
    messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID || "PLACEHOLDER",
    appId: import.meta.env.VITE_FIREBASE_APP_ID || "PLACEHOLDER"
};

console.log('[Firebase] Config loaded:', {
    apiKey: firebaseConfig.apiKey ? 'Set (' + firebaseConfig.apiKey.substring(0, 5) + '...)' : 'Missing',
    projectId: firebaseConfig.projectId,
    authDomain: firebaseConfig.authDomain
})

// Initialize Firebase
const app = initializeApp(firebaseConfig);
const auth = getAuth(app);

// Providers
const googleProvider = new GoogleAuthProvider();
const githubProvider = new GithubAuthProvider();

export const loginWithGoogle = async () => {
    const result = await signInWithPopup(auth, googleProvider);
    return result.user;
};

export const loginWithGithub = async () => {
    const result = await signInWithPopup(auth, githubProvider);
    return result.user;
};

export const getIdToken = async () => {
    if (!auth.currentUser) return null;
    return await auth.currentUser.getIdToken(true);
};

export const deleteCurrentUser = async () => {
    if (!auth.currentUser) return;
    try {
        await auth.currentUser.delete();
        console.log('[Firebase] User deleted successfully');
    } catch (e) {
        console.error('[Firebase] Failed to delete user:', e);
        if (e.code === 'auth/requires-recent-login') {
            throw new Error('This operation is sensitive and requires recent authentication. Please log out and log in again before deleting your account.');
        }
        throw e;
    }
};

export const logoutFirebase = async () => {
    try {
        await signOut(auth);
        console.log('[Firebase] Signed out successfully');
    } catch (e) {
        console.error('[Firebase] Sign out failed:', e);
    }
};

export { auth, googleProvider, githubProvider };
export default app;
