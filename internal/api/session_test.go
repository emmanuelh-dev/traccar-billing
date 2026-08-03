package api

import (
	"testing"
	"time"
)

func TestSessionSignerRoundTrip(t *testing.T) {
	signer := newSessionSigner("test-secret-at-least-32-bytes-long")

	value, err := signer.encode(42, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	tenantID, err := signer.decode(value)
	if err != nil {
		t.Fatalf("decode() error = %v", err)
	}
	if tenantID != 42 {
		t.Errorf("decode() tenantID = %d, want 42", tenantID)
	}
}

func TestSessionSignerRejectsTampering(t *testing.T) {
	signer := newSessionSigner("test-secret-at-least-32-bytes-long")

	value, err := signer.encode(42, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	tampered := value[:len(value)-1] + "x"
	if _, err := signer.decode(tampered); err == nil {
		t.Error("decode() accepted a tampered cookie")
	}
}

func TestSessionSignerRejectsExpired(t *testing.T) {
	signer := newSessionSigner("test-secret-at-least-32-bytes-long")

	value, err := signer.encode(42, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	if _, err := signer.decode(value); err == nil {
		t.Error("decode() accepted an expired cookie")
	}
}

func TestSessionSignerRejectsDifferentSecret(t *testing.T) {
	signerA := newSessionSigner("secret-a-at-least-32-bytes-long!!")
	signerB := newSessionSigner("secret-b-at-least-32-bytes-long!!")

	value, err := signerA.encode(42, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}

	if _, err := signerB.decode(value); err == nil {
		t.Error("decode() accepted a cookie signed with a different secret")
	}
}

func TestSessionSignerRejectsMalformed(t *testing.T) {
	signer := newSessionSigner("test-secret-at-least-32-bytes-long")

	tests := []string{"", "no-dot-separator", ".", "abc.", ".abc"}
	for _, value := range tests {
		if _, err := signer.decode(value); err == nil {
			t.Errorf("decode(%q) accepted malformed input", value)
		}
	}
}
