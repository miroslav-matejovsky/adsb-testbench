package cli

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ErrInvalidAddress identifies a listen address outside the accepted form.
var ErrInvalidAddress = errors.New("invalid listen address")

// ValidateListenAddress checks a TCP listen address of the form host:port.
//
// The host is required: an IPv4 literal, a bracketed IPv6 literal (with an
// optional zone), or a DNS host name. Wildcard hosts such as 0.0.0.0 and [::]
// are accepted because they are explicitly configured. The port is a decimal
// number from 0 to 65535; port 0 asks the operating system for an ephemeral
// port. URL syntax is rejected. Validation performs no DNS lookup and opens no
// socket.
func ValidateListenAddress(address string) error {
	if strings.Contains(address, "/") {
		return fmt.Errorf("%w: %q must be host:port, not a URL", ErrInvalidAddress, address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %q: %w", ErrInvalidAddress, address, err)
	}
	if err := validPort(port); err != nil {
		return fmt.Errorf("%w: %q: %w", ErrInvalidAddress, address, err)
	}
	if err := validHost(address, host); err != nil {
		return fmt.Errorf("%w: %q: %w", ErrInvalidAddress, address, err)
	}
	return nil
}

func validPort(port string) error {
	if port == "" {
		return errors.New("port is missing")
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return fmt.Errorf("port %q is not a decimal number", port)
		}
	}
	number, err := strconv.Atoi(port)
	if err != nil || number > 65535 {
		return fmt.Errorf("port %q is outside 0..65535", port)
	}
	return nil
}

func validHost(address, host string) error {
	if host == "" {
		return errors.New("host is missing")
	}
	bracketed := strings.HasPrefix(address, "[")
	if strings.Contains(host, ":") || bracketed {
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || !bracketed {
			return fmt.Errorf("host %q is not a bracketed IPv6 literal", host)
		}
		return nil
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}
	return validHostName(host)
}

// validHostName checks RFC 1123 host name syntax without resolving it.
func validHostName(host string) error {
	if len(host) > 253 {
		return fmt.Errorf("host name %q is longer than 253 bytes", host)
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("host name %q has an empty or oversized label", host)
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("host name %q has a label starting or ending with '-'", host)
		}
		for _, r := range label {
			if !hostNameRune(r) {
				return fmt.Errorf("host name %q contains %q", host, r)
			}
		}
	}
	return nil
}

func hostNameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-'
}
