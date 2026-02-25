<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <CorvicLogo />
        <h1>Benchmarking Platform</h1>
        <p class="login-subtitle">{{ isRegister ? 'Create your account' : 'Sign in to continue' }}</p>
      </div>

      <div class="login-auth-phase">
        <div class="social-login-section animate-in">
          <div v-if="socialError" class="error-message">
            <div>{{ socialError.message }}</div>
            <div v-if="socialError.email" class="error-meta">Email: {{ socialError.email }}</div>
            <div v-if="socialError.name" class="error-meta">Nome: {{ socialError.name }}</div>
          </div>
          <!-- Removed pre-login ToS checkbox -->
          <button @click="handleSocialLogin('google')" class="btn btn-social btn-google" :disabled="loading">
            <img src="https://www.gstatic.com/firebasejs/ui/2.0.0/images/auth/google.svg" alt="Google" />
            <span>Continue with Google</span>
          </button>
          <button @click="handleSocialLogin('github')" class="btn btn-social btn-github" :disabled="loading">
            <img src="https://www.gstatic.com/firebasejs/ui/2.0.0/images/auth/github.svg" alt="GitHub" />
            <span>Continue with GitHub</span>
          </button>

          <div class="legacy-toggle-divider">
            <button @click="showLegacyAuth = !showLegacyAuth" class="btn-legacy-toggle">
              {{ showLegacyAuth ? 'Hide legacy login' : 'Or use traditional login' }}
            </button>
          </div>
        </div>

        <form v-if="showLegacyAuth" @submit.prevent="handleSubmit" class="login-form animate-in">
          <div v-if="error" class="error-message">{{ error }}</div>

          <div v-if="isRegister" class="registration-flow">
            <div class="form-section">
              <div class="section-title">👤 User Details</div>
              <div class="form-group">
                <label>Full Name</label>
                <input v-model="form.name" name="name" autocomplete="name" type="text" placeholder="Your name" required />
              </div>
              <div class="form-group">
                <label>Email</label>
                <input v-model="form.email" name="email" autocomplete="username" type="email" placeholder="you@example.com" required />
              </div>
              <div class="form-group">
                <label>Password</label>
                <input v-model="form.password" name="password" autocomplete="new-password" type="password" placeholder="••••••••" required />
              </div>
            </div>
          </div>

          <div v-else class="login-fields">
            <div class="form-group">
              <label>Email</label>
              <input 
                v-model="form.email" 
                name="email"
                autocomplete="username"
                type="email" 
                placeholder="you@example.com"
                required
              />
            </div>

            <div class="form-group">
              <label>Password</label>
              <input 
                v-model="form.password" 
                name="password"
                autocomplete="current-password"
                type="password" 
                placeholder="••••••••"
                required
              />
            </div>
          </div>

          <div v-if="isRegister" class="form-group checkbox-group animate-in">
            <!-- Redundant checkbox removed as it is now global -->
          </div>

          <button type="submit" class="btn btn-primary btn-submit" :disabled="loading">
            <span v-if="loading" class="loading-spinner"></span>
            {{ isRegister ? 'Create Account' : 'Sign In' }}
          </button>

          <div v-if="!isRegister && isWebAuthnSupported" class="passkey-section">
            <div class="passkey-divider">
              <span>or</span>
            </div>
            <button 
              type="button" 
              class="btn btn-secondary btn-passkey" 
              @click="handlePasskeyLogin" 
              :disabled="loading"
            >
              🔑 Sign in with Passkey
            </button>
          </div>
        </form>
      </div>


      <div class="login-footer">
        <p v-if="isRegister">
          Already have an account? 
          <a href="#" @click.prevent="isRegister = false">Sign in</a>
        </p>
        <p v-else>
          Don't have an account? 
          <a href="#" @click.prevent="isRegister = true">Create one</a>
        </p>
      </div>

      <div v-if="showTermsModal" class="modal-overlay" @click.self="showTermsModal = false">
        <div class="bootstrap-modal terms-modal">
          <h3>📄 Terms of Service</h3>
          <div class="terms-content">
            <p><strong>Terms of Service and Privacy Policy</strong></p>
            <p>By registering for the Benchmarking Platform, you agree to the following terms:</p>
            <ol>
              <li><strong>Data Collection:</strong> We collect data related to your use of the platform, including agent execution logs, evaluation results, performance metrics, and system interactions. This data is used to improve benchmarking features and provide statistical insights.</li>
              <li><strong>Data Privacy:</strong> We guarantee that we do not collect or store private, confidential, or sensitive data (such as access credentials, API secrets, or personally identifiable information) that is not strictly necessary for the operation of the service or that has not been voluntarily provided for evaluation.</li>
              <li><strong>Third-Party APIs:</strong> The use of third-party AI models (e.g., OpenAI) may be subject to the respective terms of service of those providers.</li>
              <li><strong>Responsibility:</strong> Users are responsible for the actions and outputs of the agents configured on the platform.</li>
            </ol>
          </div>
          <div class="modal-actions">
            <button v-if="requiresTerms" type="button" class="btn btn-primary" @click="handleAcceptTermsAction">I Accept the Terms</button>
            <button v-else type="button" class="btn btn-primary" @click="showTermsModal = false">Close</button>
          </div>
        </div>
      </div>
    </div>
  </div>
  <!-- Dev Mode Quick Login (Floating) -->
  <div v-if="isDev" class="dev-login-floating">
    <div class="dev-panel-header">🚧 Dev Quick Login</div>

    <!-- Test Users Section -->
    <div class="dev-section">
      <div class="dev-section-title">🧪 Test Users</div>
      <div class="dev-login-list">
        <button 
          v-for="user in devUsers" 
          :key="user.email" 
          class="dev-login-btn"
          :class="user.role"
          @click="quickLogin(user)"
          :disabled="loading"
          :title="user.email"
        >
          <span class="dev-user-icon">{{ user.icon }}</span>
          <div class="dev-user-info">
            <span class="dev-user-name">{{ user.name }}</span>
            <span class="dev-user-role-badge">{{ user.role }}</span>
          </div>
        </button>
      </div>
    </div>

    <!-- Managers Section -->
    <div v-if="managers.length" class="dev-section">
      <div class="dev-section-title">👔 Managers</div>
      <div class="dev-login-list">
        <button 
          v-for="mgr in managers" 
          :key="mgr.id" 
          class="dev-login-btn manager"
          @click="quickLoginManager(mgr)"
          :disabled="loading"
          :title="mgr.email"
        >
          <span class="dev-user-icon">👔</span>
          <div class="dev-user-info">
            <span class="dev-user-name">{{ mgr.name }}</span>
            <span class="dev-user-role-badge">{{ mgr.org_name }}</span>
          </div>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import CorvicLogo from './CorvicLogo.vue'
