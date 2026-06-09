package models

import "testing"

func TestConfigDecryptionFailed(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want bool
	}{
		{"decryption marker", []byte(`{"_error":"decryption_failed"}`), true},
		{"normal config", []byte(`{"api_key":"sk-123"}`), false},
		{"empty", nil, false},
		{"empty object", []byte(`{}`), false},
		{"non-json", []byte(`not json`), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConfigDecryptionFailed(tc.raw); got != tc.want {
				t.Fatalf("ConfigDecryptionFailed(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
