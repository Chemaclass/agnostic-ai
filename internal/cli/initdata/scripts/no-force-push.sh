#!/bin/sh
# Blocks git push --force or -f and lets --force-with-lease through.
# It catches a mistake. It is not a sandbox.

# Reads tool_input.command from the hook JSON on stdin, splits it into
# words the way sh would (quotes, backslashes, line continuations), and
# checks each command between unquoted ; & | ( ) and newlines.
awk -v q='"' -v sq="'" '
function flush() {
  if (inword) words[++n] = w
  w = ""
  inword = 0
}

function check(   i, j, k, a) {
  i = 1
  while (i <= n && words[i] ~ /^[A-Za-z_][A-Za-z0-9_]*=/) i++
  if (i > n || words[i] !~ /(^|\/)git$/) return
  for (i++; i <= n && words[i] ~ /^-/; i++)
    if (words[i] ~ /^(-C|-c|--git-dir|--work-tree|--namespace)$/) i++
  if (i > n || words[i] != "push") return
  for (j = i + 1; j <= n; j++) {
    a = words[j]
    if (a == "--") return
    if (a == "--force") { blocked = 1; return }
    if (a ~ /^--(repo|receive-pack|exec|push-option)$/) { j++; continue }
    if (a ~ /^--/ || a !~ /^-./) continue
    for (k = 2; k <= length(a); k++) {
      if (substr(a, k, 1) == "f") { blocked = 1; return }
      if (substr(a, k, 1) == "o") { if (k == length(a)) j++; break }
    }
  }
}

function end_command() {
  flush()
  check()
  n = 0
}

{ json = json $0 "\n" }

END {
  i = index(json, q "command" q)
  if (i == 0) exit 0
  s = substr(json, i + 9)
  if (!match(s, /^[ \t\r\n]*:[ \t\r\n]*/)) exit 0
  s = substr(s, RLENGTH + 1)
  if (substr(s, 1, 1) != q) exit 0
  s = substr(s, 2)

  cmd = ""
  while (length(s) > 0) {
    c = substr(s, 1, 1)
    if (c == q) break
    if (c != "\\") { cmd = cmd c; s = substr(s, 2); continue }
    e = substr(s, 2, 1)
    if (e == "u") {
      code = tolower(substr(s, 3, 4))
      if (code == "0026") cmd = cmd "&"
      else if (code == "003c") cmd = cmd "<"
      else if (code == "003e") cmd = cmd ">"
      else if (code == "0027") cmd = cmd sq
      else cmd = cmd " "
      s = substr(s, 7)
      continue
    }
    if (e == "n") cmd = cmd "\n"
    else if (e == "t") cmd = cmd "\t"
    else if (e == "r" || e == "b" || e == "f") cmd = cmd " "
    else cmd = cmd e
    s = substr(s, 3)
  }

  n = 0
  w = ""
  inword = 0
  mode = ""
  size = length(cmd)
  for (p = 1; p <= size && !blocked; p++) {
    c = substr(cmd, p, 1)
    if (mode == "single") {
      if (c == sq) mode = ""
      else w = w c
      continue
    }
    if (c == "\\") {
      p++
      e = substr(cmd, p, 1)
      if (e == "\n") continue
      if (mode == "double" && e != q && e != "\\" && e != "$" && e != "`") w = w c
      w = w e
      inword = 1
      continue
    }
    if (mode == "double") {
      if (c == q) mode = ""
      else w = w c
      continue
    }
    if (c == q) { mode = "double"; inword = 1; continue }
    if (c == sq) { mode = "single"; inword = 1; continue }
    if (c == " " || c == "\t" || c == "\r") { flush(); continue }
    if (index(";&|()\n", c)) { end_command(); continue }
    w = w c
    inword = 1
  }
  if (!blocked) end_command()
  exit (blocked ? 2 : 0)
}'
status=$?

if [ "$status" = 2 ]; then
  echo "Blocked: git push --force rewrites the remote branch. Use --force-with-lease, or run the push yourself." >&2
fi
exit "$status"
