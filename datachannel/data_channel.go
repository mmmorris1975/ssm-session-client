package datachannel

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// DataChannel is the interface definition for handling communication with the AWS SSM messaging service.
type DataChannel interface {
	Open(aws.Config, *ssm.StartSessionInput, *SSMMessagesResover) error
	HandleMsg(data []byte) ([]byte, error)
	SetTerminalSize(rows, cols uint32) error
	TerminateSession() error
	DisconnectPort() error
	WriteMsg(*AgentMessage) (int, error)
	io.ReadWriteCloser
	io.ReaderFrom
	io.WriterTo
}

// SsmDataChannel represents the data channel of the websocket connection used to communicate with the AWS
// SSM service.  A new(SsmDataChannel) is ready for use, and should immediately call the Open() method.
type SsmDataChannel struct {
	seqNum      int64
	inSeqNum    int64
	mu          sync.Mutex
	ws          *websocket.Conn
	synSent     bool
	handshakeCh chan bool
	pausePub    bool
	outMsgBuf   MessageBuffer
	inMsgBuf    MessageBuffer
	lastRows    uint32
	lastCols    uint32

	// KMS encryption state
	encryptionEnabled bool
	encryptKey        []byte     // 32 bytes - for encrypting outbound data
	decryptKey        []byte     // 32 bytes - for decrypting inbound data
	cfg               aws.Config // stored for KMS client creation
	sessionId         string     // from StartSessionOutput
	targetId          string     // from StartSessionInput.Target
	kmsClientOverride KMSClient  // for testing; if set, used instead of creating from cfg

	// Agent version from handshake
	agentVersion string

	// Reconnection state
	reconnectEnabled bool
	maxReconnects    int
	reconnectCount   int
	ssmClient        *ssm.Client
}

func StreamEndpointOverride(resolver *SSMMessagesResover, output *ssm.StartSessionOutput) error {
	//get the endpoint from the config

	if resolver.Endpoint != "" {
		//replace the hostname part of the stream url with the vpc endpoint
		parsedUrl, err := url.Parse(*output.StreamUrl)
		if err != nil {
			return err
		}
		parsedUrl.Host = resolver.Endpoint
		newStreamUrl := parsedUrl.String()
		output.StreamUrl = &newStreamUrl
	}
	return nil
}

// Open creates the web socket connection with the AWS service and opens the data channel.
func (c *SsmDataChannel) Open(cfg aws.Config, in *ssm.StartSessionInput, resolver *SSMMessagesResover) error {
	c.handshakeCh = make(chan bool, 1)
	c.outMsgBuf = NewMessageBuffer(50)
	c.inMsgBuf = NewMessageBuffer(50)
	c.cfg = cfg
	if in.Target != nil {
		c.targetId = *in.Target
	}

	go c.processOutboundQueue()

	return c.startSession(cfg, in, resolver)
}

// Close shuts down the web socket connection with the AWS service. Type-specific actions (like sending
// TerminateSession for port forwarding should be handled before calling Close().
func (c *SsmDataChannel) Close() error {
	var err error
	if c.ws != nil {
		err = c.ws.Close()
	}
	return err
}

// AgentVersion returns the version string reported by the SSM agent during handshake.
// This is used to determine which features are supported (e.g., connection multiplexing).
func (c *SsmDataChannel) AgentVersion() string {
	return c.agentVersion
}

// WaitForHandshakeComplete blocks further processing until the required SSM handshake sequence used for
// port-based clients (including ssh) completes.
func (c *SsmDataChannel) WaitForHandshakeComplete(ctx context.Context) error {
	buf := make([]byte, 4096)

	for {
		select {
		case <-c.handshakeCh:
			// make stream unbuffered
			c.inMsgBuf = nil
			c.outMsgBuf = nil
			c.handshakeCh = nil
			zap.S().Debug("handshake complete")
			return nil
		case <-ctx.Done():
			c.inMsgBuf = nil
			c.outMsgBuf = nil
			c.handshakeCh = nil
			return context.Canceled
		default:
			n, err := c.Read(buf)
			if err != nil {
				return err
			}

			m := new(AgentMessage)
			if uerr := m.UnmarshalBinary(buf[:n]); uerr == nil {
				zap.S().Debugf("handshake recv: type=%s flags=%d seq=%d payloadType=%d len=%d",
					m.MessageType, m.Flags, m.SequenceNumber, m.PayloadType, len(m.Payload))
			}

			if _, err = c.HandleMsg(buf[:n]); err != nil {
				return err
			}
		}
	}
}