import { wsService } from '../services/websocket.js'
import * as api from '../services/api.js'
import { webauthnService } from '../services/webauthn.js'
import { fetchSignInMethodsForEmail } from 'firebase/auth'
import { auth, loginWithGoogle, loginWithGithub, getIdToken, deleteCurrentUser } from '../services/firebase.js'
import { config } from '../config'

const emit = defineEmits(['login'])

const showLegacyAuth = ref(false)

// Dev mode detection
const isDev = config.DEV && !config.PROD

// Dev users for quick login
const devUsers = [
  { name: 'Michel Diz', email: 'micheldiz@corvic.ai', password: 'admin123', icon: '👑', role: 'admin' },
  { name: 'Gurbinder Gill', email: 'gill@corvic.ai', password: 'gill123', icon: '🛡️', role: 'admin' },
  { name: 'Hadi Ahmadi', email: 'hadi@corvic.ai', password: 'hadi123', icon: '🛡️', role: 'admin' },
  { name: 'Alice Dev', email: 'alice@corvic.ai', password: 'alice123', icon: '👩‍💻', role: 'user' },
  { name: 'Bob Engineer', email: 'bob@corvic.ai', password: 'bob123', icon: '👨‍💻', role: 'user' },
]

// Managers from backend
const managers = ref([])

const isRegister = ref(false)
const acceptTerms = ref(false)
const showTermsModal = ref(false)
const form = ref({
  name: '',
  email: '',
  password: ''
});
const error = ref('')
const socialError = ref(null)
const loading = ref(false)

const requiresTerms = ref(false)
const isWebAuthnSupported = ref(webauthnService.isSupported())

