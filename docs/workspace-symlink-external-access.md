# External workspace access through symbolic links

## Context

Audit finding `P-01` identified that tool path validation was lexical. A path such as `workspace/link/file` passed validation even when `link` resolved to a directory outside the workspace. Standard file operations then followed the link and accessed the external destination without the user's knowledge.

This change does not prohibit every external destination reached through a workspace symlink. It identifies the real destination and asks the user for one-time permission before reading or modifying it.

## Policy

The path policy distinguishes three cases:

1. A path lexically and physically inside the workspace proceeds normally.
2. A path lexically inside the workspace that resolves outside through a symlink requires explicit approval for each tool invocation.
3. A path explicitly outside the workspace, such as `/etc/passwd` or `../other-project/file`, remains rejected.

Approval applies to explicit reads and modifications performed by:

- `read`
- `grep`
- `write`
- `patch`, including search/replace, AST and unified diff modes

`glob` does not follow directory symlinks or read matched file contents, so listing a symlink does not itself require external-file approval. If a later tool dereferences the match, that tool enforces the policy.

Repository instruction discovery is intentionally stricter. An `AGENTS.md` or `.agents.md` symlink that resolves outside the workspace is ignored rather than injected automatically, because instruction discovery is an implicit side effect and not an explicit request to read that external file.

## Approval flow

For an external symlink destination, the tool sends a question through the existing `QuestionBroker`. The prompt displays:

- The path requested inside the workspace.
- The canonical external destination.
- Whether Motoko intends to read or modify it.

The available decisions are `Allow once` and `Deny`. Authorization succeeds only when the answer contains exactly one selection and that selection is `Allow once`.

The operation fails closed when:

- No question broker is available.
- The user selects `Deny`.
- The user cancels the question.
- The question times out.
- The answer is empty, duplicated or contradictory.

Approval is not cached. A later invocation must ask again.

## Path resolution

`internal/tools/pathpolicy` centralizes path handling for the affected tools. Resolution performs these steps:

1. Obtain and clean the current workspace path.
2. Reject lexical traversal outside the workspace.
3. Resolve the workspace and target through `filepath.EvalSymlinks`.
4. If the final target does not exist, find and resolve its nearest existing ancestor.
5. Classify the canonical destination as internal or external.
6. Capture the identity of the existing target and nearest existing ancestor.

Resolving the nearest existing ancestor is necessary for paths such as `external-link/new/directory/file.txt`, where the file and some child directories do not exist yet.

## Protected writes

Write restrictions are evaluated against both the requested path and canonical destination. A harmless alias cannot bypass blocks for:

- Git infrastructure under `.git`.
- Environment files whose base name starts with `.env`.
- SSH directories and common key or authorization file names.
- `.antigravitycli` agent configuration.

These paths remain blocked rather than becoming approvable external writes.

## TOCTOU protection

Canonicalization alone is insufficient because a symlink or directory can change while the approval popup is open.

On macOS and Linux, file access uses descriptor-relative traversal:

- Each path component is opened with `openat` and `O_NOFOLLOW`.
- Existing files are compared with the identity captured before approval.
- The nearest existing ancestor is also checked before creating missing descendants.
- New final files use exclusive creation, so a target introduced after approval is rejected.
- Existing writes truncate only after the verified descriptor has been opened.

The tool therefore operates on the object that was displayed for approval, or fails if that object or its ancestor changed. It does not dereference the original workspace symlink again after approval.

Other platforms use the same canonical classification, re-resolve the path before opening, compare captured identities and create new files exclusively. This fallback narrows the race window but cannot provide the same component-by-component guarantee as `openat` with `O_NOFOLLOW`.

## Tests

Regression coverage includes:

- Internal and external direct symlinks.
- New files below an external symlinked directory.
- Explicit paths outside the workspace.
- Canonical aliases into `.git`.
- Approved, denied and missing-broker reads.
- Approved external grep matches.
- Approved writes below external symlinked directories.
- External patch approval and mutation.
- Contradictory approval answers.
- Replacement of the workspace symlink while approval is pending.
- Replacement of the canonical final target while approval is pending.
- Replacement of the nearest existing parent before creating a file.
- External `AGENTS.md` instruction symlinks.

Validation performed on macOS ARM64 with Go 1.26:

```text
go test ./...
go test -race ./...
go vet ./...
go build -o /tmp/motoko-p01 ./cmd/motoko
GOOS=windows GOARCH=amd64 go test -c ./internal/tools/pathpolicy
git diff --check
```

All commands completed successfully.

## Residual risks and scope

- Descriptor-relative no-follow traversal is currently implemented for macOS and Linux. Other platforms use a fail-closed revalidation fallback with a smaller remaining TOCTOU window.
- Hard links are not symbolic links and do not reveal a unique external pathname from the file being opened. Hard-link policy is outside this finding and should be considered separately for protected files on filesystems where untrusted repositories can create them.
- Shell commands have their own filesystem access model and are not constrained by this tool path policy.
- The approval grants access only to the displayed filesystem destination; it does not establish trust in that file's contents.

## Acceptance criteria

- No affected tool silently follows a workspace symlink to an external destination.
- Explicit external paths remain rejected.
- Reads and modifications through external symlinks require one-time user approval.
- Rejection, cancellation, timeout and malformed answers fail closed.
- Protected write destinations cannot be hidden behind symlink aliases.
- Approved operations use a verified destination and reject relevant path replacement races.
- The complete normal and race-enabled test suites pass.
