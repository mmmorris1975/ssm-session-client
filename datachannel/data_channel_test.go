package datachannel

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// newTestWSChannel creates a SsmDataChannel connected to a test WebSocket server
// that reads and discards all messages.
func newTestWSChannel(t *testing.T) (*SsmDataChannel, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial test websocket: %v", err)
	}

	c := &SsmDataChannel{
		ws:        ws,
		inMsgBuf:  NewMessageBuffer(50),
		outMsgBuf: NewMessageBuffer(50),
	}

	cleanup := func() {
		ws.Close()
		srv.Close()
	}

	return c, cleanup
}

func TestStreamEndpointOverride_WithEndpoint(t *testing.T) {
	originalURL := "wss://ssmmessages.us-east-1.amazonaws.com/v1/data-channel/session-id"
	output := &ssm.StartSessionOutput{
		StreamUrl: &originalURL,
	}
	resolver := &SSMMessagesResover{Endpoint: "vpce-123456.ssmmessages.us-east-1.vpce.amazonaws.com"}

	if err := StreamEndpointOverride(resolver, output); err != nil {
		t.Fatalf("StreamEndpointOverride() error: %v", err)
	}

	parsedURL, err := url.Parse(*output.StreamUrl)
	if err != nil {
		t.Fatalf("failed to parse resulting URL: %v", err)
	}

	if parsedURL.Host != "vpce-123456.ssmmessages.us-east-1.vpce.amazonaws.com" {
		t.Errorf("Host = %q, want %q", parsedURL.Host, "vpce-123456.ssmmessages.us-east-1.vpce.amazonaws.com")
	}
	if parsedURL.Scheme != "wss" {
		t.Errorf("Scheme = %q, want %q", parsedURL.Scheme, "wss")
	}
}

func TestStreamEndpointOverride_EmptyEndpoint(t *testing.T) {
	originalURL := "wss://ssmmessages.us-east-1.amazonaws.com/v1/data-channel/session-id"
	output := &ssm.StartSessionOutput{
		StreamUrl: &originalURL,
	}
	resolver := &SSMMessagesResover{Endpoint: ""}

	if err := StreamEndpointOverride(resolver, output); err != nil {
		t.Fatalf("StreamEndpointOverride() error: %v", err)
	}

	if *output.StreamUrl != originalURL {
		t.Errorf("StreamUrl = %q, want %q (unchanged)", *output.StreamUrl, originalURL)
	}
}

func TestStreamEndpointOverride_InvalidURL(t *testing.T) {
	invalidURL := "://invalid-url"
	output := &ssm.StartSessionOutput{
		StreamUrl: &invalidURL,
	}
	resolver := &SSMMessagesResover{Endpoint: "vpce-123456.example.com"}

	if err := StreamEndpointOverride(resolver, output); err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestBuildHandshakeResponse_SessionType(t *testing.T) {
	c := &SsmDataChannel{}
	actions := []RequestedClientAction{
		{
			ActionType: SessionType,
		},
	}

	resp := c.buildHandshakeResponse(actions)

	if resp.ClientVersion == "" {
		t.Error("ClientVersion should not be empty")
	}
	if len(resp.ProcessedClientActions) != 1 {
		t.Fatalf("ProcessedClientActions length = %d, want 1", len(resp.ProcessedClientActions))
	}

	action := resp.ProcessedClientActions[0]
	if action.ActionType != SessionType {
		t.Errorf("ActionType = %q, want %q", action.ActionType, SessionType)
	}
	if action.ActionStatus != Success {
		t.Errorf("ActionStatus = %d, want %d", action.ActionStatus, Success)
	}
}

func TestBuildHandshakeResponse_MultipleActions(t *testing.T) {
	c := &SsmDataChannel{}
	actions := []RequestedClientAction{
		{ActionType: KMSEncryption},
		{ActionType: SessionType},
	}

	resp := c.buildHandshakeResponse(actions)

	if len(resp.ProcessedClientActions) != 2 {
		t.Fatalf("ProcessedClientActions length = %d, want 2", len(resp.ProcessedClientActions))
	}
}

func TestBuildHandshakeResponse_EmptyActions(t *testing.T) {
	c := &SsmDataChannel{}
	resp := c.buildHandshakeResponse(nil)

	if len(resp.ProcessedClientActions) != 0 {
		t.Errorf("ProcessedClientActions length = %d, want 0", len(resp.ProcessedClientActions))
	}
}

func TestHandleMsg_OutputStreamData(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()

	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 0
	msg.Payload = []byte("hello world")

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	payload, err := c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	// Message should be buffered and returned since sequence matches
	if string(payload) != "hello world" {
		t.Errorf("payload = %q, want %q", string(payload), "hello world")
	}
}

func TestHandleMsg_Acknowledge(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()

	msg := NewAgentMessage()
	msg.MessageType = Acknowledge
	msg.Flags = Ack
	msg.PayloadType = Undefined
	msg.SequenceNumber = 5
	msg.Payload = []byte(`{"AcknowledgedMessageType":"input_stream_data"}`)

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	_, err = c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}
}

