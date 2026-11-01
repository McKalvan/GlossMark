// @gloss implements gmi:SourcePlugin scope=file

package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/infer"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// Client runs one external source as an infer.Source: the process starts
// at Index and ends at Close. After the first failure (a crash, output
// that is not the protocol, a wrong name) the client answers nothing
// more and keeps the error for Err, so the other sources run on; a
// contract mismatch is a ContractError, which fails the command.
type Client struct {
	name    string
	command []string
	origin  string // where the source was discovered: PATH or glossmark.yaml
	dir     string // working directory: the workspace root

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	enc    *json.Encoder
	dec    *json.Decoder
	stderr tail

	files  infer.Files
	v      infer.Vocabulary
	items  map[string][]infer.Item
	priors map[string]float64
	err    error
}

// New prepares a client for a source discovered under name, run as
// command from dir. Nothing runs until Index.
func New(name string, command []string, origin, dir string) *Client {
	return &Client{name: name, command: command, origin: origin, dir: dir}
}

// Name is the source id the plugin was discovered under.
func (c *Client) Name() string { return c.name }

// Command is the argument vector the plugin runs as.
func (c *Client) Command() []string { return c.command }

// Origin says where the plugin was found: "PATH" or "glossmark.yaml".
func (c *Client) Origin() string { return c.origin }

// Err is the first failure, or nil. A ContractError is returned by Index
// as well; every other failure is reported here only, so the runner can
// say which plugin broke and keep the others.
func (c *Client) Err() error { return c.err }

// Priors are the precisions the plugin declared for its bases ("basis" or
// "basis/role", without the source's name).
func (c *Client) Priors() map[string]float64 { return c.priors }

// ContractError: the plugin speaks another protocol version.
type ContractError struct {
	Plugin string
	Got    int
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("source plugin %s speaks contract v%d; this gloss speaks v%d: upgrade gloss or the plugin so they agree, or remove the plugin from PATH or sources: in %s", e.Plugin, e.Got, Contract, load.ConfigName)
}

// Index starts the plugin and runs the index exchange.
func (c *Client) Index(files infer.Files, v infer.Vocabulary) error {
	c.files, c.v = files, v
	if err := c.start(); err != nil {
		return c.fail(err)
	}
	if err := c.send(Message{Type: TypeIndex, Contract: Contract, Root: files.Root, Files: files.List, Vocabulary: v.Concepts()}); err != nil {
		return c.fail(err)
	}
	m, err := c.await(TypeIndexed)
	if err != nil {
		return c.fail(err)
	}
	if m.Contract != Contract {
		c.err = &ContractError{Plugin: c.name, Got: m.Contract}
		c.Close()
		return c.err
	}
	if m.Name != c.name {
		return c.fail(fmt.Errorf("answered as %q, but was discovered as %q (rename the executable, or the name: in sources:)", m.Name, c.name))
	}
	for p, items := range m.Items {
		for i := range items {
			if items[i].Line < 1 || items[i].End < items[i].Line {
				return c.fail(fmt.Errorf("item %s#%s: line %d, end %d are not a 1-based line range", p, items[i].Symbol, items[i].Line, items[i].End))
			}
		}
	}
	for b, p := range m.Priors {
		if p <= 0 || p > 1 {
			return c.fail(fmt.Errorf("prior %s: %v is not in (0, 1]", b, p))
		}
	}
	c.items, c.priors = m.Items, m.Priors
	return nil
}

// Items are what the plugin declared for a file at index time.
func (c *Client) Items(p string) []infer.Item { return c.items[p] }

// Hints asks the plugin about one declaration.
func (c *Client) Hints(d infer.Decl) []infer.Hint {
	if c.err != nil || c.cmd == nil {
		return nil
	}
	if err := c.send(Message{Type: TypeHints, Decl: &d}); err != nil {
		c.fail(err)
		return nil
	}
	m, err := c.await(TypeHints)
	if err != nil {
		c.fail(err)
		return nil
	}
	return m.Hints
}

// Close ends the run: stdin closes, the plugin exits. A plugin that
// ignores the end of its input is killed after a grace period.
func (c *Client) Close() {
	if c.cmd == nil {
		return
	}
	if c.stdin != nil {
		c.stdin.Close()
	}
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.cmd.Process.Kill()
		<-done
	}
	c.cmd = nil
}

