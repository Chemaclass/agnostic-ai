package emit

import (
	"reflect"
	"testing"
)

func TestCommandArgv(t *testing.T) {
	cases := []struct {
		in   any
		want []string
	}{
		{"npm run dev", []string{"npm", "run", "dev"}},
		{`node "my server.js" --port=3000`, []string{"node", "my server.js", "--port=3000"}},
		{`echo 'a b' c\ d`, []string{"echo", "a b", "c d"}},
		{"npm run dev && open http://localhost", []string{"sh", "-c", "npm run dev && open http://localhost"}},
		{"PORT=4000 npm start", []string{"sh", "-c", "PORT=4000 npm start"}},
		{"echo $HOME", []string{"sh", "-c", "echo $HOME"}},
		{`echo "unterminated`, []string{"sh", "-c", `echo "unterminated`}},
		{[]any{"pnpm", "dev"}, []string{"pnpm", "dev"}},
		{"", nil},
		{[]any{}, nil},
	}
	for _, c := range cases {
		if got := CommandArgv(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("CommandArgv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
