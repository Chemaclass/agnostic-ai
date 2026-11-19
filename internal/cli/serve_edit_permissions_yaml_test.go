package cli

import "testing"

func TestSetPermissionList_KeepsTheListStyle(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			"block",
			"permissions:\n  allow:\n    - read(src/**)\n    - shell(go test:*) # tests\n  deny: [shell(git push)]\n",
			"permissions:\n  allow:\n    - read(src/**)\n    - 'shell(git status: *)' # tests\n  deny: [shell(git push)]\n",
		},
		{
			"indentless block",
			"permissions:\n  allow:\n  - read(src/**)\nmodel: sonnet\n",
			"permissions:\n  allow:\n  - read(src/**)\n  - 'shell(git status: *)'\nmodel: sonnet\n",
		},
		{
			"windows line endings",
			"permissions:\r\n  allow:\r\n    - read(src/**)\r\n",
			"permissions:\r\n  allow:\r\n    - read(src/**)\r\n    - 'shell(git status: *)'\r\n",
		},
		{
			"flow",
			"permissions:\n  allow: [read(src/**)]\n",
			"permissions:\n  allow: [\"read(src/**)\",\"shell(git status: *)\"]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := setPermissionList([]byte(tc.in), "permissions.allow", []string{"read(src/**)", "shell(git status: *)"})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}
