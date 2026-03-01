package session

import (
	"fmt"
	"net"
	"strings"
)

// ParseHostPort parses a target string in the format "[user@]host[:port]" and returns the user,
// host, and port components. If no user is specified, defaultUser is used. If no port is
// specified, defaultPort is used.
func ParseHostPort(target string, defaultUser string, defaultPort int) (user, host string, port int, err error) {
	user = defaultUser

	if strings.Contains(target, "@") {
		parts := strings.SplitN(target, "@", 2)
		user = parts[0]
		target = parts[1]
	}

	if !strings.Contains(target, ":") {
		target = fmt.Sprintf("%s:%d", target, defaultPort)
	}

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return user, target, defaultPort, fmt.Errorf("invalid target format %q: %w", target, err)
	}

	port, err = net.LookupPort("tcp", portStr)
	if err != nil {
		return user, host, defaultPort, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	return user, host, port, nil
}
