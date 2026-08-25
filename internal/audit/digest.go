package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func digestJSON(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func validHexDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
