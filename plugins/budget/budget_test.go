package budget

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/testapp"
)

func run(t *testing.T, opts ...Option) (string, error) {
	t.Helper()
	var out bytes.Buffer
	_, err := generate.Run(generate.Options{
		Root:    fixture(t),
		Plugins: []generate.Plugin{New(append(opts, Output(&out))...)},
	})
	return out.String(), err
}

func TestReportsEveryEntry(t *testing.T) {
	out, err := run(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"boot ", "islands/user-card ", "KB gz"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "OVER") {
		t.Fatalf("no limits, nothing can be over:\n%s", out)
	}
}

func TestOverBudgetFailsGenerate(t *testing.T) {
	out, err := run(t, Limit("boot", 0.01), Limit("islands/user-card", 10000))
	if err == nil || !strings.HasPrefix(err.Error(), "plugin budget: over budget: boot is") {
		t.Fatalf("want attributed over-budget error, got %v", err)
	}
	if strings.Contains(err.Error(), "user-card") {
		t.Fatalf("an entry within budget must not be reported over: %v", err)
	}
	if !strings.Contains(out, "OVER") {
		t.Fatalf("report must flag the entry:\n%s", out)
	}
}

func TestGenerousLimitsPass(t *testing.T) {
	if _, err := run(t, Limit("boot", 10000)); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownEntryLimitIsAnError(t *testing.T) {
	_, err := run(t, Limit("islands/typo", 5))
	if err == nil || !strings.Contains(err.Error(), "unknown entry: islands/typo") {
		t.Fatalf("typo'd entry must fail: %v", err)
	}
}

func TestBadLimitFailsGenerate(t *testing.T) {
	for _, l := range []Option{Limit("boot", 0), Limit("boot", -1), Limit("", 5)} {
		_, err := run(t, l)
		if err == nil || !strings.Contains(err.Error(), "want an entry and a positive KB") {
			t.Errorf("want a bad-limit error, got %v", err)
		}
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	return root
}
