package util

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTripAndFreshNonce(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	plain := []byte("sk-upstream-secret-value")
	a, err := EncryptKey(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptKey(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("each encryption must use a fresh nonce")
	}
	if bytes.Contains(a, plain) {
		t.Fatal("ciphertext must not contain plaintext")
	}
	for _, blob := range [][]byte{a, b} {
		out, err := DecryptKey(key, blob)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, plain) {
			t.Fatalf("round trip mismatch: %q", out)
		}
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	blob, err := EncryptKey(key, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	blob[len(blob)-1] ^= 0x01
	if _, err := DecryptKey(key, blob); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
	wrong := make([]byte, 32)
	_, _ = rand.Read(wrong)
	if _, err := DecryptKey(wrong, blob); err == nil {
		t.Fatal("wrong master key must fail")
	}
	if _, err := DecryptKey(key, []byte("short")); err == nil {
		t.Fatal("too-short blob must fail")
	}
}
