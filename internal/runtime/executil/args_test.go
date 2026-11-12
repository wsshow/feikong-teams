package executil

import "testing"

func TestValidateOperandsRejectsOptionInjection(t *testing.T) {
	for _, values := range [][]string{{"--python=/outside"}, {"pkg", "--target=/outside"}, {"pkg\n--flag"}, {""}} {
		if err := ValidateOperands(values); err == nil {
			t.Fatalf("accepted option injection: %q", values)
		}
	}
	if err := ValidateOperands([]string{"requests==2.0", "@scope/package@1.0", "git+https://example.com/repo"}); err != nil {
		t.Fatal(err)
	}
}
