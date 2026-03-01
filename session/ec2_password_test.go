package session

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"strings"
	"testing"
)

// generateTestKeyPEM creates a test RSA key pair and returns the private key PEM bytes.
func generateTestKeyPEM(t *testing.T) (privPEM []byte, privKey *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test RSA key: %v", err)
	}
	privPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return privPEM, key
}

func TestParseRSAPrivateKey_PKCS1(t *testing.T) {
	privPEM, originalKey := generateTestKeyPEM(t)

	parsed, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.N.Cmp(originalKey.N) != 0 {
		t.Error("parsed key does not match original")
	}
}

func TestParseRSAPrivateKey_PKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test RSA key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling PKCS8 key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})

	parsed, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.N.Cmp(key.N) != 0 {
		t.Error("parsed PKCS8 key does not match original")
	}
}

func TestParseRSAPrivateKey_InvalidPEM(t *testing.T) {
	_, err := parseRSAPrivateKey([]byte("not a pem block"))
	if err == nil {
		t.Error("expected error for invalid PEM, got nil")
	}
}

func TestGetEC2Password_RoundTrip(t *testing.T) {
	privPEM, privKey := generateTestKeyPEM(t)

	// Simulate AWS: encrypt the password with the public key
	plaintext := "TestPassword123!"
	//nolint:staticcheck,gosec
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, &privKey.PublicKey, []byte(plaintext))
	if err != nil {
		t.Fatalf("encrypting test password: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(encrypted)

	// Write the private key to a temp file
	keyFile, err := os.CreateTemp("", "test-key-*.pem")
	if err != nil {
		t.Fatalf("creating temp key file: %v", err)
	}
	defer os.Remove(keyFile.Name())
	if _, err := keyFile.Write(privPEM); err != nil {
		t.Fatalf("writing temp key file: %v", err)
	}
	keyFile.Close()

	// Test decryption
	key, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		t.Fatalf("parsing key: %v", err)
	}
	encryptedData, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decoding base64: %v", err)
	}
	//nolint:staticcheck,gosec
	result, err := rsa.DecryptPKCS1v15(rand.Reader, key, encryptedData)
	if err != nil {
		t.Fatalf("decrypting: %v", err)
	}
	if string(result) != plaintext {
		t.Errorf("expected %q, got %q", plaintext, string(result))
	}
}

func TestGetEC2Password_MissingKeyFile(t *testing.T) {
	_, err := parseRSAPrivateKey([]byte{})
	if err == nil || !strings.Contains(err.Error(), "decode PEM") {
		t.Errorf("expected PEM decode error, got: %v", err)
	}
}