// Read will get a single message from the websocket connection. The unprocessed message is copied to the
// requested []byte (which should be sized to handle at least 1536 bytes).
func (c *SsmDataChannel) Read(data []byte) (int, error) {
	_, msg, err := c.ws.ReadMessage()
	n := copy(data[:len(msg)], msg)

	if err != nil {
		// gorilla code states this is uber-fatal, and we just need to bail out
		if websocket.IsCloseError(err, 1000, 1001, 1006) {
			err = io.EOF
		}
		return n, err
	}

	if n < agentMsgHeaderLen {
		return n, errors.New("invalid message received, too short")
	}

	return n, nil
}

// WriteTo uses the data channel as an io.Copy read source, writing output to the provided writer.
func (c *SsmDataChannel) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4096)
	var nr, nw int
	var payload []byte

	for {
		nr, err = c.Read(buf)
		if err != nil {
			zap.S().Debugf("WriteTo read error: %v", err)
			return n, err
		}

		if nr > 0 {
			payload, err = c.HandleMsg(buf[:nr])
			var isEOF bool
			if err != nil {
				if errors.Is(err, io.EOF) {
					isEOF = true
				} else {
					zap.S().Infof("WriteTo HandleMsg error: %v", err)
					return n, err
				}
			}

			if len(payload) > 0 {
				nw, err = w.Write(payload)
				n += int64(nw)
				if err != nil {
					zap.S().Infof("WriteTo write error: %v", err)
					return n, err
				}
			}

			if isEOF {
				return n, nil
			}
		}
	}
}

// ReadFrom uses the data channel as an io.Copy write destination, reading data from the provided reader.
func (c *SsmDataChannel) ReadFrom(r io.Reader) (n int64, err error) {
	buf := make([]byte, 1536) // 1536 appears to be a default websocket max packet size
	var nr int

	for {
		nr, err = r.Read(buf)
		n += int64(nr)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// the contract of ReaderFrom states that io.EOF should not be returned, just
				// exit the loop and return no error to indicate we are done
				err = nil
				zap.S().Debug("ReadFrom reader is closed")
			}
			break
		}

		if _, err = c.Write(buf[:nr]); err != nil {
			zap.S().Infof("ReadFrom write error: %v", err)
			break
		}
	}
	return
}

// Write sends an input stream data message type with the provided payload bytes as the message payload.
func (c *SsmDataChannel) Write(payload []byte) (int, error) {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)

	if c.encryptionEnabled {
		encrypted, err := Encrypt(c.encryptKey, payload)
		if err != nil {
			return 0, fmt.Errorf("encrypt payload: %w", err)
		}
		msg.Payload = encrypted
	} else {
		msg.Payload = payload
	}

	return c.WriteMsg(msg)
}

// WriteMsg is the underlying method which marshals AgentMessage types and sends them to the AWS service.
// This is provided as a convenience so that messages types not already handled can be sent. If the message
// SequenceNumber field is less than 0, it will be automatically incremented using the internal counter.
func (c *SsmDataChannel) WriteMsg(msg *AgentMessage) (int, error) {
	// Only set the SYN flag on the first non-Acknowledge message.
	// Acks (e.g. for StartPublication) must keep their Ack flag — Windows SSM agents
	// reject SYN-flagged acks and the handshake stalls.
	if !c.synSent && msg.MessageType != Acknowledge {
		atomic.StoreInt64(&c.seqNum, 0)
		msg.Flags = Syn
		msg.SequenceNumber = c.seqNum
	}

	if msg.SequenceNumber < 0 {
		atomic.StoreInt64(&c.seqNum, 1)
	}

	zap.S().Debugf("WriteMsg: type=%s flags=%d seq=%d payloadType=%d len=%d",
		msg.MessageType, msg.Flags, msg.SequenceNumber, msg.PayloadType, len(msg.Payload))

	data, err := msg.MarshalBinary()
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if msg.MessageType != Acknowledge {
		c.synSent = true
	}

	if c.outMsgBuf != nil && msg.MessageType != Acknowledge && msg.PayloadType != HandshakeResponse {
		err = c.outMsgBuf.Add(msg)
	}

	if !c.pausePub {
		return int(msg.payloadLength), c.ws.WriteMessage(websocket.BinaryMessage, data)
	}
	return int(msg.payloadLength), err
}

