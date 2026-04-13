// Package askuser implements the AskUserQuestion tool that lets the Agent
// present interactive questions with selectable options to the user.
package askuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// QuestionOption is a single selectable choice within a question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Question represents one question with a header, options, and multi-select flag.
type Question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
}

// AnswerFunc is a callback that presents questions to the user and returns answers.
// The map keys are question texts, values are selected option labels (or custom input).
// The bool indicates whether the user answered (false if cancelled).
type AnswerFunc func(ctx context.Context, questions []Question) (map[string]string, bool)

// Tool implements the AskUserQuestion tool for interactive user prompts.
type Tool struct {
	AnswerFn AnswerFunc
}

// Name returns the tool's unique identifier.
func (t *Tool) Name() string { return "AskUserQuestion" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Ask the user questions during execution. " +
		"Supports 1-4 questions, each with 2-4 options. " +
		"Use this to gather preferences, clarify instructions, or get decisions."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"description": "Questions to ask the user (1-4 questions)",
				"items": {
					"type": "object",
					"properties": {
						"question": {
							"type": "string",
							"description": "The complete question to ask"
						},
						"header": {
							"type": "string",
							"description": "Short label displayed as a tag (max 12 chars)"
						},
						"options": {
							"type": "array",
							"description": "Available choices (2-4 options)",
							"items": {
								"type": "object",
								"properties": {
									"label": {
										"type": "string",
										"description": "Display text for this option"
									},
									"description": {
										"type": "string",
										"description": "Explanation of what this option means"
									}
								},
								"required": ["label", "description"]
							},
							"minItems": 2,
							"maxItems": 4
						},
						"multiSelect": {
							"type": "boolean",
							"description": "Allow multiple selections (default false)"
						}
					},
					"required": ["question", "header", "options", "multiSelect"]
				},
				"minItems": 1,
				"maxItems": 4
			}
		},
		"required": ["questions"]
	}`)
}

// IsReadOnly returns true — AskUserQuestion does not modify state.
func (t *Tool) IsReadOnly() bool { return true }

// toolInput is the deserialized tool input.
type toolInput struct {
	Questions []Question `json:"questions"`
}

// Execute presents questions to the user and returns their answers.
func (t *Tool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in toolInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if err := validate(in.Questions); err != nil {
		return "", err
	}

	if t.AnswerFn == nil {
		return "", fmt.Errorf("AskUserQuestion requires an interactive session (no answer handler configured)")
	}

	answers, ok := t.AnswerFn(ctx, in.Questions)
	if !ok {
		return "User cancelled the question prompt.", nil
	}

	return formatAnswers(in.Questions, answers), nil
}

// validate checks that questions conform to the constraints.
func validate(questions []Question) error {
	if len(questions) == 0 {
		return fmt.Errorf("at least 1 question is required")
	}
	if len(questions) > 4 {
		return fmt.Errorf("at most 4 questions are allowed, got %d", len(questions))
	}

	seen := make(map[string]bool)
	for i, q := range questions {
		if q.Question == "" {
			return fmt.Errorf("question %d: question text is required", i+1)
		}
		if seen[q.Question] {
			return fmt.Errorf("question %d: duplicate question text %q", i+1, q.Question)
		}
		seen[q.Question] = true

		if len(q.Options) < 2 {
			return fmt.Errorf("question %d: at least 2 options required, got %d", i+1, len(q.Options))
		}
		if len(q.Options) > 4 {
			return fmt.Errorf("question %d: at most 4 options allowed, got %d", i+1, len(q.Options))
		}

		labels := make(map[string]bool)
		for j, opt := range q.Options {
			if opt.Label == "" {
				return fmt.Errorf("question %d option %d: label is required", i+1, j+1)
			}
			if labels[opt.Label] {
				return fmt.Errorf("question %d: duplicate option label %q", i+1, opt.Label)
			}
			labels[opt.Label] = true
		}
	}
	return nil
}

// formatAnswers builds a human-readable summary of the user's answers.
func formatAnswers(questions []Question, answers map[string]string) string {
	var parts []string
	for _, q := range questions {
		ans, ok := answers[q.Question]
		if !ok {
			ans = "(no answer)"
		}
		parts = append(parts, fmt.Sprintf("%q = %q", q.Question, ans))
	}
	return fmt.Sprintf("User has answered your questions: %s. You can now continue.", strings.Join(parts, ", "))
}
