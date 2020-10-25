package ssmclient

import (
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
	SendAcknowledgeMessage(*AgentMessage) error
	ReaderChannel() (chan []byte, chan error)
	io.Writer
}

type dataChanel struct {
	seqNum int64
	mu     sync.Mutex
	ws     *websocket.Conn
}

func (c *dataChanel) Open(cfg client.ConfigProvider, in *ssm.StartSessionInput) error {
	return c.startSession(cfg, in)
}

func (c *dataChanel) Close() error {
	var err error
	if c.ws != nil {
		err = c.ws.Close()
	}
	return err
}

func (c *dataChanel) ReaderChannel() (chan []byte, chan error) {
	dataCh := make(chan []byte, 65535)
	errCh := make(chan error, 4)

	go c.startReadLoop(dataCh, errCh)

	return dataCh, errCh
}

func (c *dataChanel) Write(payload []byte) (int, error) {
	seqNum := atomic.AddInt64(&c.seqNum, 1)

	msg := NewAgentMessage()
	msg.MessageType = InputStreamData
	msg.SequenceNumber = seqNum
	msg.Flags = Data
	msg.PayloadType = Output
	msg.Payload = payload

	data, err := msg.MarshalBinary()
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return int(msg.payloadLength), c.ws.WriteMessage(websocket.BinaryMessage, data)
}

func (c *dataChanel) ProcessHandshakeRequest(msg *AgentMessage) error {
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

	var data []byte
	data, err = out.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.BinaryMessage, data)
}

func (c *dataChanel) SendAcknowledgeMessage(msg *AgentMessage) error {
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

	var data []byte
	data, err = agentMsg.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.BinaryMessage, data)
}

func (c *dataChanel) startSession(cfg client.ConfigProvider, in *ssm.StartSessionInput) error {
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

func (c *dataChanel) openDataChannel(token string) error {
	openDataChanInput := map[string]string{
		"MessageSchemaVersion": "1.0",
		"RequestId":            uuid.New().String(),
		"TokenValue":           token,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(openDataChanInput)
}

func (c *dataChanel) startReadLoop(dataCh chan []byte, errCh chan error) {
	//defer close(dataCh)
	//defer close(errCh)

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
				log.Println("Handshake complete")
			default:
				log.Printf("UNKNOWN PAYLOAD MSG IN: %s\n%s", m, m.Payload)
			}
		case ChannelClosed:
			log.Print("closed")
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
