package selfupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

type Config struct {
	ExecutablePath, HomeDir, Version, MinimumVersion string
	Now                                              func() time.Time
	Source                                           Source
	Readback                                         ReadbackFunc
}
type Updater struct{ config Config }

func DefaultConfig(version string) (Config, error) {
	exe, e := os.Executable()
	if e != nil {
		return Config{}, errors.New("executable unavailable")
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return Config{}, errors.New("home unavailable")
	}
	exe, e = filepath.Abs(exe)
	if e != nil || !filepath.IsAbs(home) {
		return Config{}, errors.New("invalid installation")
	}
	return Config{ExecutablePath: exe, HomeDir: home, Version: version, MinimumVersion: "0.16.0", Now: time.Now, Source: newGitHubSource(nil), Readback: ReadInstalled}, nil
}
func New(c Config) *Updater {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.MinimumVersion == "" {
		c.MinimumVersion = "0.16.0"
	}
	if c.Source == nil {
		c.Source = newGitHubSource(nil)
	}
	if c.Readback == nil {
		c.Readback = ReadInstalled
	}
	return &Updater{config: c}
}
func (u *Updater) Check(ctx context.Context) (Status, error) {
	s, e := ReadCached(u.config)
	if e != nil {
		return Status{}, e
	}
	v, e := u.config.Readback(ctx, u.config.ExecutablePath)
	if e != nil || v == "" {
		return Status{}, errors.New("installed version unavailable")
	}
	if s.LastResult == "updated" && v != s.Latest {
		return Status{}, errors.New("installed version contradicts cached update")
	}
	s.Installed = v
	channel, auto, _, e := updatePolicy(u.config)
	if e != nil {
		return Status{}, e
	}
	s.Channel = channel
	s.Auto = auto
	return s, nil
}
func (u *Updater) Apply(ctx context.Context, automatic bool) (Status, error) {
	c := u.config
	installed, e := c.Readback(ctx, c.ExecutablePath)
	if e != nil || installed == "" {
		return Status{}, errors.New("installed version unavailable")
	}
	channel, auto, _, e := updatePolicy(c)
	if e != nil {
		return Status{}, e
	}
	s := Status{Schema: "update-status/v1", Tool: "mi-lsp", Installed: installed, Channel: channel, Auto: auto}
	var inside bool
	e = runtimeinfo.WithFileLock(updateLockPath(c.HomeDir), func() error {
		inside = true
		actual, err := c.Readback(ctx, c.ExecutablePath)
		if err != nil || actual == "" {
			return errors.New("installed version unavailable")
		}
		s.Installed = actual
		channel, auto, currentReason, err := updatePolicy(c)
		if err != nil {
			return err
		}
		s.Channel, s.Auto = channel, auto
		return u.applyLocked(ctx, automatic, currentReason, &s)
	})
	if !inside && isLockBusy(e) {
		s.LastResult = "deferred"
		code := "UPDATE_LOCKED"
		s.Code = &code
		return s, nil
	}
	if e != nil {
		return s, errors.New("update failed")
	}
	return s, nil
}
func (u *Updater) applyLocked(ctx context.Context, automatic bool, reason string, s *Status) error {
	c := u.config
	prior, _ := ReadCached(c)
	if reason != "" {
		s.LastResult = reason
		s.Latest = prior.Latest
		s.LastCheck = prior.LastCheck
		return saveStatus(c, *s)
	}
	now := c.Now().UTC()
	var cache updateCache
	cacheErr := readJSON(cachePath(c), &cache)
	if automatic && cacheErr == nil && !cache.LastCheck.IsZero() && now.Sub(cache.LastCheck) < 24*time.Hour {
		if prior.LastResult != "" {
			*s = prior
			actual, e := c.Readback(ctx, c.ExecutablePath)
			if e != nil {
				return errors.New("readback failed")
			}
			if s.LastResult == "updated" && actual != s.Latest {
				return errors.New("readback mismatch")
			}
			s.Installed = actual
			return nil
		}
		s.LastCheck = &cache.LastCheck
		if cache.Error != "" {
			return u.finishFailure(s, cache.Error)
		}
		return errors.New("no state for installation")
	}
	cache = updateCache{LastCheck: now, Error: "UPDATE_NETWORK_UNAVAILABLE"}
	if e := saveCache(c, cache); e != nil {
		return e
	}
	release, e := c.Source.Latest(ctx)
	if e != nil {
		code := "UPDATE_NETWORK_UNAVAILABLE"
		if errors.Is(e, errNoRelease) {
			code = "UPDATE_NO_RELEASE"
		}
		cache.Error = code
		_ = saveCache(c, cache)
		s.LastCheck = &now
		return u.finishFailure(s, code)
	}
	cache.Release = release
	cache.Error = ""
	if e = saveCache(c, cache); e != nil {
		return e
	}
	s.LastCheck = &now
	s.Latest = release.Version
	cmp, e := compareVersions(release.Version, s.Installed)
	if e != nil {
		return u.finishFailure(s, "UPDATE_NO_RELEASE")
	}
	if cmp < 0 {
		return u.finishFailure(s, "UPDATE_DOWNGRADE_REFUSED")
	}
	if cmp == 0 {
		s.LastResult = "up_to_date"
		return saveStatus(c, *s)
	}
	floor, e := compareVersions(release.Version, c.MinimumVersion)
	if e != nil {
		return u.finishFailure(s, "UPDATE_NO_RELEASE")
	}
	if floor < 0 {
		return u.finishFailure(s, "UPDATE_BELOW_FLOOR")
	}
	archiveName := releaseAssetName(release.Version)
	checksName := "mi-lsp_" + release.Version + "_checksums.txt"
	archive, ok := findAsset(release.Assets, archiveName)
	if !ok {
		return u.finishFailure(s, "UPDATE_NO_RELEASE")
	}
	checks, ok := findAsset(release.Assets, checksName)
	if !ok {
		return u.finishFailure(s, "UPDATE_NO_RELEASE")
	}
	if e = os.MkdirAll(updateDir(c.HomeDir), 0700); e != nil {
		return u.finishFailure(s, "UPDATE_PERMISSION_DENIED")
	}
	dir, e := os.MkdirTemp(updateDir(c.HomeDir), "stage-")
	if e != nil {
		return u.finishFailure(s, "UPDATE_PERMISSION_DENIED")
	}
	defer os.RemoveAll(dir)
	checkPath := filepath.Join(dir, checksName)
	if e = c.Source.Download(ctx, checks, checkPath); e != nil {
		return u.finishFailure(s, "UPDATE_NETWORK_UNAVAILABLE")
	}
	hash, e := checksumFor(checkPath, archiveName)
	if e != nil {
		return u.finishFailure(s, "UPDATE_CHECKSUM_MISMATCH")
	}
	archivePath := filepath.Join(dir, archiveName)
	if e = c.Source.Download(ctx, archive, archivePath); e != nil {
		return u.finishFailure(s, "UPDATE_NETWORK_UNAVAILABLE")
	}
	f, e := os.Open(archivePath)
	if e != nil {
		return errors.New("archive unavailable")
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, maxAsset+1))
	_ = f.Close()
	if e != nil || n > maxAsset || hex.EncodeToString(h.Sum(nil)) != hash {
		return u.finishFailure(s, "UPDATE_CHECKSUM_MISMATCH")
	}
	previous := s.Installed
	result, txErr := ApplyTransaction(ctx, Transaction{ExecutablePath: c.ExecutablePath, ArchivePath: archivePath, ExpectedArchiveSHA256: hash, TargetVersion: release.Version, Readback: c.Readback})
	actual, readErr := c.Readback(ctx, c.ExecutablePath)
	if readErr != nil || actual == "" {
		return errors.New("installed version unavailable")
	}
	s.Installed = actual
	if txErr != nil || result.Code != "" {
		if !allowedCodes[result.Code] {
			return errors.New("transaction failed")
		}
		return u.finishFailure(s, result.Code)
	}
	if !result.Changed || actual != release.Version {
		return errors.New("transaction readback mismatch")
	}
	old := result.Previous
	if old == "" || old != previous {
		return errors.New("previous version unavailable")
	}
	s.Previous = &old
	s.LastResult = "updated"
	return saveStatus(c, *s)
}
func (u *Updater) finishFailure(s *Status, code string) error {
	if !allowedCodes[code] {
		return errors.New("unknown update failure")
	}
	s.LastResult = "failed"
	if code == "UPDATE_LOCKED" || code == "UPDATE_IN_USE_DEFERRED" {
		s.LastResult = "deferred"
	}
	s.Code = &code
	s.Previous = nil
	return saveStatus(u.config, *s)
}
func findAsset(as []Asset, name string) (Asset, bool) {
	var out Asset
	found := false
	for _, a := range as {
		if a.Name == name {
			if found {
				return Asset{}, false
			}
			out = a
			found = true
		}
	}
	return out, found
}
func releaseAssetName(version string) string {
	osname := runtime.GOOS
	arch := runtime.GOARCH
	if osname == "windows" {
		osname = "win"
	}
	if arch == "amd64" {
		arch = "x64"
	}
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return "mi-lsp_" + version + "_" + osname + "-" + arch + ext
}
func checksumFor(file, name string) (string, error) {
	f, e := os.Open(file)
	if e != nil {
		return "", e
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 2<<20))
	found := ""
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		n := strings.TrimPrefix(fields[1], "*")
		if n != name {
			continue
		}
		if found != "" || len(fields[0]) != 64 {
			return "", errors.New("invalid checksum")
		}
		if _, e := hex.DecodeString(fields[0]); e != nil {
			return "", errors.New("invalid checksum")
		}
		found = strings.ToLower(fields[0])
	}
	if sc.Err() != nil || found == "" {
		return "", errors.New("missing checksum")
	}
	return found, nil
}
