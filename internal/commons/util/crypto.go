package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// DecryptAES256GCM decrypts a base64-encoded string using AES-256-GCM
// The encrypted value should be [IV (12 bytes)][Ciphertext][Tag (16 bytes)]
func DecryptAES256GCM(encryptedBase64 string, key string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("MASTER_INTERNAL_KEY must be exactly 32 characters (32 bytes)")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		// If it's not valid base64, return original (or handle error)
		// This allows mixed encrypted and plain text during transition
		return encryptedBase64, nil 
	}

	if len(ciphertext) < 12+16 {
		// Too short to be AES-GCM, return as is
		return encryptedBase64, nil
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesgcm.NonceSize()
	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := aesgcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		// Decryption failed. It might be plain text that happened to be valid base64.
		// Return original if decryption fails.
		return encryptedBase64, nil
	}

	return string(plaintext), nil
}

// EncryptAES256GCM encrypts a string using AES-256-GCM and returns base64 string
func EncryptAES256GCM(plaintext string, key string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("MASTER_INTERNAL_KEY must be exactly 32 characters")
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
