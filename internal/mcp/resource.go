package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ResourceInfo describes a resource returned by resources/list.
type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult is the response to resources/list.
type ResourcesListResult struct {
	Resources []ResourceInfo `json:"resources"`
}

// ResourceContent is a content block returned by resources/read.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ResourceReadResult is the response to resources/read.
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ListResources calls resources/list to discover available resources.
func (c *Client) ListResources(ctx context.Context) ([]ResourceInfo, error) {
	resp, err := c.transport.Send(ctx, &Request{
		Method: "resources/list",
		Params: json.RawMessage(`{}`),
	})
	if err != nil {
		return nil, fmt.Errorf("resources/list: %w", err)
	}

	var result ResourcesListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse resources/list: %w", err)
	}
	return result.Resources, nil
}

// ReadResource calls resources/read to retrieve a specific resource.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ResourceReadResult, error) {
	params, err := json.Marshal(map[string]string{"uri": uri})
	if err != nil {
		return nil, fmt.Errorf("marshal read params: %w", err)
	}

	resp, err := c.transport.Send(ctx, &Request{
		Method: "resources/read",
		Params: params,
	})
	if err != nil {
		return nil, fmt.Errorf("resources/read %q: %w", uri, err)
	}

	var result ResourceReadResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse resources/read: %w", err)
	}
	return &result, nil
}
