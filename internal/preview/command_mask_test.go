package preview

import (
	"strings"
	"testing"
)

func TestMaskCommandPasswords_MasksKnownPasswordArgumentsOnly(t *testing.T) {
	r := Redacted
	cases := []struct{ name, line, shown, rest string }{
		{"glued -p", "mysql -uroot -phunter2pass app", "mysql -uroot -p" + r + " app", "mysql -uroot app"},
		{"mysqldump path", "/usr/bin/mysqldump -p hunter2pass app", "/usr/bin/mysqldump -p " + r + " app", "/usr/bin/mysqldump hunter2pass app"},
		{"sshpass password starting with a dash", "sshpass -p -hunter2pass ssh host", "sshpass -p " + r + " ssh host", "sshpass ssh host"},
		{"command substitution", `sshpass -p "$(cat pw.txt)" ssh host`, `sshpass -p "$(cat pw.txt)" ssh host`, `sshpass -p "$(cat pw.txt)" ssh host`},
		{"quoted with a space", `mysql --password="hunter2 pass" app`, `mysql --password="` + r + `" app`, "mysql app"},
		{"unclosed quote", `mysql --password='hunter2 pass`, `mysql --password='` + r, "mysql"},
		{"after a separator", "cd /srv && mysql -phunter2pass; ls -p x", "cd /srv && mysql -p" + r + "; ls -p x", "cd /srv && mysql ; ls -p x"},
		{"behind sudo", "sudo PGPASSWORD=hunter2pass psql", "sudo PGPASSWORD=" + r + " psql", "sudo psql"},
		{"sshpass in a subshell", "(sshpass -p hunter2pass ssh host)", "(sshpass -p " + r + " ssh host)", "(sshpass ssh host)"},
		{"sshpass.exe", "sshpass.exe -p hunter2pass ssh host", "sshpass.exe -p " + r + " ssh host", "sshpass.exe ssh host"},
		{"sshpass glued", "sshpass -phunter2pass ssh host", "sshpass -p" + r + " ssh host", "sshpass ssh host"},
		{"sshpass from a file", "sshpass -f pass.txt ssh -p 22 host", "sshpass -f pass.txt ssh -p 22 host", "sshpass -f pass.txt ssh -p 22 host"},
		{"curl grouped switches", "curl -fsSu alice:hunter2pass https://x.example.com", "curl -fsSu alice:" + r + " https://x.example.com", "curl https://x.example.com"},
		{"curl quoted user", `curl -u 'alice:hunter2 pass' https://x.example.com`, `curl -u 'alice:` + r + `' https://x.example.com`, "curl https://x.example.com"},
		{"curl proxy user", "curl --proxy-user bob:hunter2pass https://x.example.com", "curl --proxy-user bob:" + r + " https://x.example.com", "curl https://x.example.com"},
		{"variable password", `PGPASSWORD="$PW" psql; curl -u "alice:$PW" x`, `PGPASSWORD="$PW" psql; curl -u "alice:$PW" x`, `PGPASSWORD="$PW" psql; curl -u "alice:$PW" x`},
		{"port flags", "ssh -p 2222 host && docker run -p 8080:80 nginx", "ssh -p 2222 host && docker run -p 8080:80 nginx", "ssh -p 2222 host && docker run -p 8080:80 nginx"},
		{"bare -p before an option", "mysql -p -u root", "mysql -p -u root", "mysql -p -u root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shown, rest, ok := MaskCommandPasswords(tc.line)
			if shown != tc.shown || rest != tc.rest || !ok {
				t.Errorf("MaskCommandPasswords(%q) = %q, %q, %v; want %q, %q, true", tc.line, shown, rest, ok, tc.shown, tc.rest)
			}
		})
	}
}

func TestMaskCommandPasswords_WithholdsAPasswordItCannotMask(t *testing.T) {
	for _, line := range []string{
		`mysql --password=hunter2\ tail9word app`,
		"mysql -p'hunter2'tail9word app",
		"mysql -p$'hunter2 tail9word' app",
		`sshpass -p hunter2\ pass ssh host`,
		`sshpass -p 'it'\''s' ssh host`,
		"timeout 30 sshpass -p hunter2pass ssh host",
		"timeout 30 mysql -p hunter2pass app",
		"sshpass -e ssh host mysql -u root -phunter2pass app",
		"sshpass -phunter2pass ssh host mysql -phunter3pass",
		"bash -c 'sshpass -p hunter2pass ssh-copy-id host'",
		"cd /srv;sshpass -p hunter2pass ssh host",
		"docker run -e MYSQL_PWD=hunter2pass mysql",
		"echo PGPASSWORD=hunter2pass",
		`mysql -u root -pold9secret -e "ALTER USER 'app'@'%' IDENTIFIED BY 'newsecret9x'"`,
		`mysql --password=old9secret -e "create user app identified by 'newsecret9x'"`,
		`curl -u alice:hunter2pass -d '{"password":"hunter2pass"}' https://x.example.com`,
		"curl --user alice:hunter2pass --data-raw x https://x.example.com",
		"echo $(sshpass -p hunter2pass ssh host)",
	} {
		if shown, _, ok := MaskCommandPasswords(line); ok {
			t.Errorf("MaskCommandPasswords(%q) = %q, want it withheld", line, shown)
		}
	}
	for _, line := range []string{"mysqldump -u root -p > dump.sql", "mysql -u root -p < schema.sql", "mysql -p 2>&1", "sshpass -p hunter2pass ssh -p 2222 host"} {
		if shown, _, ok := MaskCommandPasswords(line); !ok || strings.Contains(shown, "<redacted> ") && !strings.HasPrefix(line, "sshpass") {
			t.Errorf("MaskCommandPasswords(%q) = %q, %v; want a redirection kept", line, shown, ok)
		}
	}
}

func TestPassesPasswordArg_ReadsSSHPassAsWellAsMySQL(t *testing.T) {
	for _, command := range []string{"sshpass -p hunter2pass ssh host", "sshpass -phunter2pass ssh host", "sshpass -e -p hunter2pass ssh host"} {
		if !CommandCredentialDetected(command) || !Display("deploy.sh", command+"\n").Withheld || !ProseSensitive("Run `"+command+"`.") {
			t.Errorf("%q is not found as a password", command)
		}
	}
	for _, command := range []string{"sshpass -f pass.txt ssh -p 2222 host", "sshpass -p $SSH_PASS ssh host", `sshpass -p "$(cat pw.txt)" ssh host`, "sshpass -e ssh host", "ssh -p 2222 host"} {
		if CommandCredentialDetected(command) || ProseSensitive("Run `"+command+"`.") {
			t.Errorf("%q is found as a password", command)
		}
	}
	for _, prose := range []string{"Avoid sshpass -p in scripts, it shows in ps.", "We dropped sshpass -p for keys last year."} {
		if ProseSensitive(prose) {
			t.Errorf("%q is found as a password", prose)
		}
	}
	for _, prose := range []string{"Run sshpass -p hunter2pass ssh host.", "Try mysql.exe -u root -phunter2pass app", `bash -c "mysql -u root -phunter2pass app"`, "cd /srv&&mysql -phunter2pass", "sshpass -e ssh host mysql -u root -phunter2pass app", "sshpass -phunter2pass ssh host mysql -phunter3pass"} {
		if !ProseSensitive(prose) {
			t.Errorf("%q is not found as a password", prose)
		}
	}
}
