package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var getwd = os.Getwd

func main() {
	projectRoot, err := resolveProjectRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], projectRoot, os.Stdout, os.Stderr))
}

func run(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usageText())
		return 2
	}
	if args[0] == "ingest" {
		return runIngestCommand(args[1:], projectRoot, stdout, stderr)
	}
	if args[0] == "compile" {
		return runCompileCommand(args[1:], projectRoot, stdout, stderr)
	}
	if args[0] == "verify" {
		return runVerifyCommand(args[1:], projectRoot, stdout, stderr)
	}
	if args[0] == "memory" {
		return runMemoryCommand(args[1:], projectRoot, stdout, stderr)
	}
	if args[0] == "serve" {
		return runServeCommand(args[1:], projectRoot, stdout, stderr)
	}
	if isIngestCommand(args[0]) {
		return runIngestCommand(args, projectRoot, stdout, stderr)
	}
	fmt.Fprintln(stderr, "unknown command")
	return 2
}

func resolveProjectRoot() (string, error) {
	cwd, err := getwd()
	if err != nil {
		return "", err
	}
	return resolveProjectRootFrom(cwd, os.Getenv("VARIX_ROOT"))
}

func resolveProjectRootFrom(cwd string, envRoot string) (string, error) {
	if strings.TrimSpace(envRoot) != "" {
		return filepath.Clean(envRoot), nil
	}

	dir := filepath.Clean(cwd)
	for {
		if fileExists(filepath.Join(dir, "varix", "go.mod")) {
			return dir, nil
		}
		if filepath.Base(dir) == "varix" && fileExists(filepath.Join(dir, "go.mod")) {
			return filepath.Dir(dir), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not resolve project root from %s; set VARIX_ROOT", cwd)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
