package mcp

import (
	"bufio"
	"context"
	"io"
	"testing"
	"time"
)

func TestClientResourcesAndPrompts(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := runFakeServer(ctx, serverReader, serverWriter, nil, false)
	defer srv.Shutdown()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "test", Version: "0"},
		RequestTimeout: 2 * time.Second,
	})
	client.Start(ctx)
	defer client.Close()

	_, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Test Resources
	resList, err := client.ListAllResources(ctx)
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resList) != 1 || resList[0].Name != "test.txt" {
		t.Errorf("unexpected resources: %+v", resList)
	}

	templates, err := client.ListAllResourceTemplates(ctx)
	if err != nil {
		t.Fatalf("ListAllResourceTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].Name != "Project Files" {
		t.Errorf("unexpected templates: %+v", templates)
	}

	readRes, err := client.ReadResource(ctx, "file:///test.txt")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(readRes.Contents) != 1 || readRes.Contents[0].Text != "Hello, resources!" {
		t.Errorf("unexpected read content: %+v", readRes)
	}

	err = client.Subscribe(ctx, "file:///test.txt")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	err = client.Unsubscribe(ctx, "file:///test.txt")
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	// Test Prompts
	promptList, err := client.ListAllPrompts(ctx)
	if err != nil {
		t.Fatalf("ListAllPrompts: %v", err)
	}
	if len(promptList) != 1 || promptList[0].Name != "test-prompt" {
		t.Errorf("unexpected prompts: %+v", promptList)
	}

	pGet, err := client.GetPrompt(ctx, "test-prompt", nil)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(pGet.Messages) != 1 || pGet.Messages[0].Content.Text != "Hello from test prompt!" {
		t.Errorf("unexpected prompt contents: %+v", pGet)
	}
}

func TestManagerResourcesAndPrompts(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := runFakeServer(ctx, serverReader, serverWriter, nil, false)
	defer srv.Shutdown()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "test", Version: "0"},
		RequestTimeout: 2 * time.Second,
	})
	client.Start(ctx)
	defer client.Close()

	_, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	m := NewManager(ManagerConfig{
		Timeout: 2 * time.Second,
	})

	entry := &managedServer{
		cfg:    ServerConfig{Name: "fake"},
		client: client,
	}
	m.mu.Lock()
	m.servers["fake"] = entry
	m.mu.Unlock()

	// Trigger manual refreshes to populate cache
	if err = m.refreshResources(ctx, entry); err != nil {
		t.Fatalf("refreshResources: %v", err)
	}
	if err = m.refreshTemplates(ctx, entry); err != nil {
		t.Fatalf("refreshTemplates: %v", err)
	}
	if err = m.refreshPrompts(ctx, entry); err != nil {
		t.Fatalf("refreshPrompts: %v", err)
	}

	// Validate public aggregated queries
	resList := m.ListResources(ctx)
	if len(resList) != 1 || resList[0].Name != "test.txt" {
		t.Errorf("ListResources cache failed: %+v", resList)
	}

	templates := m.ListResourceTemplates(ctx)
	if len(templates) != 1 || templates[0].Name != "Project Files" {
		t.Errorf("ListResourceTemplates cache failed: %+v", templates)
	}

	prompts := m.ListPrompts(ctx)
	if len(prompts) != 1 || prompts[0].Name != "test-prompt" {
		t.Errorf("ListPrompts cache failed: %+v", prompts)
	}

	readRes, err := m.ReadResource(ctx, "fake", "file:///test.txt")
	if err != nil {
		t.Fatalf("Manager ReadResource failed: %v", err)
	}
	if len(readRes.Contents) != 1 || readRes.Contents[0].Text != "Hello, resources!" {
		t.Errorf("unexpected manager read: %+v", readRes)
	}

	pGet, err := m.GetPrompt(ctx, "fake", "test-prompt", nil)
	if err != nil {
		t.Fatalf("Manager GetPrompt failed: %v", err)
	}
	if len(pGet.Messages) != 1 || pGet.Messages[0].Content.Text != "Hello from test prompt!" {
		t.Errorf("unexpected manager prompt get: %+v", pGet)
	}
}
