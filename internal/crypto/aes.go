package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Encrypt takes a plaintext string and a 32-byte master key,
// and returns a Base64-encoded AES-256-GCM ciphertext string.
func Encrypt(plaintext string, masterKey string) (string, error) {
	key := []byte(masterKey)
	if len(key) != 32 {
		return "", errors.New("master key must be exactly 32 bytes (characters) long")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt takes a Base64-encoded AES-256-GCM ciphertext string and a 32-byte master key,
// and returns the decrypted plaintext string.
func Decrypt(base64Ciphertext string, masterKey string) (string, error) {
	key := []byte(masterKey)
	if len(key) != 32 {
		return "", errors.New("master key must be exactly 32 bytes (characters) long")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(base64Ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
