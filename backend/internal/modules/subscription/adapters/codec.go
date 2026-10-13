package adapters

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"io"
)

type TokenCodec struct{ aead cipher.AEAD }

func NewTokenCodec(key []byte) (*TokenCodec, error) {
	if len(key) != 32 {
		return nil, domain.ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return &TokenCodec{aead: aead}, nil
}
func (c *TokenCodec) Seal(token, aad string) ([]byte, error) {
	if c == nil || c.aead == nil || !domain.ValidToken(token) || aad == "" {
		return nil, domain.ErrInvalid
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, domain.ErrUnavailable
	}
	result := append([]byte{1}, nonce...)
	return c.aead.Seal(result, nonce, []byte(token), []byte(aad)), nil
}
func (c *TokenCodec) Open(raw []byte, aad string) (string, error) {
	if c == nil || c.aead == nil || len(raw) < 1+c.aead.NonceSize()+c.aead.Overhead() || raw[0] != 1 {
		return "", domain.ErrDenied
	}
	n := c.aead.NonceSize()
	clear, err := c.aead.Open(nil, raw[1:1+n], raw[1+n:], []byte(aad))
	if err != nil || !domain.ValidToken(string(clear)) {
		return "", domain.ErrDenied
	}
	return string(clear), nil
}
