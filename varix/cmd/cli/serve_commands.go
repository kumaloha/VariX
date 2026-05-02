package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/server"
)

const defaultServeAddr = "127.0.0.1:8000"

func runServeCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", defaultServeAddr, "listen address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*addr) == "" {
		fmt.Fprintln(stderr, "usage: varix serve --addr <host:port>")
		return 2
	}
	token := config.FirstConfiguredValue(projectRoot, "VARIX_API_TOKEN", "INVARIX_API_TOKEN")
	userID := config.FirstConfiguredValue(projectRoot, "VARIX_API_USER", "INVARIX_API_USER")
	if err := validateServeAuth(*addr, token, userID); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	fmt.Fprintf(stdout, "varix api listening on %s\n", strings.TrimSpace(*addr))
	handler := server.NewServer(store).Handler()
	if strings.TrimSpace(token) != "" {
		handler = server.BearerTokenAuth(handler, token, userID)
	}
	if err := http.ListenAndServe(strings.TrimSpace(*addr), handler); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return 0
}

func validateServeAuth(addr string, token string, userID string) error {
	if isLocalListenAddr(addr) {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("varix serve requires VARIX_API_TOKEN when listening on a non-local address")
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("varix serve requires VARIX_API_USER when listening on a non-local address")
	}
	return nil
}

func isLocalListenAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.Trim(addr, "[]")
		if idx := strings.LastIndex(host, ":"); idx >= 0 {
			host = host[:idx]
		}
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}
