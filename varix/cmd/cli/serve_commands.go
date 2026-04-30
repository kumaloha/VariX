package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kumaloha/VariX/varix/api"
)

func runServeCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", ":8000", "listen address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*addr) == "" {
		fmt.Fprintln(stderr, "usage: varix serve --addr <host:port>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	fmt.Fprintf(stdout, "varix api listening on %s\n", strings.TrimSpace(*addr))
	if err := http.ListenAndServe(strings.TrimSpace(*addr), api.NewServer(store).Handler()); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return 0
}
