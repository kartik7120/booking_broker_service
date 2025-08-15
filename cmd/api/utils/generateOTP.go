package utils

import "crypto/rand"

func GenerateOTP(length int) (string, error) {
	const digits = "0123456789"
	otp := make([]byte, length)
	_, err := rand.Read(otp)
	if err != nil {
		return "", err
	}
	for i := range otp {
		otp[i] = digits[otp[i]%byte(len(digits))]
	}
	return string(otp), nil
}
