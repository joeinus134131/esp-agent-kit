package pair

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"
)

var ErrDeclined = errors.New("pairing ditolak user")

type Msg struct {
	V      int    `json:"v"`
	T      string `json:"t"`
	Nonce  string `json:"nonce,omitempty"`
	Serial string `json:"serial,omitempty"`
	FW     string `json:"fw,omitempty"`
	Pub    string `json:"pub,omitempty"`
	Cert   string `json:"cert,omitempty"`
	Sig    string `json:"sig,omitempty"`
}

type Conn struct {
	rw  io.ReadWriter
	buf []byte
}

func NewConn(rw io.ReadWriter) *Conn { return &Conn{rw: rw} }

func (c *Conn) send(m Msg) error {
	m.V = 1
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = c.rw.Write(append(b, '\n'))
	return err
}

// Port serial dibuka dengan read timeout pendek, jadi Read bisa mengembalikan n=0.
func (c *Conn) readLine(d time.Duration) ([]byte, error) {
	deadline := time.Now().Add(d)
	one := make([]byte, 1)
	for time.Now().Before(deadline) {
		n, err := c.rw.Read(one)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}
		if one[0] == '\n' {
			line := c.buf
			c.buf = nil
			return line, nil
		}
		if one[0] != '\r' {
			c.buf = append(c.buf, one[0])
		}
	}
	return nil, errors.New("timeout menunggu balasan device")
}

func (c *Conn) recv(want string, timeout time.Duration) (Msg, error) {
	var m Msg
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return m, errors.New("timeout menunggu balasan device")
		}
		line, err := c.readLine(left)
		if err != nil {
			return m, err
		}
		// boot log ROM, panic, atau teks lain: lewati, tapi tampilkan di log
		if len(line) == 0 || line[0] != '{' {
			log.Printf("device: %s", line)
			continue
		}
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if m.T != want {
			return m, fmt.Errorf("pesan tak terduga: %q (harap %q)", m.T, want)
		}
		return m, nil
	}
}

type TrustStore interface {
	Get(serial string) (ed25519.PublicKey, bool)
	Put(serial string, pub ed25519.PublicKey) error
}

type Agent struct {
	Key       ed25519.PrivateKey
	VendorPub ed25519.PublicKey // nil = mode dev (cert tidak diperiksa)
	Trust     TrustStore
	Confirm   func(serial, code string) bool // blok sampai user klik Allow/Deny
}

func (a *Agent) Handshake(c *Conn) (string, error) {
	na := rnd(16)
	if err := c.send(Msg{T: "hello", Nonce: enc(na)}); err != nil {
		return "", err
	}
	id, err := c.recv("id", 3*time.Second)
	if err != nil {
		return "", err
	}
	pub, e1 := dec(id.Pub)
	nd, e2 := dec(id.Nonce)
	cert, _ := dec(id.Cert) // boleh kosong di mode dev
	sig, e4 := dec(id.Sig)
	if err := errors.Join(e1, e2, e4); err != nil || len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return "", errors.New("pesan id tidak valid")
	}

	// 1. Keaslian perangkat (ditandatangani vendor saat produksi)
	if a.VendorPub != nil &&
		!ed25519.Verify(a.VendorPub, cat([]byte("DEV1"), []byte(id.Serial), pub), cert) {
		return "", errors.New("perangkat tidak dikenal (cert salah)")
	}
	// 2. Device memegang private key-nya, untuk sesi ini
	if !ed25519.Verify(pub, cat([]byte("ID1"), na, nd, []byte(id.Serial)), sig) {
		return "", errors.New("tanda tangan device salah")
	}

	agentPub := a.Key.Public().(ed25519.PublicKey)

	// Jalur trusted
	if known, ok := a.Trust.Get(id.Serial); ok {
		if !bytes.Equal(known, pub) {
			return "", errors.New("kunci device berubah: tolak, pairing ulang manual")
		}
		s := ed25519.Sign(a.Key, cat([]byte("AG1"), nd, na))
		if err := c.send(Msg{T: "auth", Pub: enc(agentPub), Sig: enc(s)}); err != nil {
			return "", err
		}
		_, err := c.recv("ready", 3*time.Second)
		return id.Serial, err
	}

	// Jalur pairing
	if err := c.send(Msg{T: "pair", Pub: enc(agentPub)}); err != nil {
		return "", err
	}
	code := sas(na, nd, pub, agentPub)
	if !a.Confirm(id.Serial, code) {
		_ = c.send(Msg{T: "pair_cancel"})
		return "", ErrDeclined
	}
	s := ed25519.Sign(a.Key, cat([]byte("PAIR1"), nd, na))
	if err := c.send(Msg{T: "pair_ok", Sig: enc(s)}); err != nil {
		return "", err
	}
	// device membalas setelah tombol fisik ditekan
	if _, err := c.recv("paired", 130*time.Second); err != nil {
		return "", fmt.Errorf("menunggu konfirmasi tombol di device: %w", err)
	}
	return id.Serial, a.Trust.Put(id.Serial, pub)
}

func sas(na, nd, devPub, agentPub []byte) string {
	h := sha256.Sum256(cat([]byte("SAS1"), na, nd, devPub, agentPub))
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(h[:4])%1000000)
}

func rnd(n int) []byte             { b := make([]byte, n); _, _ = rand.Read(b); return b }
func enc(b []byte) string          { return base64.StdEncoding.EncodeToString(b) }
func dec(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
func cat(parts ...[]byte) []byte   { return bytes.Join(parts, nil) }
