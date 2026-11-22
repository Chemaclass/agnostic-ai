### General

- A forgotten shell's command in the companion list masks a password passed as a plain argument, as `mysql -p<redacted>`: `-p` and `--password` for `mysql` and `mariadb`, `sshpass -p`, `curl -u user:password` and `--user`, and `PGPASSWORD=`, `MYSQL_PWD=` or `SSHPASS=` before the command. `-p` for other tools, such as `ssh -p 2222`, stays as written (#3918).
- Previews, chat texts and mailbox messages also count a literal `sshpass -p` password as a credential, so they withhold it as they do `mysql -psecret`. Both are found inside a quoted `sh -c` command and right after `;`, `&` or `|` too (#3918).
