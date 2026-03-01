package datachannel

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// mockKMSClient implements KMSClient for testing.
type mockKMSClient struct {
	plaintext      []byte
	ciphertextBlob []byte
	err            error
}

func (m *mockKMSClient) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &kms.GenerateDataKeyOutput{
		Plaintext:      m.plaintext,
		CiphertextBlob: m.ciphertextBlob,
	}, nil
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("hello, encrypted world!")
	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 1)
	}

	plaintext := []byte("secret data")
	ciphertext, err := Encrypt(key1, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	_, err = Decrypt(key2, ciphertext)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	key := make([]byte, 32)

	// Too short to contain nonce + tag
	_, err := Decrypt(key, []byte("short"))
	if err == nil {
		t.Error("Decrypt() with truncated ciphertext should fail")
	}
}

func TestEncrypt_NonceRandomness(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("same plaintext")
	c1, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("first Encrypt() error: %v", err)
	}

	c2, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("second Encrypt() error: %v", err)
	}

	if bytes.Equal(c1, c2) {
		t.Error("two encryptions of same plaintext should produce different ciphertext (different nonces)")
	}

	// But both should decrypt to the same plaintext
	d1, _ := Decrypt(key, c1)
	d2, _ := Decrypt(key, c2)
	if !bytes.Equal(d1, d2) {
		t.Error("both ciphertexts should decrypt to the same plaintext")
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	key := make([]byte, 15) // invalid AES key size
	_, err := Encrypt(key, []byte("data"))
	if err == nil {
		t.Error("Encrypt() with invalid key size should fail")
	}
}

func TestDecrypt_InvalidKeySize(t *testing.T) {
	key := make([]byte, 15) // invalid AES key size
	_, err := Decrypt(key, make([]byte, 40))
	if err == nil {
		t.Error("Decrypt() with invalid key size should fail")
	}
}

func TestGenerateEncryptionKeys_Success(t *testing.T) {
	plainKey := make([]byte, 64)
	for i := range plainKey {
		plainKey[i] = byte(i)
	}
	cipherBlob := []byte("encrypted-key-blob")

	client := &mockKMSClient{
		plaintext:      plainKey,
		ciphertextBlob: cipherBlob,
	}

	encryptKey, decryptKey, ciphertextKey, err := GenerateEncryptionKeys(client, "key-id", "session-id", "target-id")
	if err != nil {
		t.Fatalf("GenerateEncryptionKeys() error: %v", err)
	}

	// Verify key split: first 32 bytes -> decrypt key, second 32 bytes -> encrypt key
	if !bytes.Equal(decryptKey, plainKey[:32]) {
		t.Error("decryptKey should be first 32 bytes of plaintext")
	}
	if !bytes.Equal(encryptKey, plainKey[32:]) {
		t.Error("encryptKey should be second 32 bytes of plaintext")
	}
	if !bytes.Equal(ciphertextKey, cipherBlob) {
		t.Error("ciphertextKey should match ciphertext blob from KMS")
	}
}

func TestGenerateEncryptionKeys_KMSError(t *testing.T) {
	client := &mockKMSClient{
		err: fmt.Errorf("KMS error"),
	}

	_, _, _, err := GenerateEncryptionKeys(client, "key-id", "session-id", "target-id")
	if err == nil {
		t.Error("GenerateEncryptionKeys() should fail on KMS error")
	}
}

func TestGenerateEncryptionKeys_WrongKeyLength(t *testing.T) {
	client := &mockKMSClient{
		plaintext:      make([]byte, 32), // wrong: should be 64
		ciphertextBlob: []byte("blob"),
	}

	_, _, _, err := GenerateEncryptionKeys(client, "key-id", "session-id", "target-id")
	if err == nil {
		t.Error("GenerateEncryptionKeys() should fail with wrong key length")
	}
}

func TestChallengeFlow(t *testing.T) {
	// Simulate the full challenge flow:
	// 1. Agent encrypts challenge with agent's encrypt key (= client's decrypt key)
	// 2. Client decrypts with decrypt key
	// 3. Client re-encrypts with encrypt key
	// 4. Agent decrypts with agent's decrypt key (= client's encrypt key)
	encryptKey := make([]byte, 32)
	decryptKey := make([]byte, 32)
	for i := range encryptKey {
		encryptKey[i] = byte(i)
		decryptKey[i] = byte(i + 32)
	}

	challenge := []byte("challenge-data-12345")

	// Agent encrypts with client's decrypt key
	agentEncrypted, err := Encrypt(decryptKey, challenge)
	if err != nil {
		t.Fatalf("agent Encrypt: %v", err)
	}

	// Client decrypts with decrypt key
	decrypted, err := Decrypt(decryptKey, agentEncrypted)
	if err != nil {
		t.Fatalf("client Decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, challenge) {
		t.Fatalf("decrypted challenge = %q, want %q", decrypted, challenge)
	}

	// Client re-encrypts with encrypt key
	reEncrypted, err := Encrypt(encryptKey, decrypted)
	if err != nil {
		t.Fatalf("client re-Encrypt: %v", err)
	}

	// Agent decrypts with client's encrypt key
	agentDecrypted, err := Decrypt(encryptKey, reEncrypted)
	if err != nil {
		t.Fatalf("agent Decrypt: %v", err)
	}

	if !bytes.Equal(agentDecrypted, challenge) {
		t.Errorf("agent decrypted = %q, want %q", agentDecrypted, challenge)
	}
}

func TestCiphertextKeyHash(t *testing.T) {
	key := []byte("test-ciphertext-key")
	hash := ciphertextKeyHash(key)
	if hash == "" {
		t.Error("ciphertextKeyHash should not return empty string")
	}

	// Same input should produce same hash
	hash2 := ciphertextKeyHash(key)
	if hash != hash2 {
		t.Error("ciphertextKeyHash should be deterministic")
	}

	// Different input should produce different hash
	hash3 := ciphertextKeyHash([]byte("different-key"))
	if hash == hash3 {
		t.Error("different inputs should produce different hashes")
	}
}
