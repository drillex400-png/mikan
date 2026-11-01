package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mikan/internal/release"
)

const testDigest = "sha256:0a6b95f8f89bf825c40bc11f10546b7c22e27531d152c540ba8ba9035675e0f8"

func TestAddonIsSignedByTheReleaseKeyAndNamesIt(t *testing.T) {
	f := newFixture(t, "")
	out := filepath.Join(f.dir, "out")
	if err := makeAddon([]string{"-version", "0.5.2.1", "-image", "ghcr.io/o/mikan-bot", "-digest", testDigest, "-out", out}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "mikan-addon.json"))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(filepath.Join(out, "mikan-addon.json.sig"))
	if err != nil {
		t.Fatal(err)
	}
	if err := release.Verify(data, strings.TrimSpace(string(sig)), f.pub); err != nil {
		t.Fatalf("the signature does not match the file: %v", err)
	}
	var doc addonFile
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Key != base64.StdEncoding.EncodeToString(f.pub) {
		t.Fatalf("the file names key %q, not the one it is signed with", doc.Key)
	}
	if len(doc.Adapters) != 1 {
		t.Fatalf("want one addon, got %d", len(doc.Adapters))
	}
	e := doc.Adapters[0]
	if e.ID != "mikan-bot" || e.Kind != "bot" || e.Protocol != 2 || e.Digest != testDigest {
		t.Fatalf("unexpected entry %+v", e)
	}
	// The official bot's permissions and no more: the admin's screens need nothing else.
	want := "bot users:read users:write billing:invoice events kv"
	if got := strings.Join(e.Permissions, " "); got != want {
		t.Fatalf("permissions %q, want %q", got, want)
	}
}

func TestAddonRefusesWhatThePanelWouldRefuse(t *testing.T) {
	f := newFixture(t, "")
	out := filepath.Join(f.dir, "out")
	base := []string{"-version", "1.0.0", "-image", "ghcr.io/o/mikan-bot", "-out", out}
	for name, args := range map[string][]string{
		"the official bot's id": append([]string{"-id", "telegram", "-digest", testDigest}, base...),
		"no digest":             append([]string{}, base...),
		"a tag, not a digest":   append([]string{"-digest", "latest"}, base...),
		"an invalid id":         append([]string{"-id", "Bad Id", "-digest", testDigest}, base...),
		"no image":              {"-version", "1.0.0", "-digest", testDigest, "-out", out},
	} {
		if err := makeAddon(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "mikan-addon.json")); err == nil {
		t.Error("a refused addon left a file")
	}
}
