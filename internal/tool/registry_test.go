package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.NotNil(t, r.Get("Alpha"))
	assert.Nil(t, r.Get("Missing"))
}

func TestRegistry_AllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Zeta"})
	r.Register(&stubTool{name: "Alpha"})
	r.Register(&stubTool{name: "Mu"})

	all := r.All()
	require.Len(t, all, 3)
	assert.Equal(t, "Alpha", all[0].Name())
	assert.Equal(t, "Mu", all[1].Name())
	assert.Equal(t, "Zeta", all[2].Name())
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	assert.Panics(t, func() {
		r := NewRegistry()
		r.Register(&stubTool{name: "Same"})
		r.Register(&stubTool{name: "Same"})
	})
}

func TestRegistry_Definitions(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "Read", readOnly: true})

	defs := r.Definitions()
	require.Len(t, defs, 1)
	assert.Equal(t, "Read", defs[0].Name)
	assert.Equal(t, "stub: Read", defs[0].Description)
}
