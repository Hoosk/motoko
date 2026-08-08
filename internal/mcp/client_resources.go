package mcp

import "context"

// ListResources returns the first page of resources.
func (c *Client) ListResources(ctx context.Context, cursor string) (*ListResourcesResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListResourcesResult
	if err := c.Request(ctx, "resources/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllResources paginates through ListResources until completion.
func (c *Client) ListAllResources(ctx context.Context) ([]Resource, error) {
	var (
		cursor string
		all    []Resource
	)
	for {
		page, err := c.ListResources(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Resources...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// ListResourceTemplates returns the first page of resource templates.
func (c *Client) ListResourceTemplates(ctx context.Context, cursor string) (*ListResourceTemplatesResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListResourceTemplatesResult
	if err := c.Request(ctx, "resources/templates/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllResourceTemplates paginates through ListResourceTemplates until completion.
func (c *Client) ListAllResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	var (
		cursor string
		all    []ResourceTemplate
	)
	for {
		page, err := c.ListResourceTemplates(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.ResourceTemplates...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// ReadResource fetches the contents of a resource by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	params := struct {
		URI string `json:"uri"`
	}{URI: uri}
	var result ReadResourceResult
	if err := c.Request(ctx, "resources/read", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Subscribe registers interest in a resource to receive notifications/resources/updated.
// Deprecated in 2026-07-28; legacy servers still use it.
func (c *Client) Subscribe(ctx context.Context, uri string) error {
	params := struct {
		URI string `json:"uri"`
	}{URI: uri}
	return c.Request(ctx, "resources/subscribe", params, nil)
}

// SubscriptionFilter selects the notification types a subscriptions/listen
// stream should deliver (spec 2026-07-28).
type SubscriptionFilter struct {
	ResourceSubscriptions []string `json:"resourceSubscriptions,omitempty"`
	ToolsListChanged      bool     `json:"toolsListChanged,omitempty"`
	PromptsListChanged    bool     `json:"promptsListChanged,omitempty"`
	ResourcesListChanged  bool     `json:"resourcesListChanged,omitempty"`
}

// OpenSubscriptionStream opens a long-lived notification stream (spec
// 2026-07-28). The POST returns immediately; notifications and the optional
// graceful-close response arrive on the request's SSE response stream and
// are routed to the client's OnNotification callback by the read loop.
func (c *Client) OpenSubscriptionStream(ctx context.Context, filter SubscriptionFilter) error {
	params := struct {
		Notifications SubscriptionFilter `json:"notifications"`
	}{Notifications: filter}
	return c.Send(ctx, "subscriptions/listen", params)
}

// Unsubscribe unregisters interest in a resource.
func (c *Client) Unsubscribe(ctx context.Context, uri string) error {
	params := struct {
		URI string `json:"uri"`
	}{URI: uri}
	return c.Request(ctx, "resources/unsubscribe", params, nil)
}

// ListPrompts returns the first page of prompts.
func (c *Client) ListPrompts(ctx context.Context, cursor string) (*ListPromptsResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListPromptsResult
	if err := c.Request(ctx, "prompts/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllPrompts paginates through ListPrompts until completion.
func (c *Client) ListAllPrompts(ctx context.Context) ([]Prompt, error) {
	var (
		cursor string
		all    []Prompt
	)
	for {
		page, err := c.ListPrompts(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Prompts...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// GetPrompt retrieves a prompt by name and arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]string) (*GetPromptResult, error) {
	params := struct {
		Arguments map[string]string `json:"arguments,omitempty"`
		Name      string            `json:"name"`
	}{Name: name, Arguments: arguments}
	var result GetPromptResult
	if err := c.Request(ctx, "prompts/get", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
