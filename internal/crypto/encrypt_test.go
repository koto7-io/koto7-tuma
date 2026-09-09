package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptBytesRoundTrip(t *testing.T) {
	enc, err := NewEncryptor([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"customer":"cus_123","amount":5000}`)
	cipher, err := enc.EncryptBytes(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(cipher, plain) {
		t.Fatal("ciphertext must differ from plaintext")
	}
	got, err := enc.DecryptBytes(cipher)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip failed: %q", got)
	}
}
