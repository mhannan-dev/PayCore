// mfa-key provisions a persistent local encryption key without printing it.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
	"paycore/api/internal/mfa"
)

func main() {
	if key := os.Getenv("MFA_ENCRYPTION_KEY"); key != "" {
		if _, err := mfa.New(key); err != nil {
			log.Fatal("Environment MFA_ENCRYPTION_KEY is invalid")
		}
		log.Println("MFA encryption key supplied by environment; preserved")
		return
	}
	values, err := godotenv.Read(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("Could not read .env")
	}
	if key := values["MFA_ENCRYPTION_KEY"]; key != "" {
		if _, err := mfa.New(key); err != nil {
			log.Fatal("Existing MFA_ENCRYPTION_KEY is invalid; restore the original key rather than replacing it")
		}
		log.Println("MFA encryption key already configured; preserved")
		return
	}
	var key [32]byte
	if _, err = rand.Read(key[:]); err != nil {
		log.Fatal("Key generation failed")
	}
	file, err := os.OpenFile(".env", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal("Could not open .env")
	}
	if _, err = file.WriteString("\nMFA_ENCRYPTION_KEY=" + base64.StdEncoding.EncodeToString(key[:]) + "\n"); err != nil {
		log.Fatal("Could not save MFA key")
	}
	if err = file.Sync(); err != nil {
		log.Fatal("Could not sync MFA key")
	}
	if err = file.Close(); err != nil {
		log.Fatal("Could not close .env after saving the MFA key")
	}
	log.Println("MFA encryption key saved to .env; keep a secure backup")
}
