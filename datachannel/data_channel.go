package datachannel

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// DataChannel is the interface definition for handling communication with the AWS SSM messaging service.
type DataChannel interface {
	Open(client.ConfigProvider, *ssm.StartSessionInput) error
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
}

// Open creates the web socket connection with the AWS service and opens the data channel.
func (c *SsmDataChannel) Open(cfg client.ConfigProvider, in *ssm.StartSessionInput) error {
	c.handshakeCh = make(chan bool, 1)
	c.outMsgBuf = NewMessageBuffer(50)
	c.inMsgBuf = NewMessageBuffer(50)

	go c.processOutboundQueue()

	return c.startSession(cfg, in)
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

// WaitForHandshakeComplete blocks further processing until the required SSM handshake sequence used for
// port-based clients (including ssh) completes.
func (c *SsmDataChannel) WaitForHandshakeComplete() error {
	buf := make([]byte, 4096)

	for {
		select {
		case <-c.handshakeCh:
			// make stream unbuffered
			c.inMsgBuf = nil
			c.outMsgBuf = nil
			c.handshakeCh = nil
			return nil
		default:
			n, err := c.Read(buf)
			if err != nil {
				return err
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
	buf := make([]byte, 2048)
	var nr, nw int
	var payload []byte

	for {
		nr, err = c.Read(buf)
		if err != nil {
			// log.Printf("WriteTo read error: %v", err)
			return n, err
		}

		if nr > 0 {
			payload, err = c.HandleMsg(buf[:nr])
			if err != nil {
				return int64(nw), err
			}

			if len(payload) > 0 {
				nw, err = w.Write(payload)
				n += int64(nw)
				if err != nil {
					// log.Printf("WriteTo write error: %v", err)
					return n, err
				}
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
			}
			break
		}

		if _, err = c.Write(buf[:nr]); err != nil {
			// log.Printf("ReadFrom write error: %v", err)
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
	msg.Payload = payload
	msg.SequenceNumber = atomic.AddInt64(&c.seqNum, 1)

	return c.WriteMsg(msg)
}

// WriteMsg is the underlying method which marshals AgentMessage types and sends them to the AWS service.
// This is provided as a convenience so that messages types not already handled can be sent. If the message
// SequenceNumber field is less than 0, it will be automatically incremented using the internal counter.
func (c *SsmDataChannel) WriteMsg(msg *AgentMessage) (int, error) {
	if !c.synSent {
		atomic.StoreInt64(&c.seqNum, 0)
		msg.Flags = Syn
		msg.SequenceNumber = c.seqNum
	}

	if msg.SequenceNumber < 0 {
		atomic.StoreInt64(&c.seqNum, 1)
	}

	data, err := msg.MarshalBinary()
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.synSent = true

	if c.outMsgBuf != nil && msg.MessageType != Acknowledge && msg.PayloadType != HandshakeResponse {
		err = c.outMsgBuf.Add(msg)
	}

	if !c.pausePub {
		return int(msg.payloadLength), c.ws.WriteMessage(websocket.BinaryMessage, data)
	}
	return int(msg.payloadLength), err
}

//nolint:gocognit,gocyclo
// HandleMsg takes the unprocessed message bytes from the websocket connection (a la Read()), unmarshals the data
// and takes the appropriate action based on the message type.  Messages which have an actionable payload (output
// payload types, and channel closed payloads) will have that data returned.  Errors will be returned for unknown/
// unhandled message or payload types.  A ChannelClosed message type will return an io.EOF error to indicate that
// this SSM data channel is shutting down and should no longer be used.
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
			// unbuffered - return payload directly
			if c.inMsgBuf == nil {
				_ = c.sendAcknowledgeMessage(m) // todo - handle error?
				return m.Payload, nil
			}

			// duplicate message - discard
			if m.SequenceNumber < c.inSeqNum {
				return nil, nil
			}

			// queue everything else
			if err := c.inMsgBuf.Add(m); err != nil {
				return nil, err
			}
		case HandshakeRequest:
			// port forwarding session setup, we'll consider a handshake failure fatal
			if err := c.processHandshakeRequest(m); err != nil {
				return nil, err
			}
		case HandshakeComplete:
			if c.handshakeCh != nil {
				close(c.handshakeCh)
			}
		default:
			return nil, fmt.Errorf("UNKNOWN INCOMING MSG PAYLOAD: %s\n%s", m, m.Payload)
		}
	case ChannelClosed:
		payload := new(ChannelClosedPayload)
		if err := json.Unmarshal(m.Payload, payload); err != nil {
			return nil, err
		}

		var output []byte
		if len(payload.Output) > 0 {
			output = []byte(payload.Output)
		}
		return output, io.EOF
	default:
		return nil, fmt.Errorf("UNKNOWN MESSAGE TYPE: %+v", m)
	}

	if err := c.sendAcknowledgeMessage(m); err != nil {
		// todo - handle this better (retry?)
		return nil, err
	}

	return c.processInboundQueue()
}

// SetTerminalSize sends a message to the SSM service which indicates the size to use for the remote terminal
// when using a shell session client.
func (c *SsmDataChannel) SetTerminalSize(rows, cols uint32) error {
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
	for {
		time.Sleep(500 * time.Millisecond)
		if c.pausePub {
			continue
		}

		if c.outMsgBuf == nil {
			return
		}

		for m := c.outMsgBuf.Next(); m != nil; m = c.outMsgBuf.Next() {
			if _, err := c.WriteMsg(m); err != nil {
				// todo - handle error?
			}
		}
	}
}

// sendAcknowledgeMessage sends the Acknowledge message type for each incoming message read from
// the web socket connection, which is required as part of the SSM session protocol.
func (c *SsmDataChannel) sendAcknowledgeMessage(msg *AgentMessage) error {
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

	payload, err := json.Marshal(buildHandshakeResponse(req.RequestedClientActions))
	if err != nil {
		return err
	}

	out := NewAgentMessage()
	out.MessageType = InputStreamData
	out.SequenceNumber = msg.SequenceNumber
	out.Flags = Data
	out.PayloadType = HandshakeResponse
	out.Payload = payload

	_, err = c.WriteMsg(out)
	return err
}

func (c *SsmDataChannel) startSession(cfg client.ConfigProvider, in *ssm.StartSessionInput) error {
	out, err := ssm.New(cfg).StartSession(in)
	if err != nil {
		return err
	}

	c.ws, _, err = websocket.DefaultDialer.Dial(*out.StreamUrl, http.Header{}) //nolint:bodyclose
	if err != nil {
		return err
	}

	if err = c.openDataChannel(*out.TokenValue); err != nil {
		_ = c.Close()
		return err
	}

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

// the only requirement of the handshake response is that we include an element in ProcessedClientActions
// for each element of RequestedClientActions (there's only 2 types, and port forwarding only uses the
// SessionType action type, so there should only be 1 element), and the ActionStatus is Success.  Any
// non-success is considered a failure in the receiving agent.
func buildHandshakeResponse(actions []RequestedClientAction) *HandshakeResponsePayload {
	res := HandshakeResponsePayload{
		// seems this can be whatever we need it to be, however certain features may only be available at
		// certain client versions (must report at least version 1.1.70 to do stream muxing)
		ClientVersion:          "0.0.1",
		ProcessedClientActions: make([]ProcessedClientAction, len(actions)),
	}

	for i, a := range actions {
		action := new(ProcessedClientAction)

		if a.ActionType == SessionType {
			action.ActionType = a.ActionType
			action.ActionStatus = Success
		}

		res.ProcessedClientActions[i] = *action
	}

	return &res
}
