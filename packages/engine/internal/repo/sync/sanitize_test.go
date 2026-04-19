package sync

import "testing"

func TestSanitizeURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain message no url", "plain message no url"},
		{"clone: https://github.com/acme/wf.git not found", "clone: https://github.com/acme/wf.git not found"},
		{"clone: https://user:sekrit@github.com/acme/wf.git 401", "clone: https://user:***@github.com/acme/wf.git 401"},
		{"ssh: git@github.com:acme/wf.git ok", "ssh: git@github.com:acme/wf.git ok"},
		{"fetch https://u:p@a.com/r.git and https://x:y@b.com/r.git both failed",
			"fetch https://u:***@a.com/r.git and https://x:***@b.com/r.git both failed"},
	}
	for _, c := range cases {
		got := sanitizeURL(c.in)
		if got != c.want {
			t.Errorf("sanitizeURL(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}
