// Package secrets provides authenticated symmetric encryption (AES-256-GCM)
// for credentials stored at rest in the database (ex.: senha ISAPI dos devices).
// A chave mestra vem de ISAPI_CRED_KEY (.env); nunca é logada nem persistida.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// KeySize é o tamanho da chave AES-256 em bytes.
const KeySize = 32

// ParseKey decodifica a chave mestra a partir de hex (64 chars) ou base64.
// Exige exatamente 32 bytes (AES-256). Gere com: openssl rand -hex 32.
func ParseKey(s string) ([]byte, error) {
	if len(s) == KeySize*2 {
		if b, err := hex.DecodeString(s); err == nil && len(b) == KeySize {
			return b, nil
		}
	}
	for _, dec := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
	} {
		if b, err := dec(s); err == nil && len(b) == KeySize {
			return b, nil
		}
	}
	return nil, fmt.Errorf("secrets: chave deve ter %d bytes (hex de %d chars ou base64)", KeySize, KeySize*2)
}

// Cipher cifra/decifra valores com AES-256-GCM.
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher cria um Cipher a partir de uma chave de 32 bytes.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secrets: chave deve ter %d bytes, recebeu %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

// Encrypt retorna nonce||ciphertext (o nonce é prefixado para o Decrypt).
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secrets: nonce: %w", err)
	}
	return c.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt desfaz Encrypt; falha se o blob foi adulterado (GCM autentica).
func (c *Cipher) Decrypt(blob []byte) (string, error) {
	ns := c.gcm.NonceSize()
	if len(blob) < ns {
		return "", errors.New("secrets: ciphertext muito curto")
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt: %w", err)
	}
	return string(pt), nil
}