async function handleSocialLogin(provider) {
  error.value = ''
  socialError.value = null
  loading.value = true
  try {
    let user;
    if (provider === 'google') {
      user = await loginWithGoogle()
    } else {
      user = await loginWithGithub()
    }

    if (user) {
      const token = await getIdToken()
      const result = await wsService.authenticateWithFirebase(token)
      
      if (result.success) {
        if (result.requires_terms) {
          requiresTerms.value = true
          showTermsModal.value = true
          // IMPORTANT: Save state even if ToS is required, so onLogin has data later
          if (result.user) localStorage.setItem('user', JSON.stringify(result.user))
          if (result.workspace) localStorage.setItem('workspace', JSON.stringify(result.workspace))
          if (result.token) localStorage.setItem('token', result.token)
          return
        }
        
        // Save user/workspace info like in legacy login
        if (result.user) localStorage.setItem('user', JSON.stringify(result.user))
        if (result.workspace) localStorage.setItem('workspace', JSON.stringify(result.workspace))
        if (result.token) localStorage.setItem('token', result.token) // Internal token for handshake consistency
        
        emit('login')
      }
    }
  } catch (e) {
    console.error('Social login failed:', e)
    if (e?.code === 'auth/account-exists-with-different-credential') {
      socialError.value = await buildAccountExistsError(e)
    } else if (e?.code === 'auth/unauthorized-domain' || e?.message?.includes('auth/unauthorized-domain')) {
      socialError.value = {
        message: 'OAuth is disabled on this domain. Please use traditional login.',
        email: '',
        name: ''
      }
    } else {
      socialError.value = {
        message: 'Failed to sign in with ' + provider + '. ' + (e?.message || ''),
        email: '',
        name: ''
      }
    }
  } finally {
    loading.value = false
  }
}

async function buildAccountExistsError(errorObj) {
  const email = errorObj?.customData?.email || ''
  const name =
    errorObj?.customData?.displayName ||
    errorObj?.customData?.name ||
    errorObj?.customData?._tokenResponse?.displayName ||
    errorObj?.customData?._tokenResponse?.screenName ||
    ''

  let providerHint = ''
  if (email) {
    try {
      const methods = await fetchSignInMethodsForEmail(auth, email)
      if (methods.includes('google.com')) providerHint = 'Google'
      else if (methods.includes('github.com')) providerHint = 'GitHub'
      else if (methods.includes('password')) providerHint = 'Email and password'
    } catch (err) {
      console.warn('Failed to fetch sign-in methods:', err)
    }
  }

  const message = providerHint
    ? `An account already exists with this email. Sign in with ${providerHint} to continue, then link providers in your profile.`
    : 'An account already exists with this email. Sign in with the original provider to continue, then link providers in your profile.'

  return {
    message,
    email: email || '(nao informado)',
    name: name || '(nao informado)'
  }
}

