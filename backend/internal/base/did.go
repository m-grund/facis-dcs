package base

import (
	"crypto/rand"
	"fmt"
	"os"
)

// UUID v4
func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func GetDID() (*string, error) {

	dcsIssuer := os.Getenv("DCS_ISSUER")

	uuid, err := generateUUID()
	if err != nil {
		return nil, err
	}

	did := fmt.Sprintf("did:%s:%s", dcsIssuer, uuid)
	return &did, nil
}