// HandleMsg takes the unprocessed message bytes from the websocket connection (a la Read()), unmarshals the data
// and takes the appropriate action based on the message type.  Messages which have an actionable payload (output
// payload types, and channel closed payloads) will have that data returned.  Errors will be returned for unknown/
// unhandled message or payload types.  A ChannelClosed message type will return an io.EOF error to indicate that
// this SSM data channel is shutting down and should no longer be used.
//
//nolint:gocognit,gocyclo
func (c *SsmDataChannel) HandleMsg(data []byte) ([]byte, error) {
	m := new(AgentMessage)
	if err := m.UnmarshalBinary(data); err != nil {
		// validation error
		return nil, err
	}

	//nolint:exhaustive // we'll add more as we find them
	switch m.MessageType {
	case Acknowledge:
		if c.outMsgBuf != nil {
			c.outMsgBuf.Remove(m.SequenceNumber)
		}
	case PausePublication:
		c.pausePub = true
	case StartPublication:
		c.pausePub = false
	case OutputStreamData:
		switch m.PayloadType {
		case Output:
			payload := m.Payload
			if c.encryptionEnabled {
				decrypted, err := Decrypt(c.decryptKey, payload)
				if err != nil {
					return nil, fmt.Errorf("decrypt payload: %w", err)
				}
				payload = decrypted
			}

			// unbuffered - return payload directly
			if c.inMsgBuf == nil {
				// Discard duplicate/retransmitted messages to prevent
				// data corruption in the output stream. The SSM agent may
				// retransmit before our ack arrives under heavy traffic.
				lastSeen := atomic.LoadInt64(&c.inSeqNum)
				if m.SequenceNumber <= lastSeen && lastSeen > 0 {
					if err := c.sendAcknowledgeMessage(m); err != nil {
						zap.S().Warnf("failed to send acknowledge: %v", err)
					}
					return nil, nil
				}
				atomic.StoreInt64(&c.inSeqNum, m.SequenceNumber)
				if err := c.sendAcknowledgeMessage(m); err != nil {
					zap.S().Warnf("failed to send acknowledge: %v", err)
				}
				return payload, nil
			}

			// duplicate message - discard
			if m.SequenceNumber < c.inSeqNum {
				return nil, nil
			}

			// store decrypted payload back for buffered path
			m.Payload = payload

			// queue everything else
			if err := c.inMsgBuf.Add(m); err != nil {
				return nil, err
			}
		case EncChallengeRequest:
			if err := c.processEncryptionChallenge(m); err != nil {
				return nil, fmt.Errorf("encryption challenge: %w", err)
			}
		case HandshakeRequest:
			// port forwarding session setup, we'll consider a handshake failure fatal
			if err := c.processHandshakeRequest(m); err != nil {
				return nil, err
			}
		case HandshakeComplete:
			if c.handshakeCh != nil {
				close(c.handshakeCh)
				// Do NOT nil handshakeCh here: WaitForHandshakeComplete detects
				// completion via "case <-c.handshakeCh" and needs it non-nil.
			}
			// Switch to unbuffered mode so subsequent Output messages are
			// delivered directly rather than queued waiting for seq=0.
			c.inMsgBuf = nil
			c.outMsgBuf = nil
		default:
			zap.S().Debugf("ignoring unknown payload type %d for OutputStreamData seq=%d", m.PayloadType, m.SequenceNumber)
		}
	case ChannelClosed:
		payload := new(ChannelClosedPayload)
		if err := json.Unmarshal(m.Payload, payload); err != nil {
			return nil, err
		}

		if payload.Output != "" {
			zap.S().Infof("session closed: %s", payload.Output)
		}

		var output []byte
		if len(payload.Output) > 0 {
			output = []byte(payload.Output)
		}
		return output, io.EOF
	default:
		zap.S().Debugf("ignoring unknown message type: %s seq=%d", m.MessageType, m.SequenceNumber)
		return nil, nil
	}

	if err := c.sendAcknowledgeMessage(m); err != nil {
		zap.S().Warnf("failed to send acknowledge for seq %d: %v", m.SequenceNumber, err)
		return nil, err
	}

	return c.processInboundQueue()
}

