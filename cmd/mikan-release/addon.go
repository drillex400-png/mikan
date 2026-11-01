package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mikan/internal/panel/addons"
	"mikan/internal/release"
)

// addonFile is the document of a source of third-party addons (internal/panel/addons/sources.go):
// the public key it is signed with, and the addons it offers. The panel reads it from a release
// of the repository (mikan-addon.json and mikan-addon.json.sig) when the admin adds the
// repository as a source.
type addonFile struct {
	Title    string         `json:"title"`
	Key      string         `json:"key"`
	Adapters []addons.Entry `json:"adapters"`
}

// makeAddon writes mikan-addon.json for the bot addon of this repository (the bot with the
// admin's screens) and its signature, made with the release key over the exact bytes.
//
//	RELEASE_SIGNING_KEY="$(cat key.pem)" mikan-release addon -id mikan-bot -version 0.5.2.1 \
//	    -image ghcr.io/drillex400-png/mikan-bot -digest sha256:… -out dist
func makeAddon(args []string) error {
	fs := flag.NewFlagSet("addon", flag.ContinueOnError)
	id := fs.String("id", "mikan-bot", "id of the addon (not telegram: that is the official bot's)")
	version := fs.String("version", "", "version of the addon, e.g. 0.5.2.1")
	image := fs.String("image", "", "image repository, e.g. ghcr.io/drillex400-png/mikan-bot")
	digest := fs.String("digest", "", "sha256 digest of the pushed multi-arch image")
	homepage := fs.String("homepage", "", "https address of the repository")
	minPanel := fs.String("min-panel", "", "the oldest panel that runs it")
	out := fs.String("out", "dist", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *image == "" {
		return errors.New("-version and -image are required")
	}
	if !release.ValidDigest(*digest) {
		return fmt.Errorf("-digest %q is not a sha256 digest", *digest)
	}
	if !addons.ValidID(*id) || *id == addons.OfficialBotID {
		return fmt.Errorf("-id %q: not a valid id, or the official bot's", *id)
	}
	key, err := privateKey(os.Getenv("RELEASE_SIGNING_KEY"))
	if err != nil {
		return err
	}
	pub := key.Public().(ed25519.PublicKey)
	entry := addons.Entry{
		ID:   *id,
		Name: addons.Text{"en": "Subscribers' bot with admin screens", "ru": "Бот подписчиков с экранами администратора"},
		Description: addons.Text{
			"en": "The official bot's screens plus /admin for the Telegram accounts named in Telegram settings. Take the bot from the official one in Addons.",
			"ru": "Экраны официального бота и команда /admin для аккаунтов, указанных в настройках Telegram. Отдайте ему бота в разделе аддонов.",
		},
		Version:  *version,
		Protocol: addons.Protocol2,
		Kind:     addons.KindBot,
		Mode:     addons.ModeClient,
		// The official bot's permissions, and the two the referral programme needs: kv for who
		// invited whom, users:all to find the inviter's subscription and users:grant to give the
		// days (each under a key, so a retry never gives twice). The admin approves them in Addons.
		Permissions: []string{"bot", "users:read", "users:write", "users:all", "users:grant", "billing:invoice", "events", "kv"},
		Image:       *image,
		Digest:      *digest,
		MinPanel:    *minPanel,
		Homepage:    *homepage,
	}
	f := addonFile{Title: "mikan (drillex400-png)", Key: base64.StdEncoding.EncodeToString(pub), Adapters: []addons.Entry{entry}}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	file := filepath.Join(*out, "mikan-addon.json")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	sig := release.Sign(data, key)
	if err := os.WriteFile(file+".sig", []byte(sig+"\n"), 0o644); err != nil {
		return err
	}
	if err := release.Verify(data, sig, pub); err != nil {
		return err
	}
	fmt.Printf("ok: %s %s %s@%s\n", *id, *version, strings.TrimSpace(*image), *digest)
	return nil
}
