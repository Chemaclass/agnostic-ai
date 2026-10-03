#!/bin/sh
# Blocks git push --force or -f and lets --force-with-lease through.
# It catches a mistake. It is not a sandbox.

# Reads tool_input.command from the hook JSON on stdin. An escaped newline
# becomes ";" and a unicode-escaped ampersand becomes "&", so command
# separators survive.
command=$(awk -v q='"' '
{ s = s $0 "\n" }
END {
  i = index(s, q "command" q)
  if (i == 0) exit
  s = substr(s, i + 9)
  if (!match(s, /^[ \t\r\n]*:[ \t\r\n]*"/)) exit
  s = substr(s, RLENGTH + 1)
  out = ""
  while (length(s) > 0) {
    c = substr(s, 1, 1)
    if (c == q) break
    if (c == "\\") {
      n = substr(s, 2, 1)
      if (n == "u") {
        out = out (substr(s, 3, 4) == "0026" ? "&" : " ")
        s = substr(s, 7)
        continue
      }
      if (n == "n" || n == "r") out = out ";"
      else if (n == "t") out = out " "
      else out = out n
      s = substr(s, 3)
      continue
    }
    out = out c
    s = substr(s, 2)
  }
  print out
}')

segments=$(printf '%s\n' "$command" | tr ';&|()' '\n\n\n\n\n' | tr -d \''"')

set -f
newline='
'
blocked=0
IFS=$newline
for segment in $segments; do
  IFS=' 	'
  git=0
  push=0
  for word in $segment; do
    case $word in
      git) git=1 ;;
      push) [ "$git" = 1 ] && push=1 ;;
      --force | -f) [ "$push" = 1 ] && blocked=1 ;;
      --*) ;;
      -*f*) [ "$push" = 1 ] && blocked=1 ;;
    esac
  done
  IFS=$newline
done

if [ "$blocked" = 1 ]; then
  echo "Blocked: git push --force rewrites the remote branch. Use --force-with-lease, or run the push yourself." >&2
  exit 2
fi