// SetTerminalSize sends a message to the SSM service which indicates the size to use for the remote terminal
// when using a shell session client.
func (c *SsmDataChannel) SetTerminalSize(rows, cols uint32) error {
	if c.lastRows == rows && c.lastCols == cols {
		// skip if terminal size is unchanged
		return nil
	}

	input := map[string]uint32{
		"rows": rows,
		"cols": cols,
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}

	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.Flags = Data
	msg.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)
	msg.PayloadType = Size
	msg.Payload = payload

	// Remind our future selves what the last-set values were:
	c.lastRows = rows
	c.lastCols = cols

	_, err = c.WriteMsg(msg)
	return err
}

// TerminateSession sends the TerminateSession message to the AWS service to indicate that the port forwarding
// session is ending, so it can clean up any connections used to communicate with the EC2 instance agent.
func (c *SsmDataChannel) TerminateSession() error {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)
	msg.Flags = Fin
	msg.PayloadType = Flag

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(TerminateSession))
	msg.Payload = buf

	_, err := c.WriteMsg(msg)
	return err
}

// DisconnectPort sends the DisconnectToPort message to the AWS service to indicate that a non-muxing stream is
// shutting down and any connection used to communicate with the EC2 instance agent can be cleaned up.  Unlike
// the TerminateSession action, the websocket connection is still capable of initiating a new port forwarding
// stream to the agent without needing to restart the program.
func (c *SsmDataChannel) DisconnectPort() error {
	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)
	msg.Flags = Data
	msg.PayloadType = Flag

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(DisconnectToPort))
	msg.Payload = buf

	_, err := c.WriteMsg(msg)
	return err
}

func (c *SsmDataChannel) processInboundQueue() ([]byte, error) {
	if c.inMsgBuf == nil {
		return nil, nil
	}

	var err error
	data := new(bytes.Buffer)

	for {
		if msg := c.inMsgBuf.Get(c.inSeqNum); msg != nil {
			atomic.AddInt64(&c.inSeqNum, 1)

			if _, err = data.Write(msg.Payload); err != nil {
				break
			}

			c.inMsgBuf.Remove(msg.SequenceNumber)
		} else {
			break
		}
	}

	return data.Bytes(), err
}

func (c *SsmDataChannel) processOutboundQueue() {
	backoff := 500 * time.Millisecond
	const (
		minBackoff = 500 * time.Millisecond
		maxBackoff = 30 * time.Second
	)

	for {
		time.Sleep(backoff)
		if c.pausePub {
			continue
		}

		if c.outMsgBuf == nil {
			return
		}

		hasMessages := false
		for m := c.outMsgBuf.Next(); m != nil; m = c.outMsgBuf.Next() {
			hasMessages = true
			if _, err := c.WriteMsg(m); err != nil {
				zap.S().Warnf("failed to retransmit message seq %d: %v", m.SequenceNumber, err)
			}
		}

		// Exponential backoff: double when messages are pending, reset when queue is empty
		if hasMessages {
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = minBackoff
		}
	}
}

// sendAcknowledgeMessage sends the Acknowledge message type for each incoming message read from
// the web socket connection, which is required as part of the SSM session protocol.
func (c *SsmDataChannel) sendAcknowledgeMessage(msg *AgentMessage) error {
	zap.S().Debugf("sendAck: for type=%s seq=%d msgId=%s",
		msg.MessageType, msg.SequenceNumber, msg.messageID.String())

	ack := map[string]interface{}{
		"AcknowledgedMessageType":           msg.MessageType,
		"AcknowledgedMessageId":             msg.messageID.String(),
		"AcknowledgedMessageSequenceNumber": msg.SequenceNumber,
		"IsSequentialMessage":               true,
	}

	payload, err := json.Marshal(ack)
	if err != nil {
		return err
	}

	agentMsg := NewAgentMessage()
	agentMsg.MessageType = Acknowledge
	agentMsg.SequenceNumber = msg.SequenceNumber
	agentMsg.Flags = Ack
	agentMsg.PayloadType = Undefined
	agentMsg.Payload = payload

	_, err = c.WriteMsg(agentMsg)
	return err
}

