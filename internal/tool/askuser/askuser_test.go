package askuser

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{}

	assert.Equal(t, "AskUserQuestion", tool.Name())
	assert.True(t, tool.IsReadOnly())
	assert.NotEmpty(t, tool.Description())

	var schema map[string]interface{}
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute_Success(t *testing.T) {
	tool := &Tool{
		AnswerFn: func(_ context.Context, questions []Question) (map[string]string, bool) {
			answers := make(map[string]string)
			for _, q := range questions {
				answers[q.Question] = q.Options[0].Label
			}
			return answers, true
		},
	}

	input, _ := json.Marshal(map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"question":    "Which framework?",
				"header":      "Framework",
				"multiSelect": false,
				"options": []map[string]string{
					{"label": "React", "description": "Popular UI library"},
					{"label": "Vue", "description": "Progressive framework"},
				},
			},
		},
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "User has answered")
	assert.Contains(t, result, "Which framework?")
	assert.Contains(t, result, "React")
}

func TestTool_Execute_MultipleQuestions(t *testing.T) {
	tool := &Tool{
		AnswerFn: func(_ context.Context, questions []Question) (map[string]string, bool) {
			return map[string]string{
				"Color?": "Blue",
				"Size?":  "Large",
			}, true
		},
	}

	input, _ := json.Marshal(map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"question":    "Color?",
				"header":      "Color",
				"multiSelect": false,
				"options": []map[string]string{
					{"label": "Red", "description": "Warm"},
					{"label": "Blue", "description": "Cool"},
				},
			},
			{
				"question":    "Size?",
				"header":      "Size",
				"multiSelect": false,
				"options": []map[string]string{
					{"label": "Small", "description": "Compact"},
					{"label": "Large", "description": "Spacious"},
				},
			},
		},
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, `"Color?" = "Blue"`)
	assert.Contains(t, result, `"Size?" = "Large"`)
}

func TestTool_Execute_Cancelled(t *testing.T) {
	tool := &Tool{
		AnswerFn: func(_ context.Context, _ []Question) (map[string]string, bool) {
			return nil, false
		},
	}

	input, _ := json.Marshal(map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"question":    "Test?",
				"header":      "Test",
				"multiSelect": false,
				"options": []map[string]string{
					{"label": "A", "description": "opt A"},
					{"label": "B", "description": "opt B"},
				},
			},
		},
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "cancelled")
}

func TestTool_Execute_NoAnswerFn(t *testing.T) {
	tool := &Tool{} // no AnswerFn set

	input, _ := json.Marshal(map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"question":    "Test?",
				"header":      "Test",
				"multiSelect": false,
				"options": []map[string]string{
					{"label": "A", "description": "opt A"},
					{"label": "B", "description": "opt B"},
				},
			},
		},
	})

	_, err := tool.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "interactive session")
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		qs      []Question
		wantErr string
	}{
		{
			name:    "no questions",
			qs:      nil,
			wantErr: "at least 1",
		},
		{
			name: "too many questions",
			qs: []Question{
				{Question: "Q1", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
				{Question: "Q2", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
				{Question: "Q3", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
				{Question: "Q4", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
				{Question: "Q5", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
			},
			wantErr: "at most 4",
		},
		{
			name: "empty question text",
			qs: []Question{
				{Question: "", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
			},
			wantErr: "question text is required",
		},
		{
			name: "duplicate question",
			qs: []Question{
				{Question: "Same?", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
				{Question: "Same?", Options: []QuestionOption{{Label: "c"}, {Label: "d"}}},
			},
			wantErr: "duplicate question",
		},
		{
			name: "too few options",
			qs: []Question{
				{Question: "Q?", Options: []QuestionOption{{Label: "a"}}},
			},
			wantErr: "at least 2",
		},
		{
			name: "too many options",
			qs: []Question{
				{Question: "Q?", Options: []QuestionOption{
					{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}, {Label: "e"},
				}},
			},
			wantErr: "at most 4",
		},
		{
			name: "empty option label",
			qs: []Question{
				{Question: "Q?", Options: []QuestionOption{{Label: ""}, {Label: "b"}}},
			},
			wantErr: "label is required",
		},
		{
			name: "duplicate option labels",
			qs: []Question{
				{Question: "Q?", Options: []QuestionOption{{Label: "same"}, {Label: "same"}}},
			},
			wantErr: "duplicate option label",
		},
		{
			name: "valid single question",
			qs: []Question{
				{Question: "Q?", Options: []QuestionOption{{Label: "A"}, {Label: "B"}}},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.qs)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFormatAnswers(t *testing.T) {
	questions := []Question{
		{Question: "Color?"},
		{Question: "Size?"},
	}
	answers := map[string]string{
		"Color?": "Blue",
		"Size?":  "Large",
	}

	result := formatAnswers(questions, answers)
	assert.Contains(t, result, "User has answered")
	assert.Contains(t, result, `"Color?" = "Blue"`)
	assert.Contains(t, result, `"Size?" = "Large"`)
	assert.Contains(t, result, "You can now continue")
}
