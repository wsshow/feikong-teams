package runtime

import "testing"

func TestResumeCommand(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{
			name:      "session",
			sessionID: "971835b5-ec98-45aa-8e9e-77b0ac6ed897",
			want:      "fkteams --resume 971835b5-ec98-45aa-8e9e-77b0ac6ed897",
		},
		{
			name:      "empty session",
			sessionID: "",
			want:      "",
		},
		{
			name:      "legacy cli session",
			sessionID: CLISessionID,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resumeCommand(tt.sessionID); got != tt.want {
				t.Fatalf("resumeCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
