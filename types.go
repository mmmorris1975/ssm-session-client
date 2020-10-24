package main

// REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/contracts/model.go
type MessageType string

const (
	InteractiveShell MessageType = "interactive_shell"
	TaskReply        MessageType = "agent_task_reply"
	TaskComplete     MessageType = "agent_task_complete"
	Acknowledge      MessageType = "acknowledge"
	AgentSession     MessageType = "agent_session_state"
	ChannelClosed    MessageType = "channel_closed"
	OutputStreamData MessageType = "output_stream_data"
	InputStreamData  MessageType = "input_stream_data"
	PausePublication MessageType = "pause_publication"
	StartPublication MessageType = "start_publication"
)

type AgentMessageFlag uint64

const (
	Data AgentMessageFlag = iota
	Syn  AgentMessageFlag = iota
	Fin  AgentMessageFlag = iota
	Ack  AgentMessageFlag = iota
)

type PayloadType uint32

const (
	Undefined            PayloadType = iota
	Output               PayloadType = iota
	Error                PayloadType = iota
	Size                 PayloadType = iota
	Parameter            PayloadType = iota
	HandshakeRequest     PayloadType = iota
	HandshakeResponse    PayloadType = iota
	HandshakeComplete    PayloadType = iota
	EncChallengeRequest  PayloadType = iota
	EncChallengeResponse PayloadType = iota
	Flag                 PayloadType = iota
)

type PayloadTypeFlag uint32

const (
	DisconnectToPort   PayloadTypeFlag = 1
	TerminateSession   PayloadTypeFlag = 2
	ConnectToPortError PayloadTypeFlag = 3
)
