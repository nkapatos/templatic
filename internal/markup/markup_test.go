package markup

import "testing"

func TestRenderCellValue(t *testing.T) {
	var (
		nilPtr   *int
		nilIface any
		nilSlice []string
		nilMap   map[string]int
	)
	for _, c := range []struct {
		v    any
		want string
	}{
		{"raw", "raw"},
		{3, "3"},
		{nilPtr, ""},
		{nilIface, ""},
		{nilSlice, "[]"},
		{nilMap, "{}"},
		{[]int{1, 2}, "[1,2]"},
		{make(chan int), "unencodable"},
	} {
		got := RenderCellValue(c.v)
		if c.want == "unencodable" {
			if got == "" {
				t.Fatalf("unencodable value rendered empty")
			}
			continue
		}
		if got != c.want {
			t.Fatalf("RenderCellValue(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}
