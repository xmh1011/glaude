package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
)

func TestBuildChain_Linear(t *testing.T) {
	entries := []*Entry{
		{Type: "user", UUID: "a", Message: &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hello")}}},
		{Type: "assistant", UUID: "b", ParentUUID: "a", Message: &llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("hi")}}},
		{Type: "user", UUID: "c", ParentUUID: "b", Message: &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("how?")}}},
		{Type: "assistant", UUID: "d", ParentUUID: "c", Message: &llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("fine")}}},
	}

	chain := BuildChain(entries)
	require.Len(t, chain, 4)
	assert.Equal(t, "a", chain[0].UUID)
	assert.Equal(t, "b", chain[1].UUID)
	assert.Equal(t, "c", chain[2].UUID)
	assert.Equal(t, "d", chain[3].UUID)
}

func TestBuildChain_Fork(t *testing.T) {
	// DAG:  a -> b -> c (branch 1)
	//            b -> d (branch 2, leaf)
	entries := []*Entry{
		{Type: "user", UUID: "a", Message: &llm.Message{Role: llm.RoleUser}},
		{Type: "assistant", UUID: "b", ParentUUID: "a", Message: &llm.Message{Role: llm.RoleAssistant}},
		{Type: "user", UUID: "c", ParentUUID: "b", Message: &llm.Message{Role: llm.RoleUser}},
		{Type: "assistant", UUID: "d", ParentUUID: "b", Message: &llm.Message{Role: llm.RoleAssistant}},
	}

	chain := BuildChain(entries)
	// Should pick "d" as leaf (last entry without children), walk: d -> b -> a
	require.Len(t, chain, 3)
	assert.Equal(t, "a", chain[0].UUID)
	assert.Equal(t, "b", chain[1].UUID)
	assert.Equal(t, "d", chain[2].UUID)
}

func TestBuildChain_CycleDetection(t *testing.T) {
	entries := []*Entry{
		{Type: "user", UUID: "a", ParentUUID: "b", Message: &llm.Message{Role: llm.RoleUser}},
		{Type: "assistant", UUID: "b", ParentUUID: "a", Message: &llm.Message{Role: llm.RoleAssistant}},
	}

	chain := BuildChain(entries)
	// Should not infinite loop; returns partial chain
	assert.NotNil(t, chain)
	assert.LessOrEqual(t, len(chain), 2)
}

func TestBuildChain_Empty(t *testing.T) {
	assert.Nil(t, BuildChain(nil))
	assert.Nil(t, BuildChain([]*Entry{}))
}

func TestBuildChain_SkipsMetadata(t *testing.T) {
	entries := []*Entry{
		{Type: "title", UUID: "t1", Text: "My Session"},
		{Type: "user", UUID: "a", Message: &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hi")}}},
		{Type: "last-prompt", UUID: "lp", Text: "hi"},
		{Type: "assistant", UUID: "b", ParentUUID: "a", Message: &llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("hey")}}},
	}

	chain := BuildChain(entries)
	require.Len(t, chain, 2)
	assert.Equal(t, "a", chain[0].UUID)
	assert.Equal(t, "b", chain[1].UUID)
}

func TestToMessages(t *testing.T) {
	chain := []*Entry{
		{Type: "user", UUID: "a", Message: &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hello")}}},
		{Type: "assistant", UUID: "b", Message: &llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("world")}}},
	}

	msgs := ToMessages(chain)
	require.Len(t, msgs, 2)
	assert.Equal(t, llm.RoleUser, msgs[0].Role)
	assert.Equal(t, llm.RoleAssistant, msgs[1].Role)
}

func TestToMessages_NilMessage(t *testing.T) {
	chain := []*Entry{
		{Type: "user", UUID: "a", Message: nil}, // should be skipped
		{Type: "assistant", UUID: "b", Message: &llm.Message{Role: llm.RoleAssistant}},
	}

	msgs := ToMessages(chain)
	require.Len(t, msgs, 1)
}
