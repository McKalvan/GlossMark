package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const sdl = `type Query {
  me: Customer!
  # The order with that id, if it is the caller's.
  order(id: ID!): Order
}

type Mutation {
  placeOrder(input: PlaceOrderInput!): Order!
}

"""
What a customer asked to have delivered.
"""
type Order {
  id: ID!
  lines: [OrderLine!]!
}

type OrderLine {
  sku: String!
}

input PlaceOrderInput {
  lines: [OrderLineInput!]!
}

enum Status { OPEN }
`

func TestReadSchema(t *testing.T) {
	s := &schema{}
	items := s.read(sdl)
	got := map[string]item{}
	for _, it := range items {
		got[it.Symbol] = it
	}
	if len(items) != 5 {
		t.Errorf("%d items: %+v", len(items), items)
	}
	if it := got["Mutation.placeOrder"]; it.Role != "operation" || it.Relation != "documents" || it.Line != 8 || it.End != 8 || it.Indent != "  " || strings.Join(it.Names, ",") != "place order" || strings.Join(it.Kinds, ",") != "process,capability" {
		t.Errorf("placeOrder: %+v", it)
	}
	if it := got["Query.order"]; it.Doc != "The order with that id, if it is the caller's." || it.Line != 4 || !strings.Contains(strings.Join(it.Kinds, ","), "entity") {
		t.Errorf("order: %+v", it)
	}
	if it := got["Order"]; it.Role != "type" || it.Relation != "represents" || it.Line != 14 || it.End != 17 || it.Doc != "What a customer asked to have delivered." || it.Basis != "type" {
		t.Errorf("Order: %+v", it)
	}
	if _, ok := got["PlaceOrderInput"]; ok {
		t.Error("an input type is a shape of the request, not an entity")
	}
	if _, ok := got["Status"]; ok {
		t.Error("an enum was proposed")
	}
}

func TestHints(t *testing.T) {
	s := &schema{}
	s.read(sdl)
	hs := s.hints(decl{Path: "resolvers.ts", Symbol: "placeOrder", Role: "func", Line: 10, Body: "placeOrder: async (_, { input }, ctx) => {\n  return ctx.orders.place(ctx.user.customerId, input);\n}"})
	if len(hs) != 1 || hs[0].Relation != "implements" || hs[0].Names[0] != "place order" || hs[0].Via != "placeOrder" || hs[0].Line != 10 || hs[0].Basis != "field" {
		t.Errorf("hints: %+v", hs)
	}
	hs = s.hints(decl{Path: "placement.py", Symbol: "place_order", Role: "func", Line: 20, Body: "def place_order(customer, lines):\n    return Order()"})
	if len(hs) != 1 || hs[0].Terms != "placeOrder" || hs[0].Via != "place_order" {
		t.Errorf("snake_case: %+v", hs)
	}
	hs = s.hints(decl{Path: "models.py", Symbol: "Order", Role: "type", Line: 4, Body: "class Order:"})
	if len(hs) != 1 || hs[0].Relation != "represents" || hs[0].Names[0] != "order" {
		t.Errorf("type: %+v", hs)
	}
	if hs := s.hints(decl{Symbol: "unrelated", Role: "func", Body: "x = 1"}); len(hs) != 0 {
		t.Errorf("unrelated: %+v", hs)
	}
}

func TestWords(t *testing.T) {
	for in, want := range map[string]string{"placeOrder": "place order", "OrderLine": "order line", "place_order": "place order", "HTTPServer": "http server", "me": "me"} {
		if got := words(in); got != want {
			t.Errorf("words(%s) = %q, want %q", in, got, want)
		}
	}
}

// TestServeExchange runs the whole protocol: index, read, indexed, hints.
func TestServeExchange(t *testing.T) {
	var in strings.Builder
	enc := json.NewEncoder(&in)
	enc.Encode(message{Type: "index", Contract: contract, Root: "/w", Files: []file{{Path: "gateway/schema.graphql"}, {Path: "a.py"}}})
	enc.Encode(message{Type: "files", Contents: []content{{Path: "gateway/schema.graphql", Content: sdl}}})
	enc.Encode(message{Type: "hints", Decl: &decl{Path: "a.py", Symbol: "place_order", Role: "func", Line: 3, Body: "def place_order(): pass"}})
	var out strings.Builder
	if err := serve(strings.NewReader(in.String()), &out); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(out.String()))
	var ms []message
	for dec.More() {
		var m message
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	if len(ms) != 3 || ms[0].Type != "read" || ms[1].Type != "indexed" || ms[2].Type != "hints" {
		t.Fatalf("messages: %+v", ms)
	}
	if ms[1].Name != "graphql" || ms[1].Contract != contract || len(ms[1].Items["gateway/schema.graphql"]) != 5 || ms[1].Priors["type"] != 0.7 {
		t.Errorf("indexed: %+v", ms[1])
	}
	if len(ms[2].Hints) != 1 || ms[2].Hints[0].Terms != "placeOrder" {
		t.Errorf("hints: %+v", ms[2])
	}
}

func TestServeRejectsOtherContract(t *testing.T) {
	var out strings.Builder
	err := serve(strings.NewReader(`{"type":"index","contract":7}`+"\n"), &out)
	if err == nil || !strings.Contains(err.Error(), "v7") || !strings.Contains(err.Error(), "v1") || !strings.Contains(out.String(), `"contract":1`) {
		t.Errorf("err %v, out %s", err, out.String())
	}
}
