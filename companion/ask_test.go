package main

import "testing"

func TestAsked_ReadsWhatASecondCompanionWasStartedWith(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"started by hand":           {[]string{"/apps/agnostic-companion"}, ""},
		"asked to quit":             {[]string{"/apps/agnostic-companion", "--quit"}, askQuit},
		"asked to leave the login":  {[]string{"/apps/agnostic-companion", "--disable-start-at-login"}, askLoginOff},
		"asked to join the login":   {[]string{"/apps/agnostic-companion", "--enable-start-at-login"}, askLoginOn},
		"asked for start at login":  {[]string{"/apps/agnostic-companion", "--version", "--start-at-login-status"}, askLoginStatus},
		"asked for its version":     {[]string{"/apps/agnostic-companion", "--version"}, askVersion},
		"an argument it knows not":  {[]string{"/apps/agnostic-companion", "--nothing"}, ""},
		"a link the system appends": {[]string{"/apps/agnostic-companion", "--quit", "agnostic-ai://open"}, askQuit},
		"no arguments at all":       {nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := asked(tc.args); got != tc.want {
				t.Errorf("asked(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