func TestHandleMsg_PausePublication(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()

	msg := NewAgentMessage()
	msg.MessageType = PausePublication
	msg.Flags = Data
	msg.PayloadType = Undefined
	msg.SequenceNumber = 0
	msg.Payload = []byte{}

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	_, err = c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	if !c.pausePub {
		t.Error("pausePub should be true after PausePublication")
	}
}

func TestHandleMsg_StartPublication(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()
	c.pausePub = true

	msg := NewAgentMessage()
	msg.MessageType = StartPublication
	msg.Flags = Data
	msg.PayloadType = Undefined
	msg.SequenceNumber = 0
	msg.Payload = []byte{}

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	_, err = c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	if c.pausePub {
		t.Error("pausePub should be false after StartPublication")
	}
}

func TestHandleMsg_ChannelClosed(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()

	closedPayload := ChannelClosedPayload{
		MessageType: "channel_closed",
		Output:      "session ended",
	}
	payloadBytes, _ := json.Marshal(closedPayload)

	// Build wire bytes manually since channel_closed has a 112-byte header
	// (no PayloadType field), and MarshalBinary always writes PayloadType.
	msg := NewAgentMessage()
	msg.MessageType = ChannelClosed
	msg.Flags = Data
	msg.SequenceNumber = 0
	msg.PayloadType = Output
	msg.Payload = payloadBytes

	// Use standard marshal, then fix up the header for the 112-byte format
	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	// Patch headerLength to 112 and shift payload data to remove PayloadType field
	binary.BigEndian.PutUint32(data[0:4], agentMsgHeaderLen-4)

	// Remove the 4-byte PayloadType (bytes 112-115) from the wire data
	patched := make([]byte, 0, len(data)-4)
	patched = append(patched, data[:112]...)
	patched = append(patched, data[116:]...)

	output, err := c.HandleMsg(patched)
	if err != io.EOF {
		t.Errorf("HandleMsg() error = %v, want io.EOF", err)
	}
	if string(output) != "session ended" {
		t.Errorf("output = %q, want %q", string(output), "session ended")
	}
}

func TestHandleMsg_HandshakeComplete(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()
	handshakeCh := make(chan bool, 1)
	c.handshakeCh = handshakeCh

	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = HandshakeComplete
	msg.SequenceNumber = 0
	msg.Payload = []byte(`{"HandshakeTimeToComplete":100,"CustomerMessage":"ok"}`)

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	_, err = c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	// Check that handshake channel was closed
	select {
	case _, ok := <-handshakeCh:
		if ok {
			t.Error("handshakeCh should be closed, not have value")
		}
	default:
		t.Error("handshakeCh should be closed")
	}
}

func TestHandleMsg_DuplicateMessage(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()
	c.inSeqNum = 5 // already processed up to 5

	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 3 // already processed
	msg.Payload = []byte("duplicate")

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	payload, err := c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	// Duplicate should be discarded
	if len(payload) != 0 {
		t.Errorf("duplicate payload should be empty, got %q", string(payload))
	}
}

func TestSsmDataChannel_Close_NilWebSocket(t *testing.T) {
	c := &SsmDataChannel{}
	if err := c.Close(); err != nil {
		t.Errorf("Close() with nil ws should not error, got: %v", err)
	}
}

func TestSetTerminalSize_NoChange(t *testing.T) {
	c := &SsmDataChannel{
		lastRows: 24,
		lastCols: 80,
	}

	// Same size should be a no-op (no ws connection to write to)
	if err := c.SetTerminalSize(24, 80); err != nil {
		t.Errorf("SetTerminalSize() with same size should not error: %v", err)
	}
}

