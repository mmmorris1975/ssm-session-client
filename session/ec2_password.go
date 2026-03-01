package session

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// GetEC2Password retrieves and decrypts the administrator password for a Windows EC2 instance.
// The privateKeyPath must be the path to the RSA private key (.pem) associated with the instance's key pair.
func GetEC2Password(ctx context.Context, cfg aws.Config, instanceID, privateKeyPath string) (string, error) {
	client := ec2.NewFromConfig(cfg)

	out, err := client.GetPasswordData(ctx, &ec2.GetPasswordDataInput{
		InstanceId: aws.String(instanceID),
	})
	if err != nil {
		return "", fmt.Errorf("getting EC2 password data: %w", err)
	}

	if out.PasswordData == nil || *out.PasswordData == "" {
		return "", fmt.Errorf("password not yet available for instance %s (instance may be Linux or still initializing)", instanceID)
	}

	encryptedData, err := base64.StdEncoding.DecodeString(*out.PasswordData)
	if err != nil {
		return "", fmt.Errorf("decoding password data: %w", err)
	}

	keyPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("reading private key file: %w", err)
	}

	privateKey, err := parseRSAPrivateKey(keyPEM)
	if err != nil {
		return "", err
	}

	// AWS GetPasswordData mandates PKCS#1 v1.5; OAEP is not supported.
	// DecryptPKCS1v15SessionKey cannot replace this because EC2 passwords are variable-length.
	// The rand parameter is documented as legacy and ignored; nil is correct here.
	//nolint:staticcheck
	plaintext, err := rsa.DecryptPKCS1v15(nil, privateKey, encryptedData)
	if err != nil {
		return "", fmt.Errorf("decrypting EC2 password: %w", err)
	}

	return string(plaintext), nil
}

func parseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key file")
	}

	// Try PKCS1 first (traditional RSA key format)
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Fall back to PKCS8 (newer format)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not an RSA key")
	}
	return rsaKey, nil
}
