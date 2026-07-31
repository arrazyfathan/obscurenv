package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	envelopeVersion = 1
	kdfName         = "argon2id"
	argonTime       = 1
	argonMemory     = 64 * 1024
	argonThreads    = 4
	keyLength       = 32
)

type Envelope struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Ciphertext string `json:"ciphertext"`
}

func DeriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, keyLength)
}

func Encrypt(plaintext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(base64Payload string, key []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(base64Payload)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext payload too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return aesGCM.Open(nil, nonce, ciphertext, nil)
}

func EncryptWithPassphrase(plaintext []byte, passphrase string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key := DeriveKey(passphrase, salt)
	defer Wipe(key)
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		return "", err
	}
	envelope := Envelope{
		Version:    envelopeVersion,
		KDF:        kdfName,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Ciphertext: ciphertext,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DecryptWithPassphrase(payload string, passphrase string) ([]byte, error) {
	var envelope Envelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, err
	}
	if envelope.Version != envelopeVersion || envelope.KDF != kdfName {
		return nil, errors.New("unsupported encrypted payload format")
	}
	salt, err := base64.StdEncoding.DecodeString(envelope.Salt)
	if err != nil {
		return nil, err
	}
	key := DeriveKey(passphrase, salt)
	defer Wipe(key)
	return Decrypt(envelope.Ciphertext, key)
}

func Wipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
