package secrets

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, err := NewCipher(testKey(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	for _, pt := range []string{"", "admin", "s3nh@-do-device!", strings.Repeat("x", 1000)} {
		blob, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		if pt != "" && bytes.Contains(blob, []byte(pt)) {
			t.Errorf("ciphertext contém o plaintext em claro")
		}
		got, err := c.Decrypt(blob)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != pt {
			t.Errorf("round-trip: got %q, want %q", got, pt)
		}
	}
}

func TestEncrypt_NonceIsRandom(t *testing.T) {
	c, _ := NewCipher(testKey(t))
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if bytes.Equal(a, b) {
		t.Error("dois Encrypt do mesmo texto produziram o mesmo blob (nonce não é aleatório)")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	c1, _ := NewCipher(testKey(t))
	other := testKey(t)
	other[0] ^= 0xFF
	c2, _ := NewCipher(other)
	blob, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(blob); err == nil {
		t.Error("Decrypt com chave errada deveria falhar (GCM autentica)")
	}
}

func TestDecrypt_TamperedFails(t *testing.T) {
	c, _ := NewCipher(testKey(t))
	blob, _ := c.Encrypt("secret")
	blob[len(blob)-1] ^= 0x01
	if _, err := c.Decrypt(blob); err == nil {
		t.Error("Decrypt de blob adulterado deveria falhar")
	}
}

func TestNewCipher_BadKeySize(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Error("NewCipher com chave curta deveria falhar")
	}
}

func TestParseKey(t *testing.T) {
	want := testKey(t)
	// hex (64 chars)
	if got, err := ParseKey(hex.EncodeToString(want)); err != nil || !bytes.Equal(got, want) {
		t.Errorf("ParseKey hex: got %x err %v", got, err)
	}
	// base64 std
	if got, err := ParseKey("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="); err != nil || !bytes.Equal(got, want) {
		t.Errorf("ParseKey base64: got %x err %v", got, err)
	}
	// inválida
	if _, err := ParseKey("muito-curta"); err == nil {
		t.Error("ParseKey de string inválida deveria falhar")
	}
}
