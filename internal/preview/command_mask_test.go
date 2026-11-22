package preview

import "testing"

func TestMaskCommandPasswords_MasksKnownPasswordArgumentsOnly(t *testing.T) {
	r := Redacted
	cases := []struct{ name, line, shown, rest string }{
		{"glued -p", "mysql -uroot -phunter2pass app", "mysql -uroot -p" + r + " app", "mysql -uroot app"},
		{"mysqldump path", "/usr/bin/mysqldump -p hunter2pass app", "/usr/bin/mysqldump -p " + r + " app", "/usr/bin/mysqldump app"},
		{"quoted with a space", `mysql --password="hunter2 pass" app`, `mysql --password="` + r + `" app`, "mysql app"},
		{"unclosed quote", `mysql --password='hunter2 pass`, `mysql --password='` + r, "mysql"},
		{"after a separator", "cd /srv && mysql -phunter2pass; ls -p x", "cd /srv && mysql -p" + r + "; ls -p x", "cd /srv && mysql ; ls -p x"},
		{"behind sudo", "sudo PGPASSWORD=hunter2pass psql", "sudo PGPASSWORD=" + r + " psql", "sudo psql"},
		{"sshpass glued", "sshpass -phunter2pass ssh host", "sshpass -p" + r + " ssh host", "sshpass ssh host"},
		{"sshpass from a file", "sshpass -f pass.txt ssh -p 22 host", "sshpass -f pass.txt ssh -p 22 host", "sshpass -f pass.txt ssh -p 22 host"},
		{"curl grouped switches", "curl -fsSu alice:hunter2pass https://x.example.com", "curl -fsSu alice:" + r + " https://x.example.com", "curl https://x.example.com"},
		{"curl quoted user", `curl -u 'alice:hunter2 pass' https://x.example.com`, `curl -u 'alice:` + r + `' https://x.example.com`, "curl https://x.example.com"},
		{"curl proxy user", "curl --proxy-user bob:hunter2pass https://x.example.com", "curl --proxy-user bob:" + r + " https://x.example.com", "curl https://x.example.com"},
		{"variable password", `PGPASSWORD="$PW" psql; curl -u "alice:$PW" x`, `PGPASSWORD="$PW" psql; curl -u "alice:$PW" x`, `PGPASSWORD="$PW" psql; curl -u "alice:$PW" x`},
		{"port flags", "ssh -p 2222 host && docker run -p 8080:80 nginx", "ssh -p 2222 host && docker run -p 8080:80 nginx", "ssh -p 2222 host && docker run -p 8080:80 nginx"},
		{"password variable named as an argument", "echo PGPASSWORD=x", "echo PGPASSWORD=x", "echo PGPASSWORD=x"},
		{"bare -p before an option", "mysql -p -u root", "mysql -p -u root", "mysql -p -u root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shown, rest := MaskCommandPasswords(tc.line)
			if shown != tc.shown || rest != tc.rest {
				t.Errorf("MaskCommandPasswords(%q) = %q, %q; want %q, %q", tc.line, shown, rest, tc.shown, tc.rest)
			}
		})
	}
}

func TestPassesPasswordArg_ReadsSSHPassAsWellAsMySQL(t *testing.T) {
	for _, command := range []string{"sshpass -p hunter2pass ssh host", "sshpass -phunter2pass ssh host", "sshpass -e -p hunter2pass ssh host"} {
		if !CommandCredentialDetected(command) || !Display("deploy.sh", command+"\n").Withheld || !ProseSensitive("Run `"+command+"`.") {
			t.Errorf("%q is not found as a password", command)
		}
	}
	for _, command := range []string{"sshpass -f pass.txt ssh -p 2222 host", "sshpass -p $SSH_PASS ssh host", "sshpass -e ssh host", "ssh -p 2222 host"} {
		if CommandCredentialDetected(command) || ProseSensitive("Run `"+command+"`.") {
			t.Errorf("%q is found as a password", command)
		}
	}
}
