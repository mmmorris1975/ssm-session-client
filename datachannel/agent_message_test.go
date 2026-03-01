package datachannel

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewAgentMessage(t *testing.T) {
	msg := NewAgentMessage()

	if msg.headerLength != agentMsgHeaderLen {
		t.Errorf("headerLength = %d, want %d", msg.headerLength, agentMsgHeaderLen)
	}
	if msg.schemaVersion != 1 {
		t.Errorf("schemaVersion = %d, want 1", msg.schemaVersion)
	}
	if msg.createdDate.IsZero() {
		t.Error("createdDate should not be zero")
	}
	if msg.messageID == uuid.Nil {
		t.Error("messageID should not be nil UUID")
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	original := NewAgentMessage()
	original.MessageType = InputStreamData
	original.Flags = Data
	original.PayloadType = Output
	original.SequenceNumber = 42
	original.Payload = []byte("hello world")

	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	decoded := new(AgentMessage)
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error: %v", err)
	}

	if decoded.MessageType != original.MessageType {
		t.Errorf("MessageType = %q, want %q", decoded.MessageType, original.MessageType)
	}
	if decoded.SequenceNumber != original.SequenceNumber {
		t.Errorf("SequenceNumber = %d, want %d", decoded.SequenceNumber, original.SequenceNumber)
	}
	if decoded.Flags != original.Flags {
		t.Errorf("Flags = %d, want %d", decoded.Flags, original.Flags)
	}
	if decoded.PayloadType != original.PayloadType {
		t.Errorf("PayloadType = %d, want %d", decoded.PayloadType, original.PayloadType)
	}
	if !bytes.Equal(decoded.Payload, original.Payload) {
		t.Errorf("Payload = %q, want %q", decoded.Payload, original.Payload)
	}
}

func TestMarshalUnmarshalEmptyPayload(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 0
	msg.Payload = []byte{}

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	decoded := new(AgentMessage)
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error: %v", err)
	}

	if len(decoded.Payload) != 0 {
		t.Errorf("Payload length = %d, want 0", len(decoded.Payload))
	}
}

func TestMarshalUnmarshalLargePayload(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 1
	msg.Payload = bytes.Repeat([]byte("A"), 1536)

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	decoded := new(AgentMessage)
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error: %v", err)
	}

	if len(decoded.Payload) != 1536 {
		t.Errorf("Payload length = %d, want 1536", len(decoded.Payload))
	}
}

func TestMarshalUnmarshalAllMessageTypes(t *testing.T) {
	types := []MessageType{
		InteractiveShell, TaskReply, TaskComplete,
		Acknowledge, AgentSession,
		OutputStreamData, InputStreamData,
		PausePublication, StartPublication,
	}

	for _, mt := range types {
		t.Run(string(mt), func(t *testing.T) {
			msg := NewAgentMessage()
			msg.MessageType = mt
			msg.Flags = Data
			msg.PayloadType = Output
			msg.SequenceNumber = 1
			msg.Payload = []byte("test")

			data, err := msg.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error: %v", err)
			}

			decoded := new(AgentMessage)
			if err := decoded.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error: %v", err)
			}

			if decoded.MessageType != mt {
				t.Errorf("MessageType = %q, want %q", decoded.MessageType, mt)
			}
		})
	}
}

func TestMarshalUnmarshalAllFlags(t *testing.T) {
	flags := []AgentMessageFlag{Data, Syn, Fin, Ack}

	for _, f := range flags {
		t.Run("flag_"+string(rune('0'+f)), func(t *testing.T) {
			msg := NewAgentMessage()
			msg.MessageType = InputStreamData
			msg.Flags = f
			msg.PayloadType = Output
			msg.SequenceNumber = 1
			msg.Payload = []byte("test")

			data, err := msg.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary() error: %v", err)
			}

			decoded := new(AgentMessage)
			if err := decoded.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary() error: %v", err)
			}

			if decoded.Flags != f {
				t.Errorf("Flags = %d, want %d", decoded.Flags, f)
			}
		})
	}
}

func TestValidateMessage_InvalidHeaderLength(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))

	// Too large header
	msg.headerLength = agentMsgHeaderLen + 10
	if err := msg.ValidateMessage(); err == nil {
		t.Error("expected error for header too large")
	}

	// Too small header
	msg.headerLength = agentMsgHeaderLen - 10
	if err := msg.ValidateMessage(); err == nil {
		t.Error("expected error for header too small")
	}
}

func TestValidateMessage_InvalidSchemaVersion(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))
	msg.schemaVersion = 0

	if err := msg.ValidateMessage(); err == nil {
		t.Error("expected error for invalid schema version")
	}
}

func TestValidateMessage_InvalidMessageType(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = "short"
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))

	if err := msg.ValidateMessage(); err == nil {
		t.Error("expected error for short message type")
	}
}

func TestValidateMessage_ZeroCreatedDate(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))
	msg.createdDate = time.Time{}

	if err := msg.ValidateMessage(); err == nil {
		t.Error("expected error for zero created date")
	}
}

func TestValidateMessage_PayloadLengthMismatch(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = 999

	if err := msg.ValidateMessage(); err == nil {
		t.Error("expected error for payload length mismatch")
	}
}

