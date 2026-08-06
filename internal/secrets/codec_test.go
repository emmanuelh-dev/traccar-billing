package secrets

import "testing"

func TestCodecRoundTrip(t *testing.T) {
	codec, err := NewCodec("local-preview-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := codec.Encrypt("provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "provider-token" {
		t.Fatal("Encrypt() stored plaintext")
	}
	decrypted, err := codec.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "provider-token" {
		t.Fatalf("Decrypt() = %q", decrypted)
	}
}
