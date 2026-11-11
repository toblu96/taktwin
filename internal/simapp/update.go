package simapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

	"github.com/toblu96/taktwin/internal/version"
)

// releases is the repository whose GitHub releases the app updates itself from.
const releases = "toblu96/taktwin"

// UpdateService keeps the app up to date from its GitHub releases, with Wails' updater: it
// checks for a newer release, downloads the file for this platform, verifies it against the
// release's SHA256SUMS, and swaps it in on a restart. Only a release build replaces itself;
// one run from the sources only checks.
type UpdateService struct {
	mu      sync.Mutex
	ctx     context.Context
	ready   bool
	latest  *UpdateRelease
	problem string
}

// UpdateStatus is what Settings shows about updates.
type UpdateStatus struct {
	// Version is the running app's version.
	Version string `json:"version"`
	// Installable says whether the app can replace itself: only a release build can.
	Installable bool `json:"installable"`
	// AutoCheck says whether the app looks for an update when it starts.
	AutoCheck bool `json:"autoCheck"`
	// State is the updater's: idle, checking, up-to-date, available, downloading, verifying,
	// installing, ready or error.
	State string `json:"state"`
	// Latest is the newer release, once a check found one.
	Latest *UpdateRelease `json:"latest,omitempty"`
	// Ready says that the update is downloaded and verified, and takes effect on a restart.
	Ready bool `json:"ready"`
	// Problem is what went wrong last.
	Problem string `json:"problem,omitempty"`
}

// UpdateRelease is a newer release than the running one.
type UpdateRelease struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	// Notes are the release notes, in Markdown.
	Notes string `json:"notes"`
	// URL is the release's page.
	URL string `json:"url"`
}

// NewUpdateService gives the update service.
func NewUpdateService() *UpdateService { return &UpdateService{} }

// ServiceStartup sets Wails' updater up, and looks for an update a little after the start
// when the settings say so.
func (s *UpdateService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx = ctx
	app := application.Get()
	if app == nil || app.Updater == nil {
		return nil
	}
	provider, err := github.New(github.Config{Repository: releases, ChecksumAsset: "SHA256SUMS", AssetMatcher: matchAsset})
	if err != nil {
		return err
	}
	// The app shows the update in its own Settings, not in a window of the updater's.
	if err := app.Updater.Init(updater.Config{CurrentVersion: version.Version, Providers: []updater.Provider{provider}, Window: updater.WindowNone}); err != nil {
		return err
	}
	if autoCheck() {
		go func() {
			select {
			case <-time.After(5 * time.Second):
				_, _ = s.Check()
			case <-ctx.Done():
			}
		}()
	}
	return nil
}

// Status gives what Settings shows about updates.
func (s *UpdateService) Status() UpdateStatus {
	st := UpdateStatus{Version: version.Version, Installable: releaseBuild, AutoCheck: autoCheck(), State: string(updater.StateUnconfigured)}
	if app := application.Get(); app != nil && app.Updater != nil {
		st.State = string(app.Updater.State())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st.Latest, st.Ready, st.Problem = s.latest, s.ready, s.problem
	return st
}

// Check looks for a newer release.
func (s *UpdateService) Check() (UpdateStatus, error) {
	u, err := s.updater()
	if err != nil {
		return s.Status(), err
	}
	rel, err := u.Check(s.context())
	s.mu.Lock()
	s.problem = ""
	if err != nil {
		s.problem = "The check for updates failed: " + err.Error()
	}
	if rel != nil {
		s.latest = &UpdateRelease{Version: rel.Version, Name: rel.Name, Notes: rel.Notes, URL: releasePage(rel.Version)}
	} else if err == nil {
		s.latest = nil
	}
	s.mu.Unlock()
	return s.Status(), err
}

// Install downloads the newer release that Check found and verifies it, ready to take
// effect on Restart. Its progress comes as Wails' updater events.
func (s *UpdateService) Install() (UpdateStatus, error) {
	if !releaseBuild {
		return s.Status(), errors.New("only an installed release of Taktwin updates itself; this one runs from the sources")
	}
	u, err := s.updater()
	if err != nil {
		return s.Status(), err
	}
	err = u.DownloadAndInstall(s.context())
	s.mu.Lock()
	s.ready = err == nil
	s.problem = ""
	if err != nil {
		s.problem = "The update could not be installed: " + err.Error()
	}
	s.mu.Unlock()
	return s.Status(), err
}

// Restart quits the app, swaps the update in and starts it again.
func (s *UpdateService) Restart() error {
	u, err := s.updater()
	if err != nil {
		return err
	}
	return u.Restart(s.context())
}

// SetAutoCheck says whether the app looks for an update when it starts.
func (s *UpdateService) SetAutoCheck(on bool) error {
	set := readSettings()
	set.CheckUpdates = &on
	return writeSettings(set)
}

// OpenRelease opens the page of a release in the browser.
func (s *UpdateService) OpenRelease(url string) {
	if app := application.Get(); app != nil && strings.HasPrefix(url, "https://github.com/"+releases+"/") {
		_ = app.Browser.OpenURL(url)
	}
}

func (s *UpdateService) updater() (*updater.Updater, error) {
	app := application.Get()
	if app == nil || app.Updater == nil {
		return nil, errors.New("the updater is not running")
	}
	return app.Updater, nil
}

func (s *UpdateService) context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// autoCheck is the setting for checking at the start, on unless switched off.
func autoCheck() bool {
	set := readSettings()
	return set.CheckUpdates == nil || *set.CheckUpdates
}

func releasePage(v string) string {
	return fmt.Sprintf("https://github.com/%s/releases/tag/v%s", releases, strings.TrimPrefix(v, "v"))
}

// matchAsset picks the file of a release that replaces the running app: the zipped app for
// macOS (one for both architectures), the portable exe for 64-bit Windows. The disk image and
// the installer are for people, not for the updater. The names are the release workflow's.
func matchAsset(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	for i, a := range assets {
		name := strings.ToLower(a.Name)
		switch req.Platform {
		case "darwin":
			if strings.Contains(name, "-macos-") && strings.HasSuffix(name, ".zip") {
				return i
			}
		case "windows":
			if req.Arch == "amd64" && strings.Contains(name, "-windows-x64") && strings.HasSuffix(name, ".exe") &&
				!strings.Contains(name, "setup") && !strings.Contains(name, "installer") {
				return i
			}
		}
	}
	return -1
}
