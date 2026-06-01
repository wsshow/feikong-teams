package fkevent

import (
	"strings"
	"unicode"
)

const agentToolPrefix = "ask_"

var agentToolLabels = map[string]string{
	"coder":           "Coder",
	"shell":           "Shell",
	"writer":          "Writer",
	"summarizer":      "Summarizer",
	"researcher":      "Researcher",
	"analyst":         "Analyst",
	"remote":          "Remote",
	"generalist":      "Generalist",
	"tasker":          "Tasker",
	"moderator":       "Moderator",
	"coordinator":     "Coordinator",
	"deep_researcher": "Deep Researcher",
}

type ToolDisplay struct {
	Name        string
	DisplayName string
	Kind        string
	Target      string
}

func FormatToolDisplay(name string) ToolDisplay {
	display := ToolDisplay{
		Name:        name,
		DisplayName: name,
		Kind:        "tool",
	}

	target, ok := strings.CutPrefix(name, agentToolPrefix)
	if !ok {
		return display
	}

	label := agentToolLabels[target]
	if label == "" {
		label = titleIdentifier(target)
	}
	display.Kind = "agent"
	display.Target = label
	display.DisplayName = "指派给 " + label
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
