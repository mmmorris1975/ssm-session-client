//go:build windows

package ssmclient

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"os/exec"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RDPInput configures the RDP session parameters.
type RDPInput struct {
	LocalPort int
	Username  string
	Password  string
}

// RDPSession launches mstsc.exe to connect to localhost:LocalPort using the provided credentials.
// The password is encrypted with DPAPI (CryptProtectData) and embedded directly in the .rdp file
// as a "password 51:b:" field. This is the same mechanism mstsc.exe uses when saving passwords
// and works reliably regardless of Credential Guard state.
func RDPSession(ctx context.Context, opts *RDPInput) error {
	rdpFile, err := os.CreateTemp("", "ssm-rdp-*.rdp")
	if err != nil {
		return fmt.Errorf("creating temp RDP file: %w", err)
	}
	tmpName := rdpFile.Name()
	defer os.Remove(tmpName)

	rdpContent, err := buildRDPFileContent(opts.LocalPort, opts.Username, opts.Password)
	if err != nil {
		rdpFile.Close()
		return err
	}
	if err := writeUTF16LEFile(rdpFile, rdpContent); err != nil {
		rdpFile.Close()
		return fmt.Errorf("writing RDP file: %w", err)
	}
	rdpFile.Close()

	cmd := exec.CommandContext(ctx, "mstsc.exe", tmpName)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching mstsc.exe: %w", err)
	}

	return cmd.Wait()
}

// encryptRDPPassword encrypts a password using DPAPI for embedding in an .rdp file.
// The password is first encoded as UTF-16 LE (as Windows expects), then encrypted
// with CryptProtectData. The result is returned as an uppercase hex string,
// matching the "password 51:b:" format used by mstsc.exe.
func encryptRDPPassword(password string) (string, error) {
	utf16Password := encodeUTF16LE(password)

	dataIn := windows.DataBlob{
		Size: uint32(len(utf16Password)),
		Data: &utf16Password[0],
	}
	var dataOut windows.DataBlob

	err := windows.CryptProtectData(&dataIn, nil, nil, 0, nil, 0, &dataOut)
	if err != nil {
		return "", fmt.Errorf("DPAPI CryptProtectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(dataOut.Data)))

	encrypted := unsafe.Slice(dataOut.Data, dataOut.Size)
	return strings.ToUpper(hex.EncodeToString(encrypted)), nil
}

// buildRDPFileContent returns the contents of a .rdp file.
// When password is non-empty, it is DPAPI-encrypted and embedded as "password 51:b:".
// When password is empty, no password field is included and mstsc.exe will prompt natively.
func buildRDPFileContent(localPort int, username, password string) (string, error) {
	base := fmt.Sprintf(
		"full address:s:localhost:%d\r\n"+
			"username:s:%s\r\n",
		localPort, username,
	)

	if password != "" {
		encPassword, err := encryptRDPPassword(password)
		if err != nil {
			return "", fmt.Errorf("encrypting RDP password: %w", err)
		}
		base += fmt.Sprintf("password 51:b:%s\r\n", encPassword)
	}

	base += "authentication level:i:0\r\n" +
		"enablecredsspsupport:i:1\r\n" +
		"prompt for credentials:i:1\r\n" +
		"prompt for credentials on client:i:1\r\n"

	return base, nil
}

// encodeUTF16LE encodes a string as UTF-16 LE bytes.
func encodeUTF16LE(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	buf := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}
	return buf
}

// writeUTF16LEFile writes content as UTF-16 LE with BOM to an open file.
// Windows .rdp files must be in UTF-16 LE encoding with a BOM.
func writeUTF16LEFile(f *os.File, content string) error {
	// Write UTF-16 LE BOM (0xFF 0xFE)
	if _, err := f.Write([]byte{0xFF, 0xFE}); err != nil {
		return err
	}

	u16 := utf16.Encode([]rune(content))
	buf := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}

	_, err := f.Write(buf)
	return err
}
