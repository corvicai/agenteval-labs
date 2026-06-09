package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"benchmarking-platform/internal/security"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// EncryptedJSON is a custom type that stores JSON as an encrypted string in the database
type EncryptedJSON json.RawMessage

// Scan scan value into EncryptedJSON, implements sql.Scanner interface
func (j *EncryptedJSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var s string
	switch v := value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal EncryptedJSON value:", value))
	}

	// If it's empty or looks like valid JSON (starts with { or [), and decryption fails,
	// we assume it's unencrypted legacy data.
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		*j = nil
		return nil
	}

	// Try to decrypt
	decrypted, err := security.Decrypt(s)
	if err != nil {
		// Fallback: If it looks like JSON, treat it as raw JSON (legacy data)
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
			*j = EncryptedJSON(s)
			return nil
		}
		// If decryption fails and it's not legacy JSON, return a marker so the record
		// can still be loaded (and thus deleted) instead of failing the whole query.
		*j = EncryptedJSON(`{"_error":"decryption_failed"}`)
		return nil
	}

	*j = EncryptedJSON(decrypted)
	return nil
}

// ConfigDecryptionFailed reports whether raw is the poison marker written by
// EncryptedJSON.Scan when a stored config could not be decrypted (see above).
// Callers should treat such configs as "credentials must be re-entered", not
// as "credentials missing".
func ConfigDecryptionFailed(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, bad := m["_error"]
	return bad
}

// Value return json value, implements driver.Valuer interface
func (j EncryptedJSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}

	// Encrypt the raw bytes
	encrypted, err := security.Encrypt([]byte(j))
	if err != nil {
		return nil, err
	}

	return encrypted, nil
}

// GormDataType gorm data type
func (EncryptedJSON) GormDataType() string {
	return "text"
}

// GormDBDataType gorm db data type
func (EncryptedJSON) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	return "text"
}

// MarshalJSON to support json.Marshaler interface
func (j EncryptedJSON) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON to support json.Unmarshaler interface
func (j *EncryptedJSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("EncryptedJSON: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}
