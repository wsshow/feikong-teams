package mcp

import (
	"context"
	"fmt"
	"sync"

	runtimeport "fkteams/internal/ports/runtime"
	toolport "fkteams/internal/ports/tools"
	"fkteams/internal/runtime/log"

	"github.com/mark3labs/mcp-go/client"
)

type ToolProvider func(context.Context, *client.Client) ([]runtimeport.Tool, error)

type managedClient interface {
	Close() error
}

type groupLoader func(context.Context, ToolProvider) (toolport.MCPToolGroups, []managedClient, error)

type Provider struct {
	mu           sync.RWMutex
	loadMu       sync.Mutex
	cachedGroups toolport.MCPToolGroups
	toolProvider ToolProvider
	clients      []managedClient
	loader       groupLoader
}

func NewProvider() *Provider {
	return &Provider{loader: loadToolGroups}
}

func (p *Provider) RegisterToolProvider(provider ToolProvider) {
	p.loadMu.Lock()
	defer p.loadMu.Unlock()
	p.mu.Lock()
	clients := p.clients
	p.toolProvider = provider
	p.cachedGroups = nil
	p.clients = nil
	p.mu.Unlock()
	closeManagedClients(clients)
}

func (p *Provider) GetToolsByName(ctx context.Context, groupName string) ([]runtimeport.Tool, error) {
	groups, err := p.GetAllToolGroups(ctx)
	if err != nil {
		return nil, err
	}
	if group, exists := groups[groupName]; exists {
		return group.Tools, nil
	}
	return nil, fmt.Errorf("MCP tool %s not found", groupName)
}

func (p *Provider) GetAllToolGroups(ctx context.Context) (toolport.MCPToolGroups, error) {
	p.mu.RLock()
	cached := p.cachedGroups
	p.mu.RUnlock()
	if cached != nil {
		return cloneGroups(cached), nil
	}

	p.loadMu.Lock()
	defer p.loadMu.Unlock()
	p.mu.RLock()
	cached = p.cachedGroups
	toolProvider := p.toolProvider
	loader := p.loader
	p.mu.RUnlock()
	if cached != nil {
		return cloneGroups(cached), nil
	}
	if loader == nil {
		loader = loadToolGroups
	}
	groups, clients, err := loader(ctx, toolProvider)
	if err != nil {
		closeManagedClients(clients)
		return nil, err
	}
	p.mu.Lock()
	oldClients := p.clients
	p.cachedGroups = cloneGroups(groups)
	p.clients = clients
	p.mu.Unlock()
	closeManagedClients(oldClients)
	return cloneGroups(groups), nil
}

func (p *Provider) ClearCache() {
	p.loadMu.Lock()
	defer p.loadMu.Unlock()
	p.mu.Lock()
	clients := p.clients
	p.cachedGroups = nil
	p.clients = nil
	p.mu.Unlock()
	closeManagedClients(clients)
}

func loadToolGroups(ctx context.Context, toolProvider ToolProvider) (toolport.MCPToolGroups, []managedClient, error) {
	clients, err := setupMCPClients(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(clients) == 0 {
		return toolport.MCPToolGroups{}, nil, nil
	}
	if toolProvider == nil {
		closeMCPClients(clients)
		return nil, nil, fmt.Errorf("MCP tool provider is not registered")
	}

	groups := make(toolport.MCPToolGroups, len(clients))
	managed := make([]managedClient, 0, len(clients))
	for _, item := range clients {
		managed = append(managed, item.Client)
	}
	for _, mcpClient := range clients {
		tools, err := toolProvider(ctx, mcpClient.Client)
		if err != nil {
			closeManagedClients(managed)
			return nil, nil, fmt.Errorf("failed to get tools from MCP server %s: %w", mcpClient.Name, err)
		}
		groups[mcpClient.Name] = toolport.MCPToolGroup{
			Name:  mcpClient.Name,
			Desc:  mcpClient.Desc,
			Tools: tools,
		}
	}
	return groups, managed, nil
}

func closeManagedClients(clients []managedClient) {
	for _, client := range clients {
		if client == nil {
			continue
		}
		if err := client.Close(); err != nil {
			log.Printf("failed to close MCP client: %v", err)
		}
	}
}

func cloneGroups(groups toolport.MCPToolGroups) toolport.MCPToolGroups {
	if groups == nil {
		return nil
	}
	clone := make(toolport.MCPToolGroups, len(groups))
	for name, group := range groups {
		group.Tools = append([]runtimeport.Tool(nil), group.Tools...)
		clone[name] = group
	}
	return clone
}
