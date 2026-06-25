package command

import (
	"bytes"
	"testing"
)

func TestLimitedWriter(t *testing.T) {
	var buf bytes.Buffer
	writer := &limitedWriter{w: &buf, limit: 5}

	n, err := writer.Write([]byte("abc"))
	if err != nil {
		t.Fatalf("first write error: %v", err)
	}
	if n != 3 || buf.String() != "abc" || writer.truncated {
		t.Fatalf("unexpected first write state n=%d buf=%q truncated=%v", n, buf.String(), writer.truncated)
	}

	n, err = writer.Write([]byte("defgh"))
	if err != nil {
		t.Fatalf("second write error: %v", err)
	}
	if n != 5 {
		t.Fatalf("second write reported n=%d, want original length 5", n)
	}
	if buf.String() != "abcde" {
		t.Fatalf("limited buffer = %q, want abcde", buf.String())
	}
	if !writer.truncated {
		t.Fatal("expected writer to mark truncation")
	}

	n, err = writer.Write([]byte("ignored"))
	if err != nil {
		t.Fatalf("third write error: %v", err)
	}
	if n != len("ignored") || buf.String() != "abcde" {
		t.Fatalf("third write n=%d buf=%q", n, buf.String())
	}
}
