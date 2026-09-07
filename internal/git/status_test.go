package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// statusOf runs the same git command the collector runs.
func statusOf(t *testing.T, dir string) []byte {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain=v2", "--branch", "--untracked-files=normal", "-z")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status in %s: %v", dir, err)
	}
	return out
}

// parseRepo is the whole pipeline under test: real git, real output, one Repo.
func parseRepo(t *testing.T, dir string) Repo {
	t.Helper()
	var r Repo
	if err := ParseStatus(statusOf(t, dir), &r); err != nil {
		t.Fatalf("ParseStatus() error = %v", err)
	}
	return r
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tryRun runs git and tolerates a non-zero exit, which testutil.Run does not.
// Only one fixture needs it: provoking a merge conflict means running a merge
// that is meant to fail.
func tryRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	_ = cmd.Run()
}

func TestParseStatusCleanRepo(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	r := parseRepo(t, dir)
	if r.Branch != "main" {
		t.Errorf("Branch = %q, want main", r.Branch)
	}
	if r.Dirty() {
		t.Errorf("clean repo reported dirty: %+v", r)
	}
	if !r.Clean {
		t.Errorf("Clean = false, want true for a repo with no changes")
	}
	if r.Unborn || r.Detached {
		t.Errorf("Unborn/Detached = %v/%v, want false/false", r.Unborn, r.Detached)
	}
}

func TestParseStatusCountsEachKind(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"),
		testutil.WithCommit(), testutil.Dirty(), testutil.Staged(), testutil.Untracked())
	r := parseRepo(t, dir)
	if r.Unstaged != 1 {
		t.Errorf("Unstaged = %d, want 1", r.Unstaged)
	}
	if r.Staged != 1 {
		t.Errorf("Staged = %d, want 1", r.Staged)
	}
	if r.Untracked != 1 {
		t.Errorf("Untracked = %d, want 1", r.Untracked)
	}
	if r.Clean {
		t.Error("Clean = true, want false for a repo with changes")
	}
}

// TestParseStatusCountsEachKindDistinctly gives every counter a different
// value, which is the only way to prove they are not being credited to each
// other: with one of each, swapping two counters is invisible.
func TestParseStatusCountsEachKindDistinctly(t *testing.T) {
	dir := kitchenSink(t)
	r := parseRepo(t, dir)

	// staged: new-staged.txt (A.), tracked3.txt (M.), renamed.txt (R.)
	if r.Staged != 3 {
		t.Errorf("Staged = %d, want 3", r.Staged)
	}
	// unstaged: tracked1.txt, tracked2.txt (.M)
	if r.Unstaged != 2 {
		t.Errorf("Unstaged = %d, want 2", r.Unstaged)
	}
	// untracked: u1.txt .. u4.txt — and NOT the rename's original path
	if r.Untracked != 4 {
		t.Errorf("Untracked = %d, want 4", r.Untracked)
	}
	// conflicted: conflict.txt (u UU)
	if r.Conflicted != 1 {
		t.Errorf("Conflicted = %d, want 1", r.Conflicted)
	}
	if !r.Dirty() || r.Clean {
		t.Errorf("Dirty/Clean = %v/%v, want true/false", r.Dirty(), r.Clean)
	}
}

// kitchenSink builds one repository holding a different number of each kind of
// change, plus a rename whose original path is spelled to look exactly like an
// untracked record.
func kitchenSink(t *testing.T) string {
	t.Helper()
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	for _, f := range []string{"tracked1.txt", "tracked2.txt", "tracked3.txt", "? tricky.txt"} {
		writeFile(t, filepath.Join(dir, f), "a\n")
	}
	writeFile(t, filepath.Join(dir, "conflict.txt"), "base\n")
	testutil.Run(t, dir, "add", "-A")
	testutil.Run(t, dir, "commit", "-qm", "base")

	// Two branches touching the same line, merged: one unmerged entry.
	testutil.Run(t, dir, "checkout", "-qb", "other")
	writeFile(t, filepath.Join(dir, "conflict.txt"), "other\n")
	testutil.Run(t, dir, "commit", "-qam", "other")
	testutil.Run(t, dir, "checkout", "-q", "main")
	writeFile(t, filepath.Join(dir, "conflict.txt"), "main\n")
	testutil.Run(t, dir, "commit", "-qam", "main")
	tryRun(t, dir, "merge", "other") // expected to conflict

	writeFile(t, filepath.Join(dir, "tracked1.txt"), "mod\n")
	writeFile(t, filepath.Join(dir, "tracked2.txt"), "mod\n")
	writeFile(t, filepath.Join(dir, "tracked3.txt"), "mod\n")
	testutil.Run(t, dir, "add", "tracked3.txt")
	writeFile(t, filepath.Join(dir, "new-staged.txt"), "new\n")
	testutil.Run(t, dir, "add", "new-staged.txt")
	testutil.Run(t, dir, "mv", "? tricky.txt", "renamed.txt")
	for _, f := range []string{"u1.txt", "u2.txt", "u3.txt", "u4.txt"} {
		writeFile(t, filepath.Join(dir, f), "u\n")
	}
	return dir
}

// TestParseStatusRenameConsumesOriginalPathField isolates the one
// variable-length record. The original path is named so that, if it were left
// in the stream, it would parse as an untracked file — the failure mode is a
// silent miscount, so the test has to make it loud.
func TestParseStatusRenameConsumesOriginalPathField(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	writeFile(t, filepath.Join(dir, "? decoy.txt"), "content\n")
	testutil.Run(t, dir, "add", "-A")
	testutil.Run(t, dir, "commit", "-qm", "add decoy")
	testutil.Run(t, dir, "mv", "? decoy.txt", "moved.txt")

	r := parseRepo(t, dir)
	if r.Staged != 1 {
		t.Errorf("Staged = %d, want 1 (the rename counts once)", r.Staged)
	}
	if r.Untracked != 0 {
		t.Errorf("Untracked = %d, want 0 — the rename's original path was parsed as a record", r.Untracked)
	}
	if r.Unstaged != 0 {
		t.Errorf("Unstaged = %d, want 0", r.Unstaged)
	}
}