func TestHandleMsg_EncryptedOutput(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c.encryptionEnabled = true
	c.decryptKey = key

	plaintext := []byte("encrypted hello")
	encrypted, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 0
	msg.Payload = encrypted

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	payload, err := c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	if string(payload) != "encrypted hello" {
		t.Errorf("payload = %q, want %q", string(payload), "encrypted hello")
	}
}

func TestHandleMsg_EncChallengeRequest(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()

	encryptKey := make([]byte, 32)
	decryptKey := make([]byte, 32)
	for i := range encryptKey {
		encryptKey[i] = byte(i)
		decryptKey[i] = byte(i + 32)
	}
	c.encryptKey = encryptKey
	c.decryptKey = decryptKey

	// Create a challenge encrypted with the decrypt key (simulates agent using its encrypt key)
	challenge := []byte("test-challenge-data")
	encryptedChallenge, err := Encrypt(decryptKey, challenge)
	if err != nil {
		t.Fatalf("Encrypt challenge: %v", err)
	}

	challengeReq := EncryptionChallengeRequest{Challenge: encryptedChallenge}
	challengePayload, err := json.Marshal(challengeReq)
	if err != nil {
		t.Fatalf("Marshal challenge: %v", err)
	}

	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = EncChallengeRequest
	msg.SequenceNumber = 0
	msg.Payload = challengePayload

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	_, err = c.HandleMsg(data)
	if err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	if !c.encryptionEnabled {
		t.Error("encryptionEnabled should be true after successful challenge")
	}
}

func TestBuildHandshakeResponse_KMSEncryption_WithMockKMS(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()
	c.sessionId = "test-session"
	c.targetId = "i-1234567890abcdef0"

	// Use a mock KMS client via the kmsClientOverride
	plainKey := make([]byte, 64)
	for i := range plainKey {
		plainKey[i] = byte(i)
	}
	cipherBlob := []byte("mock-cipher-blob")

	c.kmsClientOverride = &mockKMSClient{
		plaintext:     plainKey,
		ciphertextBlob: cipherBlob,
	}

	actions := []RequestedClientAction{
		{
			ActionType:       KMSEncryption,
			ActionParameters: map[string]interface{}{"KMSKeyId": "arn:aws:kms:us-east-1:123456789012:key/test-key"},
		},
	}

	resp := c.buildHandshakeResponse(actions)

	if len(resp.ProcessedClientActions) != 1 {
		t.Fatalf("ProcessedClientActions length = %d, want 1", len(resp.ProcessedClientActions))
	}

	action := resp.ProcessedClientActions[0]
	if action.ActionType != KMSEncryption {
		t.Errorf("ActionType = %q, want %q", action.ActionType, KMSEncryption)
	}
	if action.ActionStatus != Success {
		t.Errorf("ActionStatus = %d, want %d (error: %s)", action.ActionStatus, Success, action.Error)
	}

	// Verify keys were stored
	if len(c.encryptKey) != 32 {
		t.Errorf("encryptKey length = %d, want 32", len(c.encryptKey))
	}
	if len(c.decryptKey) != 32 {
		t.Errorf("decryptKey length = %d, want 32", len(c.decryptKey))
	}

	// Verify the action result contains the ciphertext key
	var kmsResp KMSEncryptionResponse
	if err := json.Unmarshal(action.ActionResult, &kmsResp); err != nil {
		t.Fatalf("unmarshal KMS response: %v", err)
	}
	if string(kmsResp.KMSCipherTextKey) != "mock-cipher-blob" {
		t.Errorf("KMSCipherTextKey = %q, want %q", kmsResp.KMSCipherTextKey, "mock-cipher-blob")
	}
	if kmsResp.KMSCipherTextHash == "" {
		t.Error("KMSCipherTextHash should not be empty")
	}
}

func TestBuildHandshakeResponse_KMSEncryption_EmptyKeyId(t *testing.T) {
	c := &SsmDataChannel{}
	actions := []RequestedClientAction{
		{
			ActionType:       KMSEncryption,
			ActionParameters: map[string]interface{}{"KMSKeyId": ""},
		},
	}

	resp := c.buildHandshakeResponse(actions)

	action := resp.ProcessedClientActions[0]
	if action.ActionStatus != Failed {
		t.Errorf("ActionStatus = %d, want %d (Failed)", action.ActionStatus, Failed)
	}
}
