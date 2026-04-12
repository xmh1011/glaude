package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type stubTool struct {
	name     string
	readOnly bool
}

func (s *stubTool) Name() string                  { return s.name }
func (s *stubTool) Description() string            { return "stub: " + s.name }
func (s *stubTool) InputSchema() json.RawMessage   { return json.RawMessage(`{"type":"object"}`) }
func (s *stubTool) IsReadOnly() bool               { return s.readOnly }
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "ok", nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Alpha"})
	r.Register(&stubTool{name: "Beta"})

	if got := r.Get("Alpha"); got == nil {
		t.Fatal("expected to find Alpha")
	}
	if got := r.Get("Missing"); got != nil {
		t.Fatal("expected nil for missing tool")
	}
}

func TestRegistry_AllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Zeta"})
	r.Register(&stubTool{name: "Alpha"})
	r.Register(&stubTool{name: "Mu"})

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(all))
	}
	if all[0].Name() != "Alpha" || all[1].Name() != "Mu" || all[2].Name() != "Zeta" {
		t.Fatalf("expected sorted order, got %s %s %s", all[0].Name(), all[1].Name(), all[2].Name())
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()

	r := NewRegistry()
	r.Register(&stubTool{name: "Same"})
	r.Register(&stubTool{name: "Same"})
}

func TestRegistry_Definitions(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Read", readOnly: true})

	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "Read" {
		t.Fatalf("expected name Read, got %s", defs[0].Name)
	}
	if defs[0].Description != "stub: Read" {
		t.Fatalf("unexpected description: %s", defs[0].Description)
	}
}
