package generate

import (
	"strings"
	"testing"
)

func TestIslandTagMustStartWithLetter(t *testing.T) {
	const line = "//templatic:island tag=1-chart props=P"
	_, _, _, _, _, _, err := parseIslandPragma(line)
	if err == nil || !strings.Contains(err.Error(), line) {
		t.Fatalf("1-chart must be refused with the pragma line, got %v", err)
	}
}
