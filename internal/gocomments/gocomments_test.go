package gocomments

import "testing"

func TestCount(t *testing.T) {
	cases := map[string]struct {
		src  string
		want int
	}{
		"none":            {"package m\n\nfunc F() {}\n", 0},
		"line":            {"package m\n\n" + "/" + "/ note\nfunc F() {}\n", 1},
		"block":           {"package m\n\nfunc F() {" + "/" + "* inline *" + "/" + "}\n", 1},
		"several":         {"package m\n" + "/" + "/ a\n" + "/" + "/ b\nfunc F() {} " + "/" + "/ c\n", 3},
		"embed directive": {"package m\n\nimport \"embed\"\n\n" + "/" + "/go:embed dist\nvar files embed.FS\n", 0},
		"in a string":     {"package m\n\nvar u = \"https:" + "/" + "/example.test\"\n", 0},
		"in a raw string": {"package m\n\nvar u = `" + "/" + "/ not a comment`\n", 0},
		"in a rune":       {"package m\n\nvar r = '/'\n", 0},
		"broken syntax":   {"package m\n\nfunc F() { return " + "/" + "/ swallowed }\n", 1},
		"unterminated":    {"package m\n\n" + "/" + "* never closed\n", 1},
		"empty":           {"", 0},
	}
	for name, tc := range cases {
		if got := Count([]byte(tc.src)); got != tc.want {
			t.Errorf("%s: Count = %d, want %d", name, got, tc.want)
		}
	}
}

func TestAdded(t *testing.T) {
	before := []byte("package m\n\n" + "/" + "/ one\nfunc F() {}\n")
	more := []byte("package m\n\n" + "/" + "/ one\nfunc F() {} " + "/" + "/ two\n")
	fewer := []byte("package m\n\nfunc F() {}\n")
	if got := Added(before, more); got != 1 {
		t.Errorf("one comment added: %d", got)
	}
	if got := Added(before, fewer); got != 0 {
		t.Errorf("a comment removed must not count as negative: %d", got)
	}
	if got := Added(nil, before); got != 1 {
		t.Errorf("a new file with a comment: %d", got)
	}
}
