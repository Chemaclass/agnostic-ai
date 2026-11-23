# Planted review controls

The common base is `6cfc29ab91ebb46c3c0b3b626171cc7c8ca53695`. Run each named test on the base, then apply its patch with `git apply --unidiff-zero` and run the test again. All five tests passed on the base and failed with the corresponding patch. Both reviewers found each intended bug.

## Case 21

Commit `32ee851f362f3ea38787cb65889c498e5b3193b5`

A candidate longer than the user input by exactly the allowed edit distance is dropped before distance scoring. Missing trailing characters at the configured edit-distance limit no longer receive a suggestion.

Reproduction

```sh
go test ./internal/suggest -count=1 -run ^TestName_AcceptsCandidatesAtDistanceLengthBoundary$
```

```diff
diff --git a/internal/suggest/suggest.go b/internal/suggest/suggest.go
index db52a9827..289dda97c 100644
--- a/internal/suggest/suggest.go
+++ b/internal/suggest/suggest.go
@@ -28 +28 @@ func Name(input string, names []string) string {
-		if difference < -limit || difference > limit {
+		if difference <= -limit || difference > limit {
```

## Case 22

Commit `05e1af8fe4209298d14436825306fd07663d4746`

A destination with one balanced inner pair of parentheses ends before that inner close. A relative link to docs/guide(v2).md is recorded as docs/guide(v2.

Reproduction

```sh
go test ./internal/mdlink -count=1 -run ^TestLocal_KeepsParenthesesBalancedInDestination$
```

```diff
diff --git a/internal/mdlink/mdlink.go b/internal/mdlink/mdlink.go
index f84a1f7e6..87fa67f1f 100644
--- a/internal/mdlink/mdlink.go
+++ b/internal/mdlink/mdlink.go
@@ -227 +227 @@ func inlineDestinations(line string) []destination {
-				if depth == 0 {
+				if depth <= 1 {
```

## Case 23

Commit `131874c9fda434aa3419f5a41ff84ce1bf068305`

The LSP resolves percent-encoded URI text as literal path bytes. Paths containing spaces, #, or % no longer round-trip and diagnostics map to the wrong path.

Reproduction

```sh
go test ./internal/lsp -count=1 -run ^TestURIConversion_RoundTripsEscapedFileNames$
```

```diff
diff --git a/internal/lsp/server.go b/internal/lsp/server.go
index 527792992..6fee372cb 100644
--- a/internal/lsp/server.go
+++ b/internal/lsp/server.go
@@ -272 +272 @@ func uriToPath(uri string) string {
-	p := u.Path
+	p := u.EscapedPath()
```

## Case 24

Commit `a5f8d42ff6919984367675192083f8027b3a9ef9`

A valid project GitHub remote ending in a slash leaves that trailing slash in the repository field, fails the owner/repo shape check, and has no link in the project overview.

Reproduction

```sh
go test ./internal/overview -count=1 -run ^TestGitHubRepository_NormalizesRemoteForms$
```

```diff
diff --git a/internal/overview/repository.go b/internal/overview/repository.go
index 76e020c9b..2b2ab6058 100644
--- a/internal/overview/repository.go
+++ b/internal/overview/repository.go
@@ -45 +45 @@ func GitHubRepository(remote string) *Repository {
-	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
+	path = strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".git")
```

## Case 25

Commit `00a1fd4de677f8d57b60b12a5f8acb92d12b610a`

A user cannot set conversations.stuckShellAfter to its documented explicit zero value. A valid local opt-out now returns a decode error instead of overriding the shared threshold.

Reproduction

```sh
go test ./internal/cli -count=1 -run ^TestLoadUserConversations_UsesTheSharedSettingUntilTheLocalFileOverridesIt$
```

```diff
diff --git a/internal/config/conversations.go b/internal/config/conversations.go
index e75ba6f67..dd709c682 100644
--- a/internal/config/conversations.go
+++ b/internal/config/conversations.go
@@ -33 +33 @@ func (c ConversationsConfig) ShellThreshold(source string) (*time.Duration, erro
-	if err != nil || duration < 0 {
+	if err != nil || duration <= 0 {
```
