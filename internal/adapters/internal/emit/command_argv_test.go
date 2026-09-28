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
		{"npm run build\nnpm run start", []string{"sh", "-c", "npm run build\nnpm run start"}},
		{"npm run \\\nbuild", []string{"sh", "-c", "npm run \\\nbuild"}},
		{"echo 'a\nb'", []string{"echo", "a\nb"}},
		{`node "C:\tools\x.js"`, []string{"node", `C:\tools\x.js`}},
		{`grep "a\.b" f`, []string{"grep", `a\.b`, "f"}},
		{`echo "say \"hi\""`, []string{"echo", `say "hi"`}},
		{"cd apps/web", []string{"sh", "-c", "cd apps/web"}},
		{"exec npm run dev", []string{"sh", "-c", "exec npm run dev"}},
		{"! false", []string{"sh", "-c", "! false"}},
		{"npm run build\r\nnpm start", []string{"sh", "-c", "npm run build\r\nnpm start"}},
		{[]any{"pnpm", "dev"}, []string{"pnpm", "dev"}},
		{[]any{"npm", 7, "run", true}, []string{"npm", "7", "run", "true"}},
		{[]any{"npm", []any{"x"}}, nil},
		{[]any{"npm", nil}, nil},
		{"", nil},
		{[]any{}, nil},
	}
	for _, c := range cases {
		if got := CommandArgv(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("CommandArgv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