// processHandshakeRequest handles the incoming handshake request message for a port forwarding session
// and sends the required HandshakeResponse message.  This must complete before sending data over the
// forwarded connection.
func (c *SsmDataChannel) processHandshakeRequest(msg *AgentMessage) error {
	req := new(HandshakeRequestPayload)
	if err := json.Unmarshal(msg.Payload, req); err != nil {
		return err
	}

	// Store agent version for later use in connection multiplexing decisions
	c.agentVersion = req.AgentVersion

	resp := c.buildHandshakeResponse(req.RequestedClientActions)
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	out := NewAgentMessage()
	out.MessageType = InputStreamData
	out.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)
	out.Flags = Data
	out.PayloadType = HandshakeResponse
	out.Payload = payload

	_, err = c.WriteMsg(out)
	return err
}

// processEncryptionChallenge handles the EncChallengeRequest from the agent.
// It decrypts the challenge with the decrypt key, re-encrypts with the encrypt key,
// and sends the response. After success, encryption is enabled for all subsequent data.
func (c *SsmDataChannel) processEncryptionChallenge(msg *AgentMessage) error {
	var challengeReq EncryptionChallengeRequest
	if err := json.Unmarshal(msg.Payload, &challengeReq); err != nil {
		return fmt.Errorf("unmarshal challenge request: %w", err)
	}

	decrypted, err := Decrypt(c.decryptKey, challengeReq.Challenge)
	if err != nil {
		return fmt.Errorf("decrypt challenge: %w", err)
	}

	reEncrypted, err := Encrypt(c.encryptKey, decrypted)
	if err != nil {
		return fmt.Errorf("re-encrypt challenge: %w", err)
	}

	challengeResp := EncryptionChallengeResponse{
		Challenge: reEncrypted,
	}

	payload, err := json.Marshal(challengeResp)
	if err != nil {
		return fmt.Errorf("marshal challenge response: %w", err)
	}

	out := NewAgentMessage()
	out.MessageType = InputStreamData
	out.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)
	out.Flags = Data
	out.PayloadType = EncChallengeResponse
	out.Payload = payload

	if _, err := c.WriteMsg(out); err != nil {
		return fmt.Errorf("send challenge response: %w", err)
	}

	c.encryptionEnabled = true
	zap.S().Info("KMS encryption enabled for session")
	return nil
}

func (c *SsmDataChannel) startSession(cfg aws.Config, in *ssm.StartSessionInput, resolver *SSMMessagesResover) error {
	out, err := ssm.NewFromConfig(cfg).StartSession(context.Background(), in)
	if err != nil {
		return err
	}
	if out.SessionId != nil {
		c.sessionId = *out.SessionId
	}
	StreamEndpointOverride(resolver, out)
	return c.StartSessionFromDataChannelURL(*out.StreamUrl, *out.TokenValue)
}

func (c *SsmDataChannel) StartSessionFromDataChannelURL(url string, token string) error {
	ws, _, err := websocket.DefaultDialer.Dial(url, http.Header{}) //nolint:bodyclose
	if err != nil {
		return err
	}
	c.ws = ws

	// Set up pong handler for health monitoring
	c.ws.SetPongHandler(func(appData string) error {
		return nil
	})

	if err = c.openDataChannel(token); err != nil {
		_ = c.Close()
		return err
	}

	// Start ping loop for connection health monitoring
	go c.pingLoop()

	return nil
}

