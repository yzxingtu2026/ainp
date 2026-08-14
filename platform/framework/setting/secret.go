package setting

import (
	"errors"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/credential"
)

var ErrSecretUnavailable = errors.New("setting secret unavailable")

func HasSecretFields(def Definition) bool {
	for _, field := range def.Fields {
		if field.Type == FieldSecret {
			return true
		}
	}
	return false
}

func SecretAAD(tenantID, settingKey, fieldKey string, schemaVersion int) []byte {
	return []byte(fmt.Sprintf("setting|tenant|%s|%s|%s|%d", tenantID, settingKey, fieldKey, schemaVersion))
}

type SecretCipher = credential.Cipher
