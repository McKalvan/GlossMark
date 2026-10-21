// @gloss implements gmi:MappingSets scope=file

package mapping

// Resolved is a row's endpoints and predicate as IRIs.
type Resolved struct {
	Subject, Object string
	Predicate       Predicate
	Supported       bool
	// Unresolved lists endpoint CURIEs no prefix resolves; Undeclared those
	// resolved only through the workspace's prefixes (not the set's curie_map).
	Unresolved, Undeclared []string
}

// Resolve expands a row's predicate and endpoints, falling back to the
// workspace's prefixes for endpoints the set's curie_map does not declare.
func (s *Set) Resolve(r *Row, prefixes map[string]string) Resolved {
	var out Resolved
	if iri, _, ok := s.Expand(r.Get("predicate_id"), nil); ok {
		if p, known := Predicates[iri]; known {
			out.Predicate, out.Supported = p, true
		}
	}
	if !out.Supported {
		out.Predicate = Predicate{CURIE: r.Get("predicate_id")}
	}
	one := func(curie string) string {
		iri, declared, ok := s.Expand(curie, prefixes)
		if !ok {
			out.Unresolved = append(out.Unresolved, curie)
			return ""
		}
		if !declared {
			out.Undeclared = append(out.Undeclared, curie)
		}
		return iri
	}
	out.Subject = one(r.Get("subject_id"))
	out.Object = one(r.Get("object_id"))
	return out
}
