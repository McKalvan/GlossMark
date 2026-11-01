// @gloss implements gmi:SourcePlugin scope=file

package wire

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mckalvan/glossmark/internal/infer"
)

// Serve runs src as a plugin over in and out: the other end of Client.
// gloss source serve <name> runs a built-in source this way, which is
// how the protocol is proved complete (the output of gloss suggest is
// the same either way); a plugin written in Go against this module
// could do the same. It returns when in ends, or with the first
// protocol error.
func Serve(src infer.Source, in io.Reader, out io.Writer) error {
	s := &server{enc: json.NewEncoder(out), dec: json.NewDecoder(in)}
	var m Message
	if err := s.dec.Decode(&m); err != nil {
		return fmt.Errorf("reading the index message: %w", err)
	}
	if m.Type != TypeIndex {
		return s.fail(fmt.Errorf("the first message must be %s, got %q", TypeIndex, m.Type))
	}
	if m.Contract != Contract {
		// Answer with our own version too, so the core can name both.
		s.enc.Encode(Message{Type: TypeIndexed, Contract: Contract, Name: src.Name()})
		return fmt.Errorf("gloss speaks contract v%d; this plugin speaks v%d", m.Contract, Contract)
	}
	v := &vocab{s: s, concepts: m.Vocabulary}
	if err := src.Index(infer.Files{Root: m.Root, List: m.Files}, v); err != nil {
		return s.fail(err)
	}
	items := map[string][]infer.Item{}
	for _, f := range m.Files {
		if its := src.Items(f.Path); its != nil {
			items[f.Path] = its
		}
	}
	indexed := Message{Type: TypeIndexed, Contract: Contract, Name: src.Name(), Items: items}
	if p, ok := src.(Prior); ok {
		indexed.Priors = p.Priors()
	}
	if err := s.enc.Encode(indexed); err != nil {
		return err
	}
	for {
		var q Message
		if err := s.dec.Decode(&q); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if q.Type != TypeHints || q.Decl == nil {
			return s.fail(fmt.Errorf("expected %s with a decl, got %q", TypeHints, q.Type))
		}
		if err := s.enc.Encode(Message{Type: TypeHints, Hints: src.Hints(*q.Decl)}); err != nil {
			return err
		}
	}
}

// Prior is what a served source may add to the index answer: the
// precision it expects of its bases, by "basis" or "basis/role".
type Prior interface {
	Priors() map[string]float64
}

type server struct {
	enc *json.Encoder
	dec *json.Decoder
}

func (s *server) fail(err error) error {
	s.enc.Encode(Message{Type: TypeError, Message: err.Error()})
	return err
}

// vocab is the plugin's side of infer.Vocabulary: the concepts came with
// the index message; the lookups go back over the wire.
type vocab struct {
	s        *server
	concepts []infer.Concept
}

func (v *vocab) Concepts() []infer.Concept { return v.concepts }

func (v *vocab) Named(name string, kinds ...string) []infer.Concept {
	m, err := v.ask(Message{Type: TypeNamed, Name: name, Kinds: kinds}, TypeConcepts)
	if err != nil {
		return nil
	}
	return m.Concepts
}

func (v *vocab) CommonAncestor(curies []string) string {
	m, err := v.ask(Message{Type: TypeAncestor, Curies: curies}, TypeAncestor)
	if err != nil {
		return ""
	}
	return m.Curie
}

func (v *vocab) ask(q Message, want string) (Message, error) {
	if err := v.s.enc.Encode(q); err != nil {
		return Message{}, err
	}
	var m Message
	if err := v.s.dec.Decode(&m); err != nil {
		return m, err
	}
	if m.Type != want {
		return m, fmt.Errorf("expected %s, got %q", want, m.Type)
	}
	return m, nil
}