// TestParseStatusStagedThenModifiedAgain covers the one record that counts in
// both axes at once.
func TestParseStatusStagedThenModifiedAgain(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	writeFile(t, filepath.Join(dir, "README.md"), "staged\n")
	testutil.Run(t, dir, "add", "README.md")
	writeFile(t, filepath.Join(dir, "README.md"), "and modified again\n")

	r := parseRepo(t, dir)
	if r.Staged != 1 || r.Unstaged != 1 {
		t.Errorf("Staged/Unstaged = %d/%d, want 1/1 for an MM entry", r.Staged, r.Unstaged)
	}
}

func TestParseStatusUnbornHead(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r")) // no commit
	r := parseRepo(t, dir)
	if !r.Unborn {
		t.Errorf("Unborn = false, want true for a repo with no commits")
	}
	if r.Branch != "main" {
		t.Errorf("Branch = %q, want main", r.Branch)
	}
}

func TestParseStatusDetachedHead(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	sha := testutil.Run(t, dir, "rev-parse", "HEAD")
	testutil.Run(t, dir, "checkout", "-q", sha[:40])

	r := parseRepo(t, dir)
	if !r.Detached {
		t.Error("Detached = false, want true")
	}
	// Porcelain v2 reports "(detached)" and nothing else; naming the commit
	// takes a second git call, so the parser must leave Branch empty for the
	// collector to fill rather than storing the placeholder.
	if r.Branch != "" {
		t.Errorf("Branch = %q, want empty — the collector supplies the short SHA", r.Branch)
	}
	if r.Unborn {
		t.Error("Unborn = true, want false")
	}
}

func TestParseStatusNoUpstream(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	r := parseRepo(t, dir)
	if r.HasUpstream() {
		t.Errorf("Upstream = %q, want empty", r.Upstream)
	}
	if r.Ahead != 0 || r.Behind != 0 {
		t.Errorf("Ahead/Behind = %d/%d, want 0/0 with no upstream", r.Ahead, r.Behind)
	}
}

// TestParseStatusAheadBehind uses different numbers for ahead and behind so
// that reading one into the other cannot pass.
func TestParseStatusAheadBehind(t *testing.T) {
	root := t.TempDir()
	origin := testutil.NewRepo(t, filepath.Join(root, "origin"), testutil.WithCommit())
	clone := filepath.Join(root, "clone")
	testutil.Run(t, root, "clone", "-q", origin, clone)

	for _, name := range []string{"a.txt", "b.txt"} { // 2 commits ahead
		writeFile(t, filepath.Join(clone, name), "x\n")
		testutil.Run(t, clone, "add", name)
		testutil.Run(t, clone, "commit", "-qm", name)
	}
	writeFile(t, filepath.Join(origin, "o.txt"), "x\n") // 1 commit behind
	testutil.Run(t, origin, "add", "o.txt")
	testutil.Run(t, origin, "commit", "-qm", "o")
	testutil.Run(t, clone, "fetch", "-q")

	r := parseRepo(t, clone)
	if !r.HasUpstream() || r.Upstream != "origin/main" {
		t.Errorf("Upstream = %q, want origin/main", r.Upstream)
	}
	if r.Ahead != 2 {
		t.Errorf("Ahead = %d, want 2", r.Ahead)
	}
	if r.Behind != 1 {
		t.Errorf("Behind = %d, want 1", r.Behind)
	}
}

func TestParseStatusPathWithSpaces(t *testing.T) {
	dir := testutil.NewRepo(t, filepath.Join(t.TempDir(), "r"), testutil.WithCommit())
	testutil.Run(t, dir, "config", "core.quotePath", "false")
	writeFile(t, filepath.Join(dir, "a file with spaces.txt"), "x\n")

	r := parseRepo(t, dir)
	if r.Untracked != 1 {
		t.Errorf("Untracked = %d, want 1 for a filename containing spaces", r.Untracked)
	}
}

// TestParseStatusIgnoresTruncatedRecord feeds output no git would emit. The
// point is that a short record is skipped rather than indexed past the end:
// the collector must never turn a surprising repository into a panic.
func TestParseStatusIgnoresTruncatedRecord(t *testing.T) {
	var r Repo
	if err := ParseStatus([]byte("1 M\x00? \x002 R\x00"), &r); err != nil {
		t.Fatalf("ParseStatus() error = %v", err)
	}
	if r.Staged != 0 || r.Unstaged != 0 {
		t.Errorf("Staged/Unstaged = %d/%d, want 0/0 from truncated records", r.Staged, r.Unstaged)
	}
}

func TestParseAheadBehind(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		ahead, behind int
	}{
		{"typical", "+2 -1", 2, 1},
		{"synced", "+0 -0", 0, 0},
		{"behind only", "+0 -7", 0, 7},
		{"ahead only", "+7 -0", 7, 0},
		{"reversed field order", "-3 +4", 4, 3},
		{"empty", "", 0, 0},
		{"not numbers", "+x -y", 0, 0},
		{"single character fields", "+ -", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ahead, behind := parseAheadBehind(tt.in)
			if ahead != tt.ahead || behind != tt.behind {
				t.Errorf("parseAheadBehind(%q) = %d,%d, want %d,%d", tt.in, ahead, behind, tt.ahead, tt.behind)
			}
		})
	}
}
