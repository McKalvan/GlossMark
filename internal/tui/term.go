// @gloss implements gmi:ReviewTerminal scope=file

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"
)

const (
	altScreenOn  = "\x1b[?1049h\x1b[?25l"
	altScreenOff = "\x1b[?25h\x1b[?1049l"
)

// Run drives an App on the terminal behind stdin and stdout until the user
// quits. The terminal is restored on every exit path, a panic included.
func Run(eng Engine, opt Options) (err error) {
	in, out := os.Stdin, os.Stdout
	fd := int(in.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(out.Fd())) {
		return fmt.Errorf("gloss review: not a terminal; use --format json for the headless mode")
	}
	if os.Getenv("NO_COLOR") != "" {
		opt.NoColor = true
	}
	app := New(eng, opt)
	app.Reload()

	var mu sync.Mutex
	suspended := false
	var saved *term.State
	enter := func() error {
		st, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}
		saved = st
		out.WriteString(altScreenOn)
		return nil
	}
	leave := func() {
		out.WriteString(altScreenOff)
		if saved != nil {
			term.Restore(fd, saved)
			saved = nil
		}
	}
	draw := func() {
		w, h, err := term.GetSize(int(out.Fd()))
		if err != nil {
			w, h = 80, 24
		}
		out.WriteString("\x1b[H" + app.Render(w, h))
	}
	if err := enter(); err != nil {
		return err
	}
	defer func() {
		mu.Lock()
		leave()
		mu.Unlock()
		if r := recover(); r != nil {
			err = fmt.Errorf("gloss review: %v", r)
		}
	}()
	stop := onResize(func() {
		mu.Lock()
		defer mu.Unlock()
		if !suspended {
			draw()
		}
	})
	defer stop()

	buf := make([]byte, 256)
	mu.Lock()
	draw()
	mu.Unlock()
	for {
		n, rerr := in.Read(buf)
		if rerr != nil {
			return nil
		}
		mu.Lock()
		for _, k := range Decode(buf[:n]) {
			eff := app.Update(k)
			if eff.Quit {
				mu.Unlock()
				return nil
			}
			if eff.Open != nil {
				suspended = true
				leave()
				mu.Unlock()
				openErr := openEditor(eff.Open)
				mu.Lock()
				suspended = false
				if err := enter(); err != nil {
					mu.Unlock()
					return err
				}
				app.Reload()
				if openErr != nil {
					app.setStatus(false, openErr.Error())
				}
			}
		}
		draw()
		mu.Unlock()
	}
}

// openEditor runs $VISUAL or $EDITOR (vi when neither is set) on a file at
// a line: +N file, which vi, vim, nano, emacs and micro read; VS Code and
// its kin take -g file:line.
func openEditor(loc *Location) error {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed)
	args := parts[1:]
	switch base := parts[0][strings.LastIndexAny(parts[0], `/\`)+1:]; {
	case loc.Line > 0 && (base == "code" || base == "codium" || base == "cursor"):
		args = append(args, "-g", loc.Path+":"+strconv.Itoa(loc.Line))
	case loc.Line > 0:
		args = append(args, "+"+strconv.Itoa(loc.Line), loc.Path)
	default:
		args = append(args, loc.Path)
	}
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", ed, err)
	}
	return nil
}
