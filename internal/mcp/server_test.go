package mcp

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

// An oversized request is dropped and the next one still reads.
func TestReadLineSkipsOversized(t *testing.T) {
	r := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 100)+"\n{\"id\":1}\n"), 16)
	line, tooLong, err := readLine(r, 50)
	if !tooLong || line != nil || err != nil {
		t.Fatalf("oversized: %q %v %v", line, tooLong, err)
	}
	line, tooLong, err = readLine(r, 50)
	if tooLong || string(line) != "{\"id\":1}\n" || err != nil {
		t.Fatalf("next line: %q %v %v", line, tooLong, err)
	}
	if _, _, err = readLine(r, 50); err != io.EOF {
		t.Fatalf("end: %v", err)
	}
}
