package validate

import "testing"

func TestTrim(t *testing.T) {
	if trim("abcdef", 3) != "abc\n...truncated" {
		t.Fatal("trim")
	}
}
