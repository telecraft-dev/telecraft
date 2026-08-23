package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/auth"
)

// runPasswd hashes one basic-auth secret for users.yaml (REQ-017,
// ADR-0019 §1: bootstrap and break-glass). The secret arrives on stdin,
// never an argument, which would land it in shell history, and the
// printed hash is what a user's `password` field carries.
func runPasswd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("passwd", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "passwd: the secret is read from stdin, never an argument")
		return 2
	}

	secret, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(stderr, "passwd: %v\n", err)
		return 1
	}
	secret = strings.TrimRight(secret, "\r\n")
	hash, err := auth.HashSecret(secret)
	if err != nil {
		fmt.Fprintf(stderr, "passwd: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, hash)
	return 0
}
