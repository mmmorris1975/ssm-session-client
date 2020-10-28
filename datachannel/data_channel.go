package datachannel

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
)

type DataChannel interface {
	Open(client.ConfigProvider, *ssm.StartSessionInput) error
	ProcessHandshakeRequest(*AgentMessage) error
	SetTerminalSize(rows, cols uint32) error
	SendAcknowledgeMessage(*AgentMessage) error
	TerminateSession() error
	DisconnectPort() error
	ReaderChannel() (chan []byte, chan error)
	WriteMsg(*AgentMessage) (int, error)
	io.WriteCloser
}

type SsmDataChannel struct {
	seqNum  int64
	mu      sync.Mutex
	ws      *websocket.Conn
	synSent bool
}

// Open creates the web socket connection with the AWS service and sends the request to open the data channel
func (c *SsmDataChannel) Open(cfg client.ConfigProvider, in *ssm.StartSessionInput) error {
	return c.startSession(cfg, in)
}

// Close shuts down the web socket connection with the AWS service. Type-specific actions (like sending
// TerminateSession for port forwarding should be handled before calling Close()
func (c *SsmDataChannel) Close() error {
	var err error
	if c.ws != nil {
		err = c.ws.Close()
	}
	return err
}

// ReaderChannel opens up a channel which will receive data from the web socket, and send message acknowledgements
// as needed.  If it is an output message type, the payload bytes will be written to the []byte channel, Other
// message types (handshakes, etc) will be handled internally.  Processing errors for any message type will be
// written to the error channel.
func (c *SsmDataChannel) ReaderChannel() (chan []byte, chan error) {
	dataCh := make(chan []byte, 65535)
	errCh := make(chan error, 4)

	go c.startReadLoop(dataCh, errCh)

	return dataCh, errCh
}

// Write sends an input stream data message with the provided payload set as the message payload
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
// This is provided as a convenience so that messages types not already handled can be sent.
func (c *SsmDataChannel) WriteMsg(msg *AgentMessage) (int, error) {
	if !c.synSent {
		atomic.StoreInt64(&c.seqNum, 0)
		msg.Flags = Syn
		msg.SequenceNumber = c.seqNum
	}

	data, err := msg.MarshalBinary()
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.synSent = true
	return int(msg.payloadLength), c.ws.WriteMessage(websocket.BinaryMessage, data)
}

// ProcessHandshakeRequest handles the incoming handshake request message for a port forwarding session
// and sends the required HandshakeResponse message.  This must complete before sending data over the
// forwarded connection.
func (c *SsmDataChannel) ProcessHandshakeRequest(msg *AgentMessage) error {
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

// SendAcknowledgeMessage sends the Acknowledge message type for each incoming message read from
// the web socket connection, which is required as part of the SSM session protocol
func (c *SsmDataChannel) SendAcknowledgeMessage(msg *AgentMessage) error {
	ack := map[string]interface{}{
		"AcknowledgedMessageType":           msg.MessageType,
		"AcknowledgedMessageId":             msg.messageId.String(),
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

// TerminateSession sends the TerminateSession flag to the AWS service to indicate that the port forwarding
// session is ending, and clean up any connections used to communicate with the EC2 instance agent.
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

// DisconnectPort sends the DisconnectToPort flag to the AWS service to indicate that a non-muxing stream is
// shutting down and any connection used to communicate with the EC2 instance agent can be cleaned up.  Unlike
// the TerminateSession action, the connection is still capable of initiating a new port forwarding stream to
// the agent without needing to restart the program.
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

func (c *SsmDataChannel) startSession(cfg client.ConfigProvider, in *ssm.StartSessionInput) error {
	out, err := ssm.New(cfg).StartSession(in)
	if err != nil {
		return err
	}

	c.ws, _, err = websocket.DefaultDialer.Dial(*out.StreamUrl, http.Header{})
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

func (c *SsmDataChannel) startReadLoop(dataCh chan []byte, errCh chan error) {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			// gorilla code states this is uber-fatal, and we just need to bail out
			log.Printf("ReadMessage freakout: %v", err)
			errCh <- err
			break
		}

		m := new(AgentMessage)
		if err = m.UnmarshalBinary(data); err != nil {
			// validation error
			errCh <- err
			continue
		}

		if err = c.SendAcknowledgeMessage(m); err != nil {
			// todo - handle this better (retry?)
			errCh <- err
			break
		}

		switch m.MessageType {
		case Acknowledge:
			// anything?
		case OutputStreamData:
			switch m.PayloadType {
			case Output:
				dataCh <- m.Payload
			case HandshakeRequest:
				// port forwarding session setup, we'll consider a handshake failure fatal
				if err = c.ProcessHandshakeRequest(m); err != nil {
					errCh <- err
					break
				}
			case HandshakeComplete:
				// anything?
				log.Println("Ready!")
			default:
				log.Printf("UNKNOWN PAYLOAD MSG IN: %s\n%s", m, m.Payload)
			}
		case ChannelClosed:
			payload := new(ChannelClosedPayload)
			_ = json.Unmarshal(m.Payload, payload)

			if len(payload.Output) > 0 {
				dataCh <- []byte(payload.Output)
			}
			close(dataCh) // fixme - there's an occasional double close here on shutdown when using shell & ssh
			break
		default:
			// todo handle unknown message type
			panic(fmt.Sprintf("Unknown message type: %+v", m))
		}
	}
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
