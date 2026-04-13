package configtool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{}
	assert.Equal(t, "Config", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	tool := &Tool{}

	t.Run("get existing setting", func(t *testing.T) {
		viper.Set("model", "claude-3-opus")
		defer viper.Set("model", nil)

		input, _ := json.Marshal(Input{Setting: "model"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "claude-3-opus")
	})

	t.Run("get unset setting", func(t *testing.T) {
		input, _ := json.Marshal(Input{Setting: "nonexistent_key_xyz"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "not set")
	})

	t.Run("set disallowed key", func(t *testing.T) {
		val := any("something")
		input, _ := json.Marshal(Input{Setting: "api_key", Value: &val})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "not writable")
	})

	t.Run("missing setting", func(t *testing.T) {
		input, _ := json.Marshal(Input{})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
