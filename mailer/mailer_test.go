package mailer

import "testing"

func Test_Recipients(t *testing.T) {
	cases := map[string]string{
		"a@x.com":             "a@x.com",
		"a@x.com,b@y.com":     "a@x.com, b@y.com",
		" a@x.com , b@y.com ": "a@x.com, b@y.com",
		"a@x.com,":            "a@x.com",
		",,":                  "",
		"":                    "",
	}
	for in, want := range cases {
		if got := recipients(in); got != want {
			t.Errorf("recipients(%q) = %q, want %q", in, got, want)
		}
	}
}
