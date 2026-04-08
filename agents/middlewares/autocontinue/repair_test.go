package autocontinue

import (
	"encoding/json"
	"testing"
)

func TestRepairJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"already valid", `{"key":"value"}`, true},
		{"truncated string value", `{"filepath":"/file.go","content":"package main\nfunc`, true},
		{"truncated key", `{"filepath":"/file.go","cont`, true},
		{"unclosed object", `{"filepath":"/file.go"`, true},
		{"unclosed array", `{"items":[1,2,3`, true},
		{"nested objects", `{"outer":{"inner":"val`, true},
		{"trailing comma", `{"key":"value",`, true},
		{"trailing colon", `{"key":`, true},
		{"escaped quote", `{"content":"line with \"quote`, true},
		{"trailing backslash", `{"content":"path\`, true},
		{"empty", "", false},
		{"complex write", `{"filepath":"src/main.go","content":"package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(\"Hello`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repairJSON(tt.input)
			isValid := json.Valid([]byte(result))
			if tt.valid && !isValid {
				t.Errorf("expected valid JSON, got invalid: %s", result)
			}
		})
	}
}

func TestRepairJSON_PreservesContent(t *testing.T) {
	input := `{"filepath":"/file.go","content":"hello world`
	result := repairJSON(input)

	var parsed map[string]string
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v\nresult: %s", err, result)
	}
	if parsed["filepath"] != "/file.go" {
		t.Errorf("filepath: got %q", parsed["filepath"])
	}
	if parsed["content"] != "hello world" {
		t.Errorf("content: got %q", parsed["content"])
	}
}
