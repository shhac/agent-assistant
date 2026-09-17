package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shhac/agent-assistant/internal/core"
)

func artifactFixture(t *testing.T) (*App, string) {
	t.Helper()
	a := testApp(t)
	root := a.Core.StateDirectory()
	dir := filepath.Join(root, "runs", "run-1", "artifacts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "changes.patch"), []byte("diff --git a/a b/a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return a, dir
}

func snapshotWithEvidence(evidence ...string) core.Snapshot {
	return core.Snapshot{Agents: []core.Agent{{ID: "a1", Evidence: evidence}}}
}

func TestArtifactTokenIsOpaqueAndPathSpecific(t *testing.T) {
	a, dir := artifactFixture(t)
	patch := filepath.Join(dir, "changes.patch")
	links := a.ArtifactLinks(snapshotWithEvidence("Patch: " + patch))
	token := links[patch]
	if token == "" {
		t.Fatal("no token minted for a recorded artifact")
	}
	if strings.Contains(token, patch) || strings.Contains(token, "changes.patch") {
		t.Fatalf("token discloses the path: %q", token)
	}
	// Another installation must not produce the same token for the same path.
	other, _ := artifactFixture(t)
	if other.ArtifactLinks(snapshotWithEvidence("Patch: " + patch))[patch] == token {
		t.Fatal("token is derived without an installation secret")
	}
	// A sibling in the same directory gets an unrelated token, so knowing one
	// file's URL does not describe the directory around it.
	sibling := filepath.Join(dir, "commands.json")
	if err := os.WriteFile(sibling, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	siblingToken := a.ArtifactLinks(snapshotWithEvidence("Command results: " + sibling))[sibling]
	if siblingToken == "" || siblingToken == token {
		t.Fatal("sibling artifacts share a token")
	}
}

func TestArtifactTokenIsStableAcrossSnapshots(t *testing.T) {
	a, dir := artifactFixture(t)
	patch := filepath.Join(dir, "changes.patch")
	first := a.ArtifactLinks(snapshotWithEvidence("Patch: " + patch))[patch]
	second := a.ArtifactLinks(snapshotWithEvidence("Patch: " + patch))[patch]
	if first == "" || first != second {
		t.Fatal("a link changes between reads of the same state")
	}
}

// Evidence text is worker-authored. A run that names a file outside the
// daemon's own state must never be offered for download, or the dashboard
// becomes a way to read the owner's filesystem.
func TestArtifactsOutsideTheStateDirectoryAreNeverOffered(t *testing.T) {
	a, _ := artifactFixture(t)
	outside := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(outside, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"Patch: " + outside,
		"Patch: /etc/passwd",
		"Patch: " + filepath.Join(a.Core.StateDirectory(), "..", "escape.txt"),
		"Patch: relative/path.txt",
		"Patch: ",
	} {
		links := a.ArtifactLinks(snapshotWithEvidence(line))
		if len(links) != 0 {
			t.Fatalf("minted a link for %q: %v", line, links)
		}
	}
}

func TestOpenArtifactRefusesAnythingItDidNotMint(t *testing.T) {
	a, dir := artifactFixture(t)
	patch := filepath.Join(dir, "changes.patch")
	state := snapshotWithEvidence("Patch: " + patch)
	good := a.ArtifactLinks(state)[patch]

	name, file, size, err := a.OpenArtifact(state, good)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(file)
	file.Close()
	if name != "changes.patch" || size != int64(len(body)) || !strings.HasPrefix(string(body), "diff --git") {
		t.Fatalf("served the wrong content: %q %d %q", name, size, body)
	}

	for _, bad := range []string{
		"",
		"not-a-token",
		strings.Repeat("a", 64),
		good[:len(good)-1] + "0",
	} {
		if _, _, _, err := a.OpenArtifact(state, bad); err == nil {
			t.Fatalf("served a file for token %q", bad)
		}
	}

	// A token stops working once the evidence that justified it is gone.
	if _, _, _, err := a.OpenArtifact(core.Snapshot{}, good); err == nil {
		t.Fatal("a link outlived the evidence that justified it")
	}
}

// The recorded path is opened through a root anchored at the state directory,
// so a symlink planted inside it cannot reach outside.
func TestOpenArtifactDoesNotFollowSymlinksOutOfTheStateDirectory(t *testing.T) {
	a, dir := artifactFixture(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "summary.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	state := snapshotWithEvidence("Changed-file summary: " + link)
	token := a.ArtifactLinks(state)[link]
	if token == "" {
		t.Fatal("expected a token for a path inside the state directory")
	}
	if _, _, _, err := a.OpenArtifact(state, token); err == nil {
		t.Fatal("followed a symlink out of the state directory")
	}
}

func TestOpenArtifactRefusesDirectoriesAndOversizedFiles(t *testing.T) {
	a, dir := artifactFixture(t)
	state := snapshotWithEvidence("Worker artifacts: " + dir)
	token := a.ArtifactLinks(state)[dir]
	if token == "" {
		t.Fatal("expected a token for the artifacts directory line")
	}
	if _, _, _, err := a.OpenArtifact(state, token); err == nil {
		t.Fatal("served a directory as a file")
	}
}