async function handleSubmit() {
  error.value = ''
  loading.value = true

  try {
    if (isRegister.value) {
      const result = await api.register(form.value.name, form.value.email, form.value.password)
      emit('login')
    } else {
      const result = await api.login(form.value.email, form.value.password)
      
      
      if (result.requires_terms) {
        requiresTerms.value = true
        showTermsModal.value = true
        return
      }
      
      emit('login')
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function handleAcceptTermsAction() {
  loading.value = true
  try {
    const result = await wsService.acceptTerms()
    
    // Update local user record if backend returned an updated one (with TermsAcceptedAt)
    if (result && (result.user || result.ID)) {
       const updatedUser = result.user || result
       localStorage.setItem('user', JSON.stringify(updatedUser))
    }
    
    requiresTerms.value = false
    showTermsModal.value = false
    acceptTerms.value = true
    
    // We don't necessarily need to disconnect, but we do it to match previous behavior
    wsService.disconnect()
    
    emit('login')
  } catch (e) {
    error.value = "Failed to accept terms: " + e.message
  } finally {
    loading.value = false
  }
}

async function handlePasskeyLogin() {
  if (!form.value.email) {
    error.value = 'Please enter your email first to use passkey'
    return
  }
  
  error.value = ''
  loading.value = true
  
  try {
    const options = await api.webAuthnLoginBegin(form.value.email)
    const assertion = await webauthnService.getAssertion(options)
    const result = await api.webAuthnLoginFinish(form.value.email, assertion)
    
    // result.user, result.token, result.workspace are already saved in api.webAuthnLoginFinish
    emit('login')
  } catch (e) {
    if (e.name === 'NotAllowedError') {
       // User cancelled or timed out
       return
    }
    error.value = e.message
  } finally {
    loading.value = false
  }
}


async function handleCancelAndCleanup() {
  if (confirm('Are you sure you want to cancel and delete your authentication data? This will permenantly remove your login from this system.')) {
    loading.value = true
    try {
      await deleteCurrentUser()
      // Local storage cleanup
      localStorage.removeItem('user')
      localStorage.removeItem('workspace')
      localStorage.removeItem('token')
      window.location.reload() // Go back to fresh login
    } catch (e) {
      error.value = 'Cleanup failed: ' + e.message
    } finally {
      loading.value = false
    }
  }
}


// Quick login for dev mode
async function quickLogin(user) {
  error.value = ''
  loading.value = true
  try {
    const result = await api.login(user.email, user.password)
    // No token storage
    if (result.user) {
      localStorage.setItem('user', JSON.stringify(result.user))
    }
    if (result.workspace) {
      localStorage.setItem('workspace', JSON.stringify(result.workspace))
    }
    emit('login')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

// Quick login for managers (all seeded with password123)
async function quickLoginManager(mgr) {
  error.value = ''
  loading.value = true
  try {
    const result = await api.login(mgr.email, 'password123')
    // No token storage
    if (result.user) {
      localStorage.setItem('user', JSON.stringify(result.user))
    }
    if (result.workspace) {
        localStorage.setItem('workspace', JSON.stringify(result.workspace))
    }
    emit('login')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
    try {
        // Connect WebSocket anonymously for pre-auth requests
        await wsService.connectAnonymous()
        
        // Fetch managers for dev quick login
        if (isDev) {
          try {
            const mgrs = await wsService.getDevManagers()
            // Filter out managers who are already in devUsers list (admins)
            const devEmails = devUsers.map(u => u.email.toLowerCase())
            managers.value = (mgrs || []).filter(m => !devEmails.includes(m.email.toLowerCase()))
          } catch (e) {
            console.log('Could not fetch managers:', e)
          }
        }
    } catch (e) {
        console.error('Failed to initialize login screen:', e)
        try {
           // We can't really fallback to REST easily if we want to be pure WS
           // but for this specific check, maybe it's okay? 
           // However, let's just stick to WS.
        } catch (e2) {}
    }
})
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 1rem;
  overflow-y: auto; /* Allow scrolling on small screens */
}

.login-card {
  margin: auto; /* Vertically and horizontally center if space allows, otherwise scroll safe */
  background: white;
  border-radius: 16px;
  padding: 2rem;
  width: 100%;
  max-width: 440px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.2);
}

.login-header {
  text-align: center;
  margin-bottom: 2rem;
}

.login-header h1 {
  margin: 1rem 0 0.5rem;
  font-size: 1.5rem;
  color: #1e293b;
}

/* Registration Flow Enhancements */
.registration-flow {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.role-toggle {
  display: flex;
  background: #f1f5f9;
  padding: 4px;
  border-radius: 10px;
  margin-bottom: 0.5rem;
}

.social-login-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 1.5rem;
}

.tos-agreement {
  margin-bottom: 0.5rem;
  padding: 0.5rem;
  background: #f8fafc;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
}

.btn-social {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  padding: 0.875rem;
  border-radius: 8px;
  font-weight: 600;
  font-size: 0.95rem;
  border: 1px solid #e2e8f0;
  background: white;
  color: #374151;
  transition: all 0.2s;
  cursor: pointer;
}

.btn-social img {
  width: 20px;
  height: 20px;
}

.btn-social:hover:not(:disabled) {
  background: #f8fafc;
  border-color: #cbd5e1;
  transform: translateY(-1px);
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.05);
}

.btn-google {
  /* Google colors if desired */
}

.btn-github {
  background: #24292e;
  color: white;
  border-color: #24292e;
}

.btn-github:hover:not(:disabled) {
  background: #1b1f23;
  border-color: #1b1f23;
}

.legacy-toggle-divider {
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  margin: 1.5rem 0;
}

.legacy-toggle-divider::before {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  border-bottom: 1px solid #e2e8f0;
  z-index: 1;
}

.btn-legacy-toggle {
  position: relative;
  z-index: 2;
  background: white;
  padding: 0 1rem;
  border: none;
  font-size: 0.85rem;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
  transition: color 0.2s;
}

.btn-legacy-toggle:hover {
  color: #49399d;
}

.role-toggle-btn {
  flex: 1;
  padding: 0.6rem;
  border: none;
  background: transparent;
  border-radius: 8px;
  font-size: 0.85rem;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
  transition: all 0.2s;
}

.role-toggle-btn.active {
  background: white;
  color: #49399d;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1.25rem;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #eef2f6;
}

.section-title {
  font-size: 0.75rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #94a3b8;
  margin-bottom: 0.25rem;
}

.form-help {
  font-size: 0.75rem;
  margin-top: 0.25rem;
}

.animate-in {
  animation: slideDown 0.3s ease-out;
}

@keyframes slideDown {
  from { opacity: 0; transform: translateY(-10px); }
  to { opacity: 1; transform: translateY(0); }
}

.checkbox-group {
  margin: 0.5rem 0;
  flex-direction: row !important;
  align-items: center;
  gap: 0.75rem !important;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
  color: #64748b;
  cursor: pointer;
}

.checkbox-label input {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.checkbox-label a {
  color: #49399d;
  text-decoration: underline;
}

.terms-modal {
  max-width: 500px;
}

.terms-content {
  max-height: 400px;
  overflow-y: auto;
  font-size: 0.9rem;
  line-height: 1.5;
  color: #374151;
  padding: 1rem;
  background: #f8fafc;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  margin-bottom: 1rem;
}

.terms-content ol {
  padding-left: 1.25rem;
  margin: 1rem 0;
}

.terms-content li {
  margin-bottom: 0.75rem;
}

.login-fields {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.login-subtitle {
  color: #64748b;
  font-size: 0.9rem;
  margin: 0;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.form-group label {
  font-size: 0.85rem;
  font-weight: 600;
  color: #374151;
}

.form-group input {
  padding: 0.75rem 1rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 1rem;
  transition: all 0.15s;
}

.form-group input:focus {
  outline: none;
  border-color: #49399d;
  box-shadow: 0 0 0 3px rgba(73, 57, 157, 0.1);
}

.error-message {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #dc2626;
  padding: 0.75rem 1rem;
  border-radius: 8px;
  font-size: 0.875rem;
}

.error-meta {
  margin-top: 0.25rem;
  font-size: 0.8rem;
  color: #b91c1c;
}

.btn-submit {
  padding: 0.875rem;
  font-size: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
}

.security-note {
  font-size: 0.75rem;
  color: #94a3b8;
  margin-top: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
}

.login-footer {
  text-align: center;
  margin-top: 1.5rem;
  padding-top: 1.5rem;
  border-top: 1px solid #e2e8f0;
}

.login-footer p {
  margin: 0;
  color: #64748b;
  font-size: 0.9rem;
}

.login-footer a {
  color: #49399d;
  font-weight: 600;
  text-decoration: none;
}

.login-footer a:hover {
  text-decoration: underline;
}

.admin-setup {
  margin-top: 1rem !important;
  font-size: 0.8rem !important;
}

.admin-setup a {
  color: #6366f1;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.5);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.bootstrap-modal {
  background: white;
  border-radius: 12px;
  padding: 1.5rem;
  width: 100%;
  max-width: 380px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.2);
}

.bootstrap-modal h3 {
  margin: 0 0 0.5rem;
  font-size: 1.1rem;
}

.bootstrap-modal p {
  margin: 0 0 1rem;
  font-size: 0.85rem;
  color: #64748b;
}

.modal-actions {
  display: flex;
  gap: 0.75rem;
  justify-content: flex-end;
  margin-top: 1rem;
}

/* Dev Mode Quick Login Floating Panel */
.dev-login-floating {
  position: fixed;
  top: 2rem;
  right: 2rem;
  width: 260px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.15);
  padding: 1rem;
  z-index: 1000;
  border: 1px solid #e2e8f0;
  max-height: calc(100vh - 4rem);
  overflow-y: auto;
  animation: slideIn 0.3s ease-out;
}

@keyframes slideIn {
  from { opacity: 0; transform: translateX(20px); }
  to { opacity: 1; transform: translateX(0); }
}

.dev-panel-header {
  font-size: 0.85rem;
  font-weight: 700;
  color: #1e293b;
  text-align: center;
  margin-bottom: 1rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #f1f5f9;
}

.dev-login-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dev-login-btn {
  display: flex;
  align-items: center;
  padding: 0.5rem 0.75rem;
  border: 1px solid #e2e8f0;
  background: white;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.15s;
  gap: 0.75rem;
  width: 100%;
  text-align: left;
}

.dev-login-btn:hover {
  background: #f8fafc;
  border-color: #cbd5e1;
  transform: translateX(-2px);
}

.dev-user-icon {
  font-size: 1.25rem;
  background: #f1f5f9;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
}

.dev-user-info {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  flex: 1;
}

.dev-user-name {
  font-size: 0.8rem;
  font-weight: 600;
  color: #334155;
}

.dev-user-role-badge {
  font-size: 0.65rem;
  color: #64748b;
  text-transform: uppercase;
  font-weight: 500;
  background: #f1f5f9;
  align-self: flex-start;
  padding: 1px 4px;
  border-radius: 4px;
}

.dev-login-btn.admin .dev-user-icon { background: #efeaff; }
.dev-login-btn.admin .dev-user-role-badge { background: #efeaff; color: #6d28d9; }

.dev-login-btn.manager .dev-user-icon { background: #fef3c7; }
.dev-login-btn.manager .dev-user-role-badge { background: #fef3c7; color: #b45309; text-transform: none; }

.dev-login-btn.org .dev-user-icon { background: #ecfdf5; }
.dev-login-btn.org .dev-user-role-badge { background: #ecfdf5; color: #047857; }

.dev-section {
  margin-bottom: 0.75rem;
}

.dev-section-title {
  font-size: 0.7rem;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 0.5rem;
  padding-left: 0.25rem;
}

.org-selector,
.join-org-view {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  animation: fadeIn 0.3s ease-out;
}

.org-selector-header,
.join-org-header {
  text-align: center;
}

.org-selector-icon,
.join-org-icon {
  font-size: 3rem;
  margin-bottom: 1rem;
}

.org-selector h3,
.join-org-view h3 {
  font-size: 1.4rem;
  color: #1e293b;
  margin: 0 0 0.5rem;
}

.org-selector p,
.join-org-view p {
  color: #64748b;
  font-size: 0.95rem;
  line-height: 1.5;
  margin: 0;
}

.org-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.org-btn {
  display: flex;
  align-items: center;
  width: 100%;
  padding: 1rem;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
  gap: 1rem;
}

.org-btn:hover:not(:disabled) {
  border-color: #49399d;
  background: #f8fafc;
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.org-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.org-icon {
  font-size: 1.5rem;
  background: #f1f5f9;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
}

.org-info {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.org-name {
  font-weight: 600;
  color: #1e293b;
  font-size: 1rem;
}

.org-role {
  font-size: 0.8rem;
  color: #64748b;
  text-transform: capitalize;
}

.select-arrow {
  color: #cbd5e1;
  font-weight: bold;
  font-size: 1.2rem;
}

.org-btn:hover .select-arrow {
  color: #49399d;
  transform: translateX(3px);
  transition: all 0.2s;
}

.suspended-tag {
  color: #dc2626;
  font-size: 0.7rem;
  font-weight: 700;
  background: #fef2f2;
  padding: 2px 6px;
  border-radius: 4px;
  align-self: flex-start;
  margin-left: 0.5rem;
  margin-top: 2px;
}

.btn-ghost {
  background: transparent;
  border: 1px solid transparent;
  color: #64748b;
  font-weight: 600;
  font-size: 0.9rem;
  width: 100%;
}

.btn-ghost:hover {
  color: #49399d;
  background: #f1f5f9;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.org-selector h3 {
  margin: 0 0 0.5rem;
  font-size: 1.25rem;
  color: #1e293b;
  text-align: center;
}

.org-selector p {
  font-size: 0.875rem;
  color: #64748b;
  text-align: center;
  margin-bottom: 1.5rem;
}

.mt-4 {
  margin-top: 1rem;
}

.btn-cancel-delete {
  margin-top: 1.5rem;
  background: transparent;
  border: none;
  color: #fca5a5;
  font-size: 0.8rem;
  text-decoration: underline;
  cursor: pointer;
  transition: color 0.2s;
}

.btn-cancel-delete:hover:not(:disabled) {
  color: #ef4444;
}

.btn-cancel-delete:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

.passkey-section {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.passkey-divider {
  display: flex;
  align-items: center;
  text-align: center;
  color: #94a3b8;
  font-size: 0.8rem;
  font-weight: 500;
}

.passkey-divider::before, 
.passkey-divider::after {
  content: '';
  flex: 1;
  border-bottom: 1px solid #e2e8f0;
}

.passkey-divider span {
  padding: 0 0.75rem;
}

.btn-passkey {
  border: 1px solid #e2e8f0;
  background: white;
  color: #374151;
  font-weight: 600;
  transition: all 0.2s;
  padding: 0.875rem;
  border-radius: 8px;
  cursor: pointer;
}

.btn-passkey:hover:not(:disabled) {
  background: #f8fafc;
  border-color: #cbd5e1;
  transform: translateY(-1px);
}
</style>
