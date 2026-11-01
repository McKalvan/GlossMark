// @gloss implements gmi:LanguageServer scope=file

package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// The JSON-RPC 2.0 framing the Language Server Protocol uses: each message
// is preceded by a Content-Length header and a blank line. Hand-rolled on
// the standard library, like the MCP server, so gloss stays one static
// binary (ADR 0051).

// message is a request, a notification (no id) or a response (no method).
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// JSON-RPC and LSP error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
	codeRequestFailed  = -32803
)

// maxMessage caps one message.
const maxMessage = 64 << 20

// conn reads and writes framed messages.
type conn struct {
	r    *bufio.Reader
	w    io.Writer
	mu   sync.Mutex
	next int64
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReaderSize(r, 1<<20), w: w}
}

// read returns the next message, or io.EOF when the client has gone.
func (c *conn) read() (*message, error) {
	length := -1
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(line) == "" {
				return nil, io.EOF
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("malformed header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("bad Content-Length %q", value)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("message without Content-Length")
	}
	if length > maxMessage {
		return nil, fmt.Errorf("message of %d bytes exceeds %d MiB", length, maxMessage>>20)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, &rpcError{codeParse, "parse error: " + err.Error()}
	}
	return &m, nil
}

func (c *conn) write(m message) error {
	m.JSONRPC = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = c.w.Write(b)
	return err
}

// reply answers a request; a nil result is sent as JSON null, as the
// protocol requires for "nothing" (no hover, no definition).
func (c *conn) reply(id json.RawMessage, result any, rerr *rpcError) {
	m := message{ID: id, Error: rerr}
	if rerr == nil {
		if result == nil {
			m.Result = json.RawMessage("null")
		} else {
			m.Result = result
		}
	}
	c.write(m) //nolint:errcheck // a closed client ends the session at the next read
}

// notify sends a notification to the client.
func (c *conn) notify(method string, params any) {
	c.write(message{Method: method, Params: raw(params)}) //nolint:errcheck
}

// request sends a server-to-client request. Answers are not awaited: the
// read loop drops responses, since nothing the server asks (registering a
// watcher) changes what it does next.
func (c *conn) request(method string, params any) {
	c.mu.Lock()
	c.next++
	id := json.RawMessage(strconv.FormatInt(c.next, 10))
	c.mu.Unlock()
	c.write(message{ID: id, Method: method, Params: raw(params)}) //nolint:errcheck
}

func raw(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}
