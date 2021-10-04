package main

import (
	"bufio"
	"io"
)

// newScanner wraps a reader in a line scanner with a generous buffer so
// long script lines are not truncated.
func newScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return s
}