func TestValidateMessage_PayloadDigestMismatch(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))

	// Corrupt the digest
	msg.payloadDigest = make([]byte, sha256.Size)

	// ValidateMessage logs digest mismatches but doesn't fail
	// (some SSM agent versions may send incorrect digests)
	if err := msg.ValidateMessage(); err != nil {
		t.Errorf("unexpected error for payload digest mismatch: %v", err)
	}
}

func TestValidateMessage_Valid(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = []byte("test")
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))

	if err := msg.ValidateMessage(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvertMessageType_ShortPadding(t *testing.T) {
	msg := &AgentMessage{MessageType: "ack"}
	result := msg.convertMessageType()

	if len(result) != 32 {
		t.Errorf("convertMessageType length = %d, want 32", len(result))
	}
	if string(bytes.TrimRight(result, " ")) != "ack" {
		t.Errorf("convertMessageType content = %q, want %q", bytes.TrimRight(result, " "), "ack")
	}
}

func TestConvertMessageType_ExactLength(t *testing.T) {
	// 32 characters exactly
	msgType := MessageType("12345678901234567890123456789012")
	msg := &AgentMessage{MessageType: msgType}
	result := msg.convertMessageType()

	if len(result) != 32 {
		t.Errorf("convertMessageType length = %d, want 32", len(result))
	}
}

func TestParseMessageType_SpacePadded(t *testing.T) {
	data := make([]byte, 32)
	copy(data, "input_stream_data")
	for i := len("input_stream_data"); i < 32; i++ {
		data[i] = 0x20
	}

	result := parseMessageType(data)
	if result != InputStreamData {
		t.Errorf("parseMessageType = %q, want %q", result, InputStreamData)
	}
}

func TestParseMessageType_NulPadded(t *testing.T) {
	data := make([]byte, 32)
	copy(data, "channel_closed")
	// rest is already 0x00

	result := parseMessageType(data)
	if result != ChannelClosed {
		t.Errorf("parseMessageType = %q, want %q", result, ChannelClosed)
	}
}

func TestParseTime(t *testing.T) {
	// Known timestamp: 1609459200000 ms = 2021-01-01T00:00:00Z
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, 1609459200000)

	result := parseTime(data)
	expected := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	if !result.Equal(expected) {
		t.Errorf("parseTime = %v, want %v", result, expected)
	}
}

func TestFormatUUIDBytes_Involution(t *testing.T) {
	// formatUUIDBytes should be its own inverse when applied twice
	original := make([]byte, 16)
	for i := range original {
		original[i] = byte(i)
	}

	first := formatUUIDBytes(original)
	second := formatUUIDBytes(first)

	if !bytes.Equal(second, original) {
		t.Errorf("formatUUIDBytes is not an involution: got %v, want %v", second, original)
	}
}

func TestSha256PayloadDigest(t *testing.T) {
	msg := &AgentMessage{Payload: []byte("hello")}
	digest := msg.sha256PayloadDigest()

	expected := sha256.Sum256([]byte("hello"))
	if !bytes.Equal(digest, expected[:]) {
		t.Errorf("sha256PayloadDigest mismatch")
	}
}

func TestString(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.SequenceNumber = 42
	msg.PayloadType = Output
	msg.Payload = []byte("test")

	s := msg.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
	if !bytes.Contains([]byte(s), []byte("input_stream_data")) {
		t.Error("String() should contain message type")
	}
	if !bytes.Contains([]byte(s), []byte("42")) {
		t.Error("String() should contain sequence number")
	}
}

func TestUnmarshalBinary_CorruptedData(t *testing.T) {
	// Create valid message first
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 1
	msg.Payload = []byte("test")

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	// Corrupt the payload digest area (bytes 80-112)
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	for i := 80; i < 80+sha256.Size; i++ {
		corrupted[i] = 0xFF
	}

	decoded := new(AgentMessage)
	// UnmarshalBinary succeeds even with corrupted digest
	// (some SSM agent versions may send incorrect digests)
	if err := decoded.UnmarshalBinary(corrupted); err != nil {
		t.Errorf("unexpected error for corrupted digest: %v", err)
	}
}

func TestMarshalBinary_NilPayload(t *testing.T) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 0
	// Leave Payload as nil

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}

	decoded := new(AgentMessage)
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error: %v", err)
	}

	if len(decoded.Payload) != 0 {
		t.Errorf("Payload length = %d, want 0", len(decoded.Payload))
	}
}

func TestChannelClosedMessageRoundTrip(t *testing.T) {
	// channel_closed has 112-byte header (no PayloadType)
	msg := NewAgentMessage()
	msg.headerLength = agentMsgHeaderLen - 4 // 112 bytes
	msg.MessageType = ChannelClosed
	msg.Flags = Data
	msg.SequenceNumber = 0
	msg.Payload = []byte(`{"Output":"session closed"}`)

	// Build wire data manually for a 112-byte header
	msg.sha256PayloadDigest()
	msg.payloadLength = uint32(len(msg.Payload))

	if err := msg.ValidateMessage(); err != nil {
		t.Fatalf("ValidateMessage() error: %v", err)
	}
}
