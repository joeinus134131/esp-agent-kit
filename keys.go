package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const svc = "tokenmeter"

func configDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d = filepath.Join(d, "tokenmeter")
	return d, os.MkdirAll(d, 0o700)
}

func fromSeed(s string) ed25519.PrivateKey {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil
	}
	return ed25519.NewKeyFromSeed(seed)
}

// Urutan: keychain OS, lalu file (untuk Linux tanpa secret service), lalu buat baru.
func loadAgentKey() (ed25519.PrivateKey, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	keyFile := filepath.Join(dir, "agent.key")

	if s, err := keyring.Get(svc, "agent-key"); err == nil {
		if k := fromSeed(s); k != nil {
			return k, nil
		}
	}
	if b, err := os.ReadFile(keyFile); err == nil {
		if k := fromSeed(string(b)); k != nil {
			return k, nil
		}
	}

	_, priv, err := ed25519.GenerateKey(nil) // nil = crypto/rand
	if err != nil {
		return nil, err
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed())
	if err := keyring.Set(svc, "agent-key", seed); err != nil {
		if err := os.WriteFile(keyFile, []byte(seed), 0o600); err != nil {
			return nil, err
		}
	}
	return priv, nil
}

// Trust store: public key device bukan rahasia, cukup file JSON.
type fileTrust struct {
	mu   sync.Mutex
	path string
	m    map[string]string // serial -> base64(pub)
}

func newFileTrust() (*fileTrust, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	t := &fileTrust{path: filepath.Join(dir, "trusted.json"), m: map[string]string{}}
	if b, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(b, &t.m)
	}
	return t, nil
}

func (t *fileTrust) Get(serial string) (ed25519.PublicKey, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.m[serial]
	if !ok {
		return nil, false
	}
	b, err := base64.StdEncoding.DecodeString(s)
	return b, err == nil && len(b) == ed25519.PublicKeySize
}

func (t *fileTrust) Put(serial string, pub ed25519.PublicKey) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[serial] = base64.StdEncoding.EncodeToString(pub)
	b, _ := json.MarshalIndent(t.m, "", "  ")
	return os.WriteFile(t.path, b, 0o600)
}
