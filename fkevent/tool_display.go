package fkevent

import (
	"strings"
	"sync"
	"unicode"
)

const agentToolPrefix = "ask_"

var agentToolDisplays sync.Map

type ToolDisplay struct {
	Name        string
	DisplayName string
	Kind        string
	Target      string
}

func RegisterAgentToolDisplay(toolName, displayName string) {
	if toolName == "" {
		return
	}
	target := displayName
	if target == "" {
		target = titleIdentifier(strings.TrimPrefix(toolName, agentToolPrefix))
	}
	agentToolDisplays.Store(toolName, ToolDisplay{
		Name:        toolName,
		DisplayName: "指派给 " + target,
		Kind:        "agent",
		Target:      target,
	})
}

func FormatToolDisplay(name string) ToolDisplay {
	display := ToolDisplay{
		Name:        name,
		DisplayName: name,
		Kind:        "tool",
	}

	if value, ok := agentToolDisplays.Load(name); ok {
		return value.(ToolDisplay)
	}
	return display
}

func titleIdentifier(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}
