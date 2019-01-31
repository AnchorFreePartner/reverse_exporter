package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

func uniListen(addr string) (net.Listener, error) {
	parts := strings.SplitN(addr, "://", 2)
	if len(parts) < 2 {
		return net.Listen("tcp", addr)
	}

	// "tcp", "tcp4", "tcp6", "unix" or "unixpacket"
	switch parts[0] {
	case "unix", "unixpacket":
		os.MkdirAll(filepath.Dir(parts[1]), 0700) // nolint: errcheck,gas
		os.Remove(parts[1])                       // nolint: errcheck,gas
		return net.Listen(parts[0], parts[1])
	case "tcp", "tcp4", "tcp6":
		return net.Listen(parts[0], parts[1])
	default:
		return net.Listen("tcp", addr)
	}
}
