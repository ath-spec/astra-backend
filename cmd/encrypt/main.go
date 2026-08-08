package main

import (
	"fmt"
	"os"

	"github.com/yourusername/astra-backend/internal/crypto"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: go run cmd/encrypt/main.go <secret_value_to_encrypt> <32_char_master_key>")
		os.Exit(1)
	}

	plaintext := os.Args[1]
	masterKey := os.Args[2]

	if len(masterKey) != 32 {
		fmt.Printf("Error: Master key must be exactly 32 characters long. Provided key is %d characters.\n", len(masterKey))
		os.Exit(1)
	}

	encrypted, err := crypto.Encrypt(plaintext, masterKey)
	if err != nil {
		fmt.Printf("Encryption failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== ENCRYPTED SECRET (Ready for Railway / .env) ===")
	fmt.Println(encrypted)
	fmt.Println("===================================================")
}
