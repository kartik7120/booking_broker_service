package utils

import "github.com/google/uuid"

func GenerateIdempotentKey() string {
	key := uuid.New()

	return key.String()
}
