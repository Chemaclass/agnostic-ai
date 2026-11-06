package preview

import (
	"strings"
	"testing"
)

// Every panel that shows file text reads it through Display, so a
// labeled credential in any of these layouts is hidden or withheld
// there, and Sensitive reports it (#3062).
func TestDisplay_LabeledCredentialLayoutsAreHiddenOrWithheld(t *testing.T) {
	const secret = "hunter2short"
	for name, text := range map[string]string{
		"table":                   "| field | value |\n| --- | --- |\n| password | hunter2short |\n",
		"table without pipes":     "field | value\n--- | ---\ntoken | hunter2short\n",
		"table bold label":        "| **token** | hunter2short |\n",
		"table code label":        "| `auth` | hunter2short |\n",
		"table emphasis label":    "| _cookie_ | hunter2short |\n",
		"table session id":        "| Session ID | hunter2short |\n",
		"xml element":             "<password>hunter2short</password>\n",
		"xml session element":     "<session>hunter2short</session>\n",
		"self-closing tag":        "<token value=\"hunter2short\"/>\n",
		"tag attribute":           "<config token=\"hunter2short\" />\n",
		"yaml alias":              "defaults: &login hunter2short\npassword: *login\n",
		"yaml flow map":           "Use {password: hunter2short} here.\n",
		"block scalar":            "password: |\n  hunter2short\n",
		"folded block scalar":     "Notes.\n\nclient_secret: >\n  hunter2short\n",
		"pem private key":         "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAhunter2short\n-----END OPENSSH PRIVATE KEY-----\n",
		"cut pem":                 "-----BEGIN RSA PRIVATE KEY-----\nMIIEhunter2short",
		"session label":           "session: hunter2short\n",
		"access key label":        "access key = hunter2short\n",
		"aws access key variable": "AWS_SECRET_ACCESS_KEY=hunter2short\n",
		"labeled prose":           "The password is hunter2short.\n",
		"field-only label":        "**Password:** hunter2short\n",
		"known token shape":       "Use ghp_123456789012345678901234567890123456 here.\n",
		"mysql attached password": "Run `mysql -u root -phunter2short app`.\n",
		"mysql in a shell fence":  "```sh\nmysqldump -h db -phunter2short app > dump.sql\n```\n",
	} {
		for _, path := range []string{"notes.md", "notes.txt"} {
			shown := Display(path, text)
			if !shown.Hidden && !shown.Withheld || strings.Contains(shown.Text, secret) || strings.Contains(shown.Text, "ghp_") {
				t.Errorf("%s in %s: hidden %v, withheld %v, text %q", name, path, shown.Hidden, shown.Withheld, shown.Text)
			}
			if !Sensitive(path, text) {
				t.Errorf("%s in %s: Sensitive is false", name, path)
			}
		}
	}
}

func TestDisplay_GuidanceThatOnlyNamesCredentialsStillShows(t *testing.T) {
	for _, text := range []string{
		"Never commit secrets.\n",
		"Keep the token budget small.\n",
		"Rotate API keys every 90 days and store passwords in a manager.\n",
		"Read credentials from the environment, never from a file.\n",
		"# Security\n\n- Never commit secrets or private keys.\n- Count tokens before you send a prompt.\n",
		"MCP OAuth objects beyond `oauth:false` stay out.\n",
		"Record `{\"input_tokens\":null,\"output_tokens\":12}` per run.\n",
		"Set `token: ${API_TOKEN}` in the server entry.\n",
		"Run `mysql -u root -p app`; it prompts for the password.\n",
		"Run `mysql -u root -p$MYSQL_PASSWORD app`.\n",
		"Run `mkdir -p build` and `ssh -p2222 host`.\n",
		// A path segment of letters only cannot be told from an ordinary
		// word without a dictionary, so URL paths like this stay visible.
		"See https://docs.example.com/guides/configuration/ for details.\n",
	} {
		for _, path := range []string{"notes.md", "notes.txt"} {
			shown := Display(path, text)
			if shown.Hidden || shown.Withheld || shown.Text != text || Sensitive(path, text) {
				t.Errorf("%q in %s must show unchanged: %+v", text, path, shown)
			}
		}
	}
}

// Display keeps redacting structured text key by key, while Sensitive
// also reads its labels, as the file content panel needs.
func TestSensitive_ReadsLabelsInStructuredText(t *testing.T) {
	front := "---\nname: deploy\nsession: hunter2short\n---\nBody.\n"
	if shown := Display("deploy.md", front); shown.Hidden || shown.Withheld {
		t.Errorf("Display changed frontmatter it shows today: %+v", shown)
	}
	if !Sensitive("deploy.md", front) {
		t.Error("Sensitive misses a session label in frontmatter")
	}
	reference := "env:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\n"
	if shown := Display("mcp.yaml", reference); shown.Hidden || shown.Withheld || shown.Text != reference {
		t.Errorf("a reference must still preview: %+v", shown)
	}
}

// A script that passes a MySQL password glued to -p is hidden too.
func TestDisplay_HidesAMySQLPasswordGluedToItsFlag(t *testing.T) {
	if shown := Display("db.sh", "#!/bin/sh\nmysql -u root -phunter2short app\n"); !shown.Withheld || strings.Contains(shown.Text, "hunter2short") {
		t.Errorf("script = %+v", shown)
	}
	if !CommandCredentialDetected("mariadb -phunter2short") || CommandCredentialDetected("mysql -p app") || CommandCredentialDetected("mysql -p${DB_PASSWORD} app") {
		t.Error("only a literal value glued to -p counts")
	}
}
