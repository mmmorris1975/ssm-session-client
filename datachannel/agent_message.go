package datachannel

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

const agentMsgHeaderLen = 116 // the binary size of all AgentMessage fields except payloadLength and Payload

// AgentMessage is the structural representation of the binary format of an SSM agent message use for communication
// between local clients (like this), and remote agents installed on EC2 instances.
// This is the order the fields must appear as on the wire
// REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/contracts/agentmessage.go.
//nolint:maligned
type AgentMessage struct {
	headerLength   uint32
	MessageType    MessageType // this is a 32 byte space-padded string on the wire
	schemaVersion  uint32
	createdDate    time.Time // wire format is milliseconds since unix epoch (uint64), value set to time.Now() in NewAgentMessage
	SequenceNumber int64
	Flags          AgentMessageFlag // REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/contracts/agentmessage.go
	messageID      uuid.UUID        // 16 byte UUID, auto-generated in NewAgentMessage
	payloadDigest  []byte           // SHA256 digest, value calculated in MarshalBinary
	PayloadType    PayloadType      // REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/contracts/model.go
	payloadLength  uint32           // value calculated in MarshalBinary
	Payload        []byte
}

// NewAgentMessage creates an AgentMessage ready to load with payload.
func NewAgentMessage() *AgentMessage {
	return &AgentMessage{
		headerLength:  agentMsgHeaderLen,
		schemaVersion: 1,
		createdDate:   time.Now(),
		messageID:     uuid.New(),
	}
}

// ValidateMessage performs checks on the values of the AgentMessage to ensure they are sane.
func (m *AgentMessage) ValidateMessage() error {
	// close_channel message header is 112 bytes
	if m.headerLength > agentMsgHeaderLen || m.headerLength < agentMsgHeaderLen-4 {
		return errors.New("invalid message header length")
	}

	if m.schemaVersion < 1 {
		return errors.New("invalid schema version")
	}

	// this seems to be a good minimum number after checking the SSM agent source code
	if len(m.MessageType) < 10 {
		return errors.New("invalid message type")
	}

	if m.createdDate.IsZero() {
		return errors.New("invalid message date")
	}

	if len(m.messageID[:]) != 16 {
		return errors.New("invalid message id")
	}

	if len(m.Payload) != int(m.payloadLength) {
		return fmt.Errorf("payload length mismatch, WANT: %d, GOT: %d", m.payloadLength, len(m.Payload))
	}

	if !bytes.Equal(m.sha256PayloadDigest(), m.payloadDigest) {
		return errors.New("payload digest mismatch")
	}

	return nil
}

// UnmarshalBinary reads the wire format data and updates the fields in the method receiver.  Satisfies the
// encoding.BinaryUnmarshaler interface.
func (m *AgentMessage) UnmarshalBinary(data []byte) error {
	m.headerLength = binary.BigEndian.Uint32(data)
	m.MessageType = parseMessageType(data[4:36])
	m.schemaVersion = binary.BigEndian.Uint32(data[36:40])
	m.createdDate = parseTime(data[40:48])
	m.SequenceNumber = int64(binary.BigEndian.Uint64(data[48:56]))
	m.Flags = AgentMessageFlag(binary.BigEndian.Uint64(data[56:64]))
	m.messageID = uuid.Must(uuid.FromBytes(formatUUIDBytes(data[64:80])))
	m.payloadDigest = data[80 : 80+sha256.Size]

	// The channel_closed message has a header length of 112 bytes, assuming this is what's dropped
	if m.headerLength == agentMsgHeaderLen {
		m.PayloadType = PayloadType(binary.BigEndian.Uint32(data[112:m.headerLength]))
	}

	payloadLenEnd := m.headerLength + 4
	m.payloadLength = binary.BigEndian.Uint32(data[m.headerLength:payloadLenEnd])
	m.Payload = data[payloadLenEnd : payloadLenEnd+m.payloadLength]

	return m.ValidateMessage()
}

// MarshalBinary converts the fields in the method receiver to the expected wire format used by the websocket
// protocol with the SSM messaging service.  Satisfies the encoding.BinaryMarshaler interface.
func (m *AgentMessage) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	m.sha256PayloadDigest()
	m.payloadLength = uint32(len(m.Payload))

	if err := m.ValidateMessage(); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, m.headerLength); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.convertMessageType()); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.schemaVersion); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, time.Duration(m.createdDate.UnixNano()).Milliseconds()); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.SequenceNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.Flags); err != nil {
		return nil, err
	}
	// []byte values are written directly (no endian-ness), but for consistency's sake ...
	if err := binary.Write(buf, binary.BigEndian, formatUUIDBytes(m.messageID[:])); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.payloadDigest[:sha256.Size]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.PayloadType); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.payloadLength); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.Payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (m *AgentMessage) String() string {
	sb := new(strings.Builder)
	sb.WriteString("AgentMessage{")
	sb.WriteString(fmt.Sprintf("TYPE: %s, ", m.MessageType))
	sb.WriteString(fmt.Sprintf("SCHEMA VERSION: %d, ", m.schemaVersion))
	sb.WriteString(fmt.Sprintf("SEQUENCE: %d, ", m.SequenceNumber))
	sb.WriteString(fmt.Sprintf("MESSAGE ID: %s, ", m.messageID))
	sb.WriteString(fmt.Sprintf("PAYLOAD TYPE: %d, ", m.PayloadType))
	sb.WriteString(fmt.Sprintf("PAYLOAD LENGTH: %d", m.payloadLength))
	sb.WriteString(fmt.Sprintln("}"))
	return sb.String()
}

func (m *AgentMessage) convertMessageType() []byte {
	var msgTypeLen = 32 // per spec
	var msgType []byte

	if len(m.MessageType) >= msgTypeLen {
		msgType = []byte(m.MessageType)
	} else {
		msgType = []byte(m.MessageType)
		msgType = append(msgType, bytes.Repeat([]byte{0x20}, msgTypeLen-len(m.MessageType))...)
	}

	return msgType[:msgTypeLen]
}

func (m *AgentMessage) sha256PayloadDigest() []byte {
	digest := sha256.New()
	_, _ = digest.Write(m.Payload)
	m.payloadDigest = digest.Sum(nil)
	return m.payloadDigest
}

// channel_closed message type is nul padded, others are space padded.  Handle both.
func parseMessageType(data []byte) MessageType {
	return MessageType(bytes.TrimSpace(bytes.TrimRight(data, string(rune(0x00)))))
}

func parseTime(data []byte) time.Time {
	ts := binary.BigEndian.Uint64(data)
	d := time.Duration(ts) * time.Millisecond
	return time.Unix(0, d.Nanoseconds())
}

func formatUUIDBytes(data []byte) []byte {
	return append(data[8:], data[:8]...)
}
