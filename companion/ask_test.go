package main

import "testing"

func TestAsked_ReadsWhatASecondCompanionWasStartedWith(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"started by hand":           {[]string{"/apps/agnostic-ai-companion"}, ""},
		"asked to quit":             {[]string{"/apps/agnostic-ai-companion", "--quit"}, askQuit},
		"asked to leave the login":  {[]string{"/apps/agnostic-ai-companion", "--disable-start-at-login"}, askLoginOff},
		"asked to join the login":   {[]string{"/apps/agnostic-ai-companion", "--enable-start-at-login"}, askLoginOn},
		"asked for its version":     {[]string{"/apps/agnostic-ai-companion", "--version"}, askVersion},
		"an argument it knows not":  {[]string{"/apps/agnostic-ai-companion", "--nothing"}, ""},
		"a link the system appends": {[]string{"/apps/agnostic-ai-companion", "--quit", "agnostic-ai://open"}, askQuit},
		"no arguments at all":       {nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := asked(tc.args); got != tc.want {
				t.Errorf("asked(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
