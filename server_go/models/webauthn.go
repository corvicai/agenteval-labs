package models

import (
	"benchmarking-platform/internal/logger"

	"github.com/go-webauthn/webauthn/webauthn"
)

// WebAuthnID returns the user's ID as a byte slice
func (u User) WebAuthnID() []byte {
	return u.ID[:]
}

// WebAuthnName returns the user's name (email)
func (u User) WebAuthnName() string {
	return u.Email
}

// WebAuthnDisplayName returns the user's display name
func (u User) WebAuthnDisplayName() string {
	return u.Name
}

// WebAuthnIcon returns the user's icon (not used)
func (u User) WebAuthnIcon() string {
	return ""
}

// WebAuthnCredentials returns the user's credentials
func (u User) WebAuthnCredentials() []webauthn.Credential {
	creds := make([]webauthn.Credential, len(u.Passkeys))
	for i, pk := range u.Passkeys {
		creds[i] = pk.ToWebAuthnCredential()
		logger.Debug("[WebAuthn] Loading stored key %s: BE=%v, BS=%v, SignCount=%d",
			pk.ID, pk.BackupEligible, pk.BackupState, pk.SignCount)
	}
	return creds
}

func (pk Passkey) ToWebAuthnCredential() webauthn.Credential {
	return webauthn.Credential{
		ID:              pk.CredentialID,
		PublicKey:       pk.PublicKey,
		AttestationType: pk.Attestation,
		Authenticator: webauthn.Authenticator{
			SignCount: pk.SignCount,
		},
		Flags: webauthn.CredentialFlags{
			BackupEligible: pk.BackupEligible,
			BackupState:    pk.BackupState,
		},
	}
}
