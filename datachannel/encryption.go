package datachannel

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

const (
	nonceSize     = 12 // AES-GCM standard nonce size
	dataKeyLength = 64 // 64 bytes: split into two 32-byte keys
)

// KMSClient is the interface for AWS KMS operations needed for encryption key generation.
type KMSClient interface {
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided key.
// Returns [12-byte nonce][ciphertext + 16-byte GCM tag].
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt using AES-256-GCM.
// Expects input format: [12-byte nonce][ciphertext + 16-byte GCM tag].
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	if len(ciphertext) < nonceSize+gcm.Overhead() {
		return nil, errors.New("ciphertext too short")
	}

	nonce := ciphertext[:nonceSize]
	encrypted := ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

// GenerateEncryptionKeys calls KMS GenerateDataKey to produce a 64-byte data key,
// then splits it into two 32-byte halves:
//   - decryptKey (first 32 bytes): used to decrypt data from the agent
//   - encryptKey (second 32 bytes): used to encrypt data sent to the agent
//
// It also returns the ciphertext blob (encrypted data key) to send back to the agent.
func GenerateEncryptionKeys(client KMSClient, kmsKeyID, sessionID, targetID string) (encryptKey, decryptKey, ciphertextKey []byte, err error) {
	input := &kms.GenerateDataKeyInput{
		KeyId:         aws.String(kmsKeyID),
		NumberOfBytes: aws.Int32(dataKeyLength),
		EncryptionContext: map[string]string{
			"aws:ssm:SessionId": sessionID,
			"aws:ssm:TargetId":  targetID,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := client.GenerateDataKey(ctx, input)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("KMS GenerateDataKey: %w", err)
	}

	if len(output.Plaintext) != dataKeyLength {
		return nil, nil, nil, fmt.Errorf("expected %d byte data key, got %d", dataKeyLength, len(output.Plaintext))
	}

	decryptKey = make([]byte, 32)
	encryptKey = make([]byte, 32)
	copy(decryptKey, output.Plaintext[:32])
	copy(encryptKey, output.Plaintext[32:])

	return encryptKey, decryptKey, output.CiphertextBlob, nil
}

// ciphertextKeyHash returns the base64-encoded SHA-256 hash of the ciphertext key,
// used in the KMSEncryptionResponse.
func ciphertextKeyHash(ciphertextKey []byte) string {
	hash := sha256.Sum256(ciphertextKey)
	return base64.StdEncoding.EncodeToString(hash[:])
}
