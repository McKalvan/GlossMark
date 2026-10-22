// @gloss implements gmi:ReviewTerminal scope=file

package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/review"
)

// viewerOpener builds the HTML viewer (gloss view) and opens it in a
// browser on the slice of a concept or boundary: the terminal's v key.
func viewerOpener(e *review.Engine) func(seed string) (string, error) {
	return func(seed string) (string, error) {
		if code, out := invoke("view", "--root", e.Root); code != 0 {
			return "", fmt.Errorf("gloss view: %s", strings.TrimSpace(out))
		}
		u := viewerURL(filepath.Join(e.Root, load.StateDir, "viewer.html"), seed)
		cmd := browserCommand(u)
		if cmd == nil {
			return "viewer built; no browser here, open " + u, nil
		}
		if err := cmd.Start(); err != nil {
			return fmt.Sprintf("viewer built; %s did not start (%v), open %s", cmd.Path, err, u), nil
		}
		go cmd.Wait()
		if seed == "" {
			return "opened the viewer in a browser", nil
		}
		return "opened the viewer on the slice of " + seed, nil
	}
}

// viewerURL is the file URL of the viewer, with the #slice= deep link the
// page reads when seed names a concept or boundary.
func viewerURL(page, seed string) string {
	p := filepath.ToSlash(page)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/repo/... on Windows
	}
	u := (&url.URL{Scheme: "file", Path: p}).String()
	if seed != "" {
		u += "#slice=" + url.PathEscape(seed)
	}
	return u
}

// browserCommand opens a URL: $BROWSER when set, else the platform's
// opener; nil on a Linux machine with no display (over SSH), where the
// caller prints the URL instead.
func browserCommand(u string) *exec.Cmd {
	if b := strings.Fields(os.Getenv("BROWSER")); len(b) > 0 {
		return quiet(exec.Command(b[0], append(b[1:], u)...))
	}
	switch runtime.GOOS {
	case "darwin":
		return quiet(exec.Command("open", u))
	case "windows":
		return quiet(exec.Command("rundll32", "url.dll,FileProtocolHandler", u))
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil
	}
	if _, err := exec.LookPath("xdg-open"); err != nil {
		return nil
	}
	return quiet(exec.Command("xdg-open", u))
}

// quiet detaches a browser from the terminal, so its output cannot draw
// over the review screen.
func quiet(c *exec.Cmd) *exec.Cmd {
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	return c
}
