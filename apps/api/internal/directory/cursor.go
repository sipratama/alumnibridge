package directory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

const MaxCursorLength = 2048
const cursorTTL = 15 * time.Minute

var cursorPurpose = []byte("alumnibridge-directory-v1")

type Cursor struct {
	aead cipher.AEAD
	now  func() time.Time
}
type cursorPayload struct {
	Version int      `json:"v"`
	Expires int64    `json:"exp"`
	Filters [32]byte `json:"filters"`
	Slug    string   `json:"slug"`
}

func NewCursor(key []byte) (*Cursor, error) {
	if len(key) != 32 {
		return nil, errors.New("directory cursor key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cursor{aead: aead, now: time.Now}, nil
}
func fingerprint(spec Spec) [32]byte {
	spec.After = ""
	data, _ := json.Marshal(spec)
	return sha256.Sum256(data)
}
func (c *Cursor) Seal(spec Spec, slug string) (string, error) {
	if !profiles.ValidSlug(slug) {
		return "", ErrInvalid
	}
	data, err := json.Marshal(cursorPayload{1, c.now().Add(cursorTTL).Unix(), fingerprint(spec), slug})
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(c.aead.Seal(nonce, nonce, data, cursorPurpose)), nil
}
func (c *Cursor) Open(token string, spec Spec) (string, error) {
	if len(token) == 0 || len(token) > MaxCursorLength {
		return "", ErrInvalid
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || base64.RawURLEncoding.EncodeToString(data) != token || len(data) < c.aead.NonceSize()+c.aead.Overhead() {
		return "", ErrInvalid
	}
	nonce := data[:c.aead.NonceSize()]
	plain, err := c.aead.Open(nil, nonce, data[c.aead.NonceSize():], cursorPurpose)
	if err != nil {
		return "", ErrInvalid
	}
	var p cursorPayload
	if err = json.Unmarshal(plain, &p); err != nil || p.Version != 1 || p.Expires <= c.now().Unix() || p.Expires > c.now().Add(cursorTTL).Unix() || p.Filters != fingerprint(spec) || !profiles.ValidSlug(p.Slug) {
		return "", ErrInvalid
	}
	return p.Slug, nil
}