func (c *SsmDataChannel) openDataChannel(token string) error {
	openDataChanInput := map[string]string{
		"MessageSchemaVersion": "1.0",
		"RequestId":            uuid.New().String(),
		"TokenValue":           token,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(openDataChanInput)
}

// buildHandshakeResponse builds the handshake response for each RequestedClientAction.
// It handles SessionType (always) and KMSEncryption (generates data keys via KMS).
// Any non-success is considered a failure in the receiving agent.
func (c *SsmDataChannel) buildHandshakeResponse(actions []RequestedClientAction) *HandshakeResponsePayload {
	// Advertise client version >= 1.1.70 to enable multiplexed port forwarding
	// in agents that support it (>= 3.0.196.0). Older agents ignore the version.
	clientVersion := "1.1.0"
	if versionGte(c.agentVersion, "3.0.196.0") {
		clientVersion = "1.2.0"
	}

	res := HandshakeResponsePayload{
		ClientVersion:          clientVersion,
		ProcessedClientActions: make([]ProcessedClientAction, len(actions)),
	}

	for i, a := range actions {
		action := new(ProcessedClientAction)

		switch a.ActionType {
		case SessionType:
			action.ActionType = a.ActionType
			action.ActionStatus = Success
		case KMSEncryption:
			action.ActionType = a.ActionType
			c.handleKMSEncryptionAction(a, action)
		default:
			action.ActionType = a.ActionType
			action.ActionStatus = Unsupported
		}

		res.ProcessedClientActions[i] = *action
	}

	return &res
}

// handleKMSEncryptionAction processes the KMSEncryption handshake action by generating
// data keys from KMS and storing them for session encryption.
func (c *SsmDataChannel) handleKMSEncryptionAction(req RequestedClientAction, result *ProcessedClientAction) {
	// Extract KMSKeyId from ActionParameters
	paramBytes, err := json.Marshal(req.ActionParameters)
	if err != nil {
		result.ActionStatus = Failed
		result.Error = fmt.Sprintf("marshal action parameters: %v", err)
		return
	}

	var kmsReq KMSEncryptionRequest
	if err := json.Unmarshal(paramBytes, &kmsReq); err != nil {
		result.ActionStatus = Failed
		result.Error = fmt.Sprintf("unmarshal KMS request: %v", err)
		return
	}

	if kmsReq.KMSKeyId == "" {
		result.ActionStatus = Failed
		result.Error = "empty KMS key ID"
		return
	}

	client := c.newKMSClient()
	encryptKey, decryptKey, ciphertextKey, err := GenerateEncryptionKeys(client, kmsReq.KMSKeyId, c.sessionId, c.targetId)
	if err != nil {
		zap.S().Warnf("KMS GenerateDataKey failed: %v", err)
		result.ActionStatus = Failed
		result.Error = fmt.Sprintf("generate encryption keys: %v", err)
		return
	}

	c.encryptKey = encryptKey
	c.decryptKey = decryptKey

	kmsResp := KMSEncryptionResponse{
		KMSCipherTextKey:  ciphertextKey,
		KMSCipherTextHash: ciphertextKeyHash(ciphertextKey),
	}

	actionResult, err := json.Marshal(kmsResp)
	if err != nil {
		result.ActionStatus = Failed
		result.Error = fmt.Sprintf("marshal KMS response: %v", err)
		return
	}

	result.ActionStatus = Success
	result.ActionResult = actionResult
}

// newKMSClient creates a KMS client from the stored AWS config, or returns the
// test override if set.
func (c *SsmDataChannel) newKMSClient() KMSClient {
	if c.kmsClientOverride != nil {
		return c.kmsClientOverride
	}
	return kms.NewFromConfig(c.cfg)
}

// pingLoop sends periodic WebSocket ping messages to keep the connection alive
// and detect disconnections. Runs until the connection is closed.
func (c *SsmDataChannel) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		ws := c.ws
		c.mu.Unlock()

		if ws == nil {
			return
		}

		if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
			zap.S().Debugf("ping failed, connection may be closed: %v", err)
			return
		}
	}
}

// versionGte returns true if version >= minVersion using dot-separated integer comparison.
func versionGte(version, minVersion string) bool {
	parse := func(v string) []int {
		if v == "" {
			return nil
		}
		parts := strings.Split(v, ".")
		result := make([]int, 0, len(parts))
		for _, p := range parts {
			num, err := strconv.Atoi(p)
			if err != nil || num < 0 {
				return nil
			}
			result = append(result, num)
		}
		return result
	}

	a := parse(version)
	b := parse(minVersion)
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av > bv {
			return true
		}
		if av < bv {
			return false
		}
	}
	return true
}