func (c *Client) start() error {
	name := c.command[0]
	if strings.ContainsAny(name, `/\`) && !filepath.IsAbs(name) {
		name = filepath.Join(c.dir, name) // a relative command is relative to the root
	}
	exe, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s: %w", c.command[0], err)
	}
	cmd := exec.Command(exe, c.command[1:]...)
	cmd.Dir = c.dir
	cmd.Env = append(os.Environ(), "GLOSS_SOURCE_CONTRACT="+fmt.Sprint(Contract))
	cmd.Stderr = &c.stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd, c.stdin, c.enc, c.dec = cmd, in, json.NewEncoder(in), json.NewDecoder(out)
	return nil
}

func (c *Client) send(m Message) error {
	if err := c.enc.Encode(m); err != nil {
		return fmt.Errorf("writing %s: %w", m.Type, err)
	}
	return nil
}

// await reads messages until the one of the wanted type, answering the
// plugin's questions (read, named, ancestor) on the way.
func (c *Client) await(want string) (Message, error) {
	for {
		var m Message
		if err := c.dec.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				return m, fmt.Errorf("exited before answering %s", want)
			}
			return m, fmt.Errorf("output is not the wire protocol (one JSON object per line, docs/PLUGINS.md): %v", err)
		}
		switch m.Type {
		case want:
			return m, nil
		case TypeError:
			return m, fmt.Errorf("reported: %s", m.Message)
		case TypeRead:
			if err := c.send(Message{Type: TypeFiles, Contents: c.read(m.Paths, m.Globs)}); err != nil {
				return m, err
			}
		case TypeNamed:
			if err := c.send(Message{Type: TypeConcepts, Concepts: c.v.Named(m.Name, m.Kinds...)}); err != nil {
				return m, err
			}
		case TypeAncestor:
			if err := c.send(Message{Type: TypeAncestor, Curie: c.v.CommonAncestor(m.Curies)}); err != nil {
				return m, err
			}
		default:
			return m, fmt.Errorf("sent %q while %s was expected", m.Type, want)
		}
	}
}

// read answers a read: the content of every listed workspace file the
// paths name or the globs match, in workspace order. A path outside the
// list is not read.
func (c *Client) read(paths, globs []string) []File {
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	var out []File
	for _, f := range c.files.List {
		ok := want[f.Path]
		for _, g := range globs {
			ok = ok || model.MatchGlob(g, f.Path) || matchBase(g, f.Path)
		}
		if !ok {
			continue
		}
		b, err := os.ReadFile(c.files.Abs(f.Path))
		if err != nil {
			continue
		}
		out = append(out, File{Path: f.Path, Content: string(b)})
	}
	return out
}

// matchBase: a glob without a slash matches the file's base name.
func matchBase(g, rel string) bool {
	if strings.Contains(g, "/") {
		return false
	}
	ok, _ := path.Match(g, path.Base(rel))
	return ok
}

// fail records the first failure, with the plugin's last words on
// stderr, and ends the process.
func (c *Client) fail(err error) error {
	c.Close() // waits for the process, so its stderr is complete
	if c.err == nil {
		msg := fmt.Sprintf("source plugin %s (%s, from %s) %v", c.name, strings.Join(c.command, " "), c.origin, err)
		if s := c.stderr.String(); s != "" {
			msg += "; its stderr: " + s
		}
		c.err = fmt.Errorf("%s; its proposals are left out [GM700]", msg)
	}
	return c.err
}

// tail keeps the last part of what the plugin wrote on stderr.
type tail struct{ buf bytes.Buffer }

const tailMax = 2048

func (t *tail) Write(p []byte) (int, error) {
	t.buf.Write(p)
	if t.buf.Len() > tailMax {
		b := t.buf.Bytes()
		b = append([]byte(nil), b[len(b)-tailMax:]...)
		t.buf.Reset()
		t.buf.Write(b)
	}
	return len(p), nil
}

func (t *tail) String() string {
	s := strings.TrimSpace(t.buf.String())
	if i := strings.LastIndex(s, "\n"); i >= 0 && len(s)-i < 200 {
		// The last line says the most; keep up to a few of them.
		lines := strings.Split(s, "\n")
		if len(lines) > 5 {
			lines = lines[len(lines)-5:]
		}
		s = strings.Join(lines, " | ")
	}
	return s
}

// Discover lists the external sources a suggest run uses, in order:
// the sources: entries of glossmark.yaml, then every gloss-source-<name>
// executable on PATH that no entry already names. A name a plugin shares
// with a built-in source replaces the built-in, which is how the
// built-in can be run through the protocol (gloss source serve); a
// plugin may not take a core source's name (code, route, topic,
// schema). dir is the workspace root the plugins run from.
func Discover(cfg *model.Config, dir string) ([]*Client, error) {
	var out []*Client
	seen := map[string]bool{}
	if cfg != nil {
		for _, s := range cfg.Sources {
			if coreSources[s.Name] {
				return nil, fmt.Errorf("%s: sources: %s is the name of a core source (code, route, topic, schema); pick another", load.ConfigName, s.Name)
			}
			out = append(out, New(s.Name, s.Command, load.ConfigName, dir))
			seen[s.Name] = true
		}
	}
	var found []*Client
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d == "" {
			continue
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name, ok := pluginName(e.Name())
			if !ok || seen[name] || e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil || !executable(info.Mode()) {
				continue
			}
			seen[name] = true
			found = append(found, New(name, []string{e.Name()}, "PATH", dir))
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].name < found[j].name })
	return append(out, found...), nil
}

// executable: a file the shell would run.
func executable(mode os.FileMode) bool {
	return mode.IsRegular() && (runtime.GOOS == "windows" || mode&0o111 != 0)
}

// Prefix is what an executable on PATH is called to be found: gloss-source-<name>.
const Prefix = "gloss-source-"

// pluginName is the source id of an executable's file name, if it is one.
func pluginName(file string) (string, bool) {
	name := strings.TrimPrefix(file, Prefix)
	if name == file {
		return "", false
	}
	for _, ext := range []string{".exe", ".bat", ".cmd"} {
		name = strings.TrimSuffix(name, ext)
	}
	if !model.ValidSourceName(name) || coreSources[name] {
		return "", false
	}
	return name, true
}
