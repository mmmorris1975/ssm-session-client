package ssmclient

import (
	"io"
	"net"

	"github.com/alexbacchin/ssm-session-client/datachannel"
	"go.uber.org/zap"
)

// NewSSMConn creates a net.Conn suitable for use as an SSH transport by bridging
// an SsmDataChannel via net.Pipe(). Two goroutines copy data bidirectionally
// between the pipe and the data channel, using the channel's WriteTo/ReadFrom
// methods to handle SSM message framing and encryption.
func NewSSMConn(c *datachannel.SsmDataChannel) net.Conn {
	localConn, pipeConn := net.Pipe()

	// Bridge: data channel -> pipe (uses SsmDataChannel.WriteTo for message decoding)
	go func() {
		_, err := io.Copy(pipeConn, c)
		if err != nil {
			zap.S().Debugf("datachannel->pipe copy ended: %v", err)
		}
		pipeConn.Close()
	}()

	// Bridge: pipe -> data channel (uses SsmDataChannel.ReadFrom for message encoding)
	go func() {
		_, err := io.Copy(c, pipeConn)
		if err != nil {
			zap.S().Debugf("pipe->datachannel copy ended: %v", err)
		}
		pipeConn.Close()
	}()

	return localConn
}
