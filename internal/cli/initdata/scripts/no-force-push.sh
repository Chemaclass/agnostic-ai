#!/bin/sh
# Blocks git push --force, -f, or a +refspec, and lets --force-with-lease
# through. It catches a mistake. It is not a sandbox.

# Reads tool_input.command from the hook JSON on stdin and splits it into
# words the way sh would: quotes, backslashes, line continuations,
# comments, redirections, heredoc bodies, and $( ) or backtick
# substitutions. It checks each command between unquoted ; & | ( ) and
# newlines, past reserved words such as if and then. A heredoc it cannot
# read ends the check, since a missed push beats blocking text. A coarse
# pass over the unquoted text then blocks a force push the parse missed.
awk -v q='"' -v sq="'" '
function flush() {
  if (inword && redirect) redirect = 0
  else if (inword) {
    words[++n] = w
    quoted[n] = wq
  }
  w = ""
  inword = 0
  wq = 0
}

# Drops a file descriptor number written right before a redirection.
function redirect_from() {
  if (inword && !wq && w ~ /^[0-9]+$/) {
    w = ""
    inword = 0
  }
  flush()
  redirect = 1
}

# Returns the index of the word that names the program, past reserved
# words, assignments, and wrappers such as env and sudo.
function program(   i, a, wrapper) {
  for (i = 1; i <= n; i++) {
    a = words[i]
    if (!quoted[i] && a ~ /^(if|then|else|elif|do|while|until|!|time|\{)$/) continue
    if (a ~ /^[A-Za-z_][A-Za-z0-9_]*=/) continue
    if (a ~ /^(command|exec|env|nohup|nice|sudo)$/) { wrapper = a; continue }
    if (wrapper != "" && a ~ /^-/) {
      if ((wrapper == "env" && a ~ /^(-[uCS]|--(unset|chdir|split-string))$/) || (wrapper == "exec" && a == "-a") || (wrapper == "nice" && a ~ /^(-n|--adjustment)$/) || (wrapper == "sudo" && a ~ /^(-[ugCDpUrtTR]|--(user|group|close-from|chdir|prompt|other-user|role|type|command-timeout|host))$/)) i++
      continue
    }
    if (i > 1 && words[i - 1] == "time" && a == "-p") continue
    return i
  }
  return n + 1
}

function check(   i, j, k, a, plus, lease, positional) {
  i = program()
  if (i > n || words[i] !~ /(^|\/)git$/) return
  for (i++; i <= n && words[i] ~ /^-/; i++)
    if (words[i] ~ /^(-C|-c|--git-dir|--work-tree|--namespace)$/) i++
  if (i > n || words[i] != "push") return
  for (j = i + 1; j <= n; j++) {
    a = words[j]
    if (positional || a !~ /^-./) {
      if (a ~ /^\+/) plus = 1
      continue
    }
    if (a == "--") { positional = 1; continue }
    if (a == "--force") { blocked = 1; return }
    if (a ~ /^--force-with-lease(=|$)/) { lease = 1; continue }
    if (a ~ /^--(repo|receive-pack|exec|push-option)$/) { j++; continue }
    if (a ~ /^--/) continue
    for (k = 2; k <= length(a); k++) {
      if (substr(a, k, 1) == "f") { blocked = 1; return }
      if (substr(a, k, 1) == "o") { if (k == length(a)) j++; break }
    }
  }
  if (plus && !lease) blocked = 1
}

function end_command() {
  flush()
  check()
  n = 0
  redirect = 0
}

# Starts a subshell or substitution. A substitution keeps the words of the
# command around it, which continue once it closes.
function open_nested(kind, back,   i) {
  depth++
  kinds[depth] = kind
  backs[depth] = back
  if (kind == "(") { end_command(); return }
  saved_n[depth] = n
  for (i = 1; i <= n; i++) {
    saved_words[depth, i] = words[i]
    saved_quoted[depth, i] = quoted[i]
  }
  saved_w[depth] = w
  saved_wq[depth] = wq
  saved_redirect[depth] = redirect
  n = 0
  w = ""
  inword = 0
  wq = 0
  redirect = 0
}

function close_nested(   i) {
  end_command()
  mode = backs[depth]
  if (kinds[depth] != "(") {
    n = saved_n[depth]
    for (i = 1; i <= n; i++) {
      words[i] = saved_words[depth, i]
      quoted[i] = saved_quoted[depth, i]
    }
    w = saved_w[depth] "$()"
    inword = 1
    wq = saved_wq[depth]
    redirect = saved_redirect[depth]
  }
  depth--
}

# Queues the delimiter of the heredoc whose word starts at p and returns
# the position of its last character, or 0 for a form it cannot read.
function heredoc_word(p,   strip, word, c, closing) {
  strip = 0
  if (substr(cmd, p, 1) == "-") { strip = 1; p++ }
  while (substr(cmd, p, 1) == " " || substr(cmd, p, 1) == "\t") p++
  word = ""
  for (; p <= size; p++) {
    c = substr(cmd, p, 1)
    if (c == sq || c == q) {
      closing = index(substr(cmd, p + 1), c)
      if (!closing) return 0
      word = word substr(cmd, p + 1, closing - 1)
      p += closing
      continue
    }
    if (c == "\\") { p++; word = word substr(cmd, p, 1); continue }
    if (index(" \t\n;&|()<>", c)) break
    word = word c
  }
  if (word == "") return 0
  heredocs[++pending] = word
  strips[pending] = strip
  return p - 1
}

# Skips the queued heredoc bodies that start after the newline at p and
# returns the position of the newline that ends the last terminator.
function skip_bodies(p,   h, rest, end, line) {
  for (h = 1; h <= pending; h++) {
    do {
      if (p >= size) { pending = 0; return size }
      rest = substr(cmd, p + 1)
      end = index(rest, "\n")
      line = end ? substr(rest, 1, end - 1) : rest
      p = end ? p + end : size
      if (strips[h]) sub(/^\t+/, "", line)
    } while (line != heredocs[h])
  }
  pending = 0
  return p
}

# Backs up the parse with a coarse look at the unquoted text, so a wrapper
# or syntax the parse does not follow still blocks: within a span between
# separators, git, then push as its subcommand, then a force word.
function coarse(   spans, count, s, nw, ws, i, j, a, force, lease) {
  count = split(raw, spans, /[;&|\n]/)
  for (s = 1; s <= count; s++) {
    nw = split(spans[s], ws, /[ \t\r]+/)
    for (i = 1; i <= nw; i++) {
      if (ws[i] !~ /(^|\/)git$/) continue
      for (j = i + 1; j <= nw && ws[j] ~ /^-/; j++)
        if (ws[j] ~ /^(-C|-c|--git-dir|--work-tree|--namespace)$/) j++
      if (ws[j] != "push") continue
      force = 0
      lease = 0
      for (j++; j <= nw; j++) {
        a = ws[j]
        if (a ~ /^--force-(with-lease|if-includes)(=|$)/) lease = 1
        else if (a == "--force" || a ~ /^-[A-Za-z]*f[A-Za-z]*$/ || a ~ /^\+/) force = 1
      }
      if (force && !lease) return 1
    }
  }
  return 0
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
  raw = ""
  size = length(cmd)
  for (p = 1; p <= size && !blocked; p++) {
    c = substr(cmd, p, 1)
    e = substr(cmd, p + 1, 1)
    if (mode == "single") {
      if (c == sq) mode = ""
      else w = w c
      continue
    }
    if (c == "\\") {
      p++
      if (e == "\n") continue
      if (mode == "double" && e != q && e != "\\" && e != "$" && e != "`") w = w c
      w = w e
      wq = 1
      inword = 1
      if (mode == "") raw = raw " "
      continue
    }
    if (mode == "double") {
      if (c == q) mode = ""
      else if (c == "$" && e == "(") { p++; open_nested("$(", "double"); mode = "" }
      else if (c == "`") { open_nested("`", "double"); mode = "" }
      else w = w c
      continue
    }
    raw = raw " "
    if (c == q) { mode = "double"; inword = 1; wq = 1; continue }
    if (c == sq) { mode = "single"; inword = 1; wq = 1; continue }
    if (c == "#" && !inword) {
      while (p < size && substr(cmd, p + 1, 1) != "\n") p++
      continue
    }
    if (c == "$" && e == "(" && substr(cmd, p + 2, 1) != "(") { p++; open_nested("$(", ""); continue }
    if ((c == "<" || c == ">") && e == "(") { p++; open_nested("$(", ""); continue }
    if (c == "`") {
      if (depth && kinds[depth] == "`") close_nested()
      else open_nested("`", "")
      continue
    }
    if (c == "(") { open_nested("(", ""); continue }
    if (c == ")") {
      if (depth) close_nested()
      else end_command()
      continue
    }
    if (c == "<" && substr(cmd, p, 3) == "<<<") { redirect_from(); p += 2; continue }
    if (c == "<" && e == "<") {
      redirect_from()
      redirect = 0
      p = heredoc_word(p + 2)
      if (!p) { stopped = 1; break }
      continue
    }
    if (c == "&" && e == ">") {
      flush()
      redirect = 1
      p += (substr(cmd, p + 2, 1) == ">") ? 2 : 1
      continue
    }
    if (c == ">" || c == "<") {
      redirect_from()
      if ((c == ">" && (e == ">" || e == "&" || e == "|")) || (c == "<" && (e == "&" || e == ">"))) p++
      continue
    }
    if (c == " " || c == "\t" || c == "\r") { flush(); continue }
    if (index(";&|\n", c)) {
      raw = raw c
      end_command()
      if (c == "\n" && pending) p = skip_bodies(p)
      continue
    }
    raw = substr(raw, 1, length(raw) - 1) c
    w = w c
    inword = 1
  }
  if (!blocked && !stopped) end_command()
  if (!blocked && coarse()) blocked = 1
  exit (blocked ? 3 : 0)
}'
# awk exits 2 on its own errors, so a block comes back as 3.
case $? in
  0) exit 0 ;;
  3)
    echo "Blocked: git push --force rewrites the remote branch. Use --force-with-lease, or run the push yourself." >&2
    exit 2
    ;;
  *) exit 1 ;;
esac
