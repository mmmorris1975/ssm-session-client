package ssmclient

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"golang.org/x/net/netutil"
	"io"
	"log"
	"net"
	"strconv"
)

type PortForwardingInput struct {
	Target     string
	RemotePort int
	LocalPort  int
}

// Both the basic and muxing plugins on the agent side support the Flag payload type with the
// PayloadTypeFlag of TerminateSession.  The basic plugin also supports the DisconnectToPort PayloadTypeFlag
func PortForwardingSession(cfg client.ConfigProvider, opts *PortForwardingInput) error {
	in := &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartPortForwardingSession"),
		Target:       aws.String(opts.Target),
		Parameters: map[string][]*string{
			"localPortNumber": {aws.String(strconv.Itoa(opts.LocalPort))},
			"portNumber":      {aws.String(strconv.Itoa(opts.RemotePort))},
		},
	}

	c := new(dataChanel)
	if err := c.Open(cfg, in); err != nil {
		return err
	}
	defer c.Close()

	inCh, errCh := c.ReaderChannel() // reads message from websocket
	//defer close(inCh)
	//defer close(errCh)

	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(opts.LocalPort)))
	if err != nil {
		return err
	}

	// use limit listener for now, eventually maybe we'll add muxing
	lsnr := netutil.LimitListener(l, 1)
	defer lsnr.Close()
	log.Printf("listening on %s", lsnr.Addr())

	var conn net.Conn
	conn, err = lsnr.Accept()
	if err != nil {
		log.Print(err)
	}

	outCh := writePump(conn, errCh)
	//defer close(outCh)

	for {
		select {
		case dataIn, ok := <-inCh:
			log.Print("inCh")
			if ok {
				if _, err = conn.Write(dataIn); err != nil {
					log.Printf("error reading from data channel: %v", err)
				}
			} else {
				return nil
			}
		case dataOut, ok := <-outCh:
			log.Print("outCh")
			if ok {
				if _, err = c.Write(dataOut); err != nil {
					log.Printf("error writing to data channel: %v", err)
				}
			} else {
				return nil
			}
		case err, ok := <-errCh:
			log.Print("errCh")
			if ok {
				log.Printf("data channel error: %v", err)
			} else {
				return nil
			}
		}
	}

	return nil
}

func writePump(conn net.Conn, errCh chan error) chan []byte {
	dataCh := make(chan []byte, 65535)
	buf := make([]byte, 1024)

	go func() {
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					close(dataCh)
					close(errCh)
					break
				} else {
					errCh <- err
					break
				}
			}

			dataCh <- buf[:n]
		}
	}()

	return dataCh
}
