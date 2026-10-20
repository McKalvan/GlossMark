// @gloss implements gmi:SuggestCalibration scope=file

package model

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mckalvan/glossmark/internal/calib"
)

// SuggestConfig is the suggest: block of glossmark.yaml: the repository's
// defaults for gloss suggest, and the precision auto-accept must meet.
// Command-line flags override it (ADR 0024).
type SuggestConfig struct {
	// AutoAcceptPrecision (0 to 1) is the precision auto-accepted proposals
	// must reach. The SOFT conf threshold is derived from it: the lowest
	// conf whose cumulative precision on the calibration data meets it.
	// When set it takes precedence over policy.soft-conf-threshold; when
	// neither is set it is calib.DefaultPrecision.
	AutoAcceptPrecision float64 `yaml:"auto-accept-precision,omitempty" json:"auto-accept-precision,omitempty"`
	// MinConf is the default of gloss suggest --min-conf.
	MinConf float64 `yaml:"min-conf,omitempty" json:"min-conf,omitempty"`
	// Stopwords extend the built-in list of programming words (handler,
	// manager, process, ...) that never match a concept on their own.
	Stopwords []string `yaml:"stopwords,omitempty" json:"stopwords,omitempty"`
	// Labels is a labels file (path, symbol or *, relation, target) that
	// gloss suggest --calibrate scores against instead of the workspace's
	// own links.
	Labels string `yaml:"labels,omitempty" json:"labels,omitempty"`
	// Calibration is the repository's own measurement, written by gloss
	// suggest --calibrate --write: per-basis counts the threshold is derived
	// from instead of the shipped calibration.
	Calibration string `yaml:"calibration,omitempty" json:"calibration,omitempty"`

	measured calib.Table // the Calibration file, when it exists
	missing  bool        // Calibration is set and the file does not exist
}

// loadSuggest validates the suggest: block and reads its calibration file.
func (c *Config) loadSuggest(path string) error {
	s := c.Suggest
	if s == nil {
		return nil
	}
	if s.AutoAcceptPrecision < 0 || s.AutoAcceptPrecision > 1 {
		return fmt.Errorf("%s: suggest.auto-accept-precision must be between 0 and 1, got %v", path, s.AutoAcceptPrecision)
	}
	if s.MinConf < 0 || s.MinConf > 1 {
		return fmt.Errorf("%s: suggest.min-conf must be between 0 and 1, got %v", path, s.MinConf)
	}
	if s.Calibration == "" {
		return nil
	}
	f, err := os.Open(c.SuggestPath(s.Calibration))
	if os.IsNotExist(err) {
		s.missing = true
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	t, err := calib.Read(f)
	if err != nil {
		return fmt.Errorf("%s: suggest.calibration %s: %w", path, s.Calibration, err)
	}
	s.measured = t
	return nil
}

// SuggestPath resolves a path under suggest: against the repository root.
func (c *Config) SuggestPath(p string) string {
	if filepath.IsAbs(p) || c == nil {
		return p
	}
	return filepath.Join(c.Dir, filepath.FromSlash(p))
}

// SuggestMinConf is gloss suggest's floor: suggest.min-conf, else the
// default.
func (c *Config) SuggestMinConf() float64 {
	if c != nil && c.Suggest != nil && c.Suggest.MinConf > 0 {
		return c.Suggest.MinConf
	}
	return calib.DefaultMinConf
}

// AutoAccept is the auto-accept rule in force (ADR 0024):
//
//  1. suggest.auto-accept-precision, when set, derives the threshold;
//  2. else policy.soft-conf-threshold, when set, is the threshold (the
//     rule before ADR 0024, kept for repositories that chose a value);
//  3. else calib.DefaultPrecision derives it.
//
// The derivation reads suggest.calibration when that file exists, else
// the shipped calibration.
func (c *Config) AutoAccept() calib.AutoAccept { return c.AutoAcceptWith(0, 0) }

// AutoAcceptWith is AutoAccept with a precision target and a floor that
// override the configuration when positive: gloss suggest's
// --auto-accept-precision and --min-conf.
func (c *Config) AutoAcceptWith(target, minConf float64) calib.AutoAccept {
	tab, src := calib.Shipped, "shipped"
	var s *SuggestConfig
	if c != nil {
		s = c.Suggest
	}
	if s != nil && s.measured != nil {
		tab, src = s.measured, s.Calibration
	}
	conf := calib.Shipped.Confs()
	if minConf <= 0 {
		minConf = c.SuggestMinConf()
	}
	var a calib.AutoAccept
	switch {
	case target > 0:
		a = calib.Derive(conf, tab, target, minConf)
	case s != nil && s.AutoAcceptPrecision > 0:
		a = calib.Derive(conf, tab, s.AutoAcceptPrecision, minConf)
	case c != nil && c.Policy != nil && c.Policy.SoftConfThreshold > 0:
		a = calib.At(conf, tab, c.Policy.SoftConfThreshold, minConf)
	default:
		a = calib.Derive(conf, tab, calib.DefaultPrecision, minConf)
	}
	a.Source = src
	return a
}

// AutoAcceptNotes are what doctor and suggest say about the rule's
// configuration: a soft-conf-threshold the precision target overrides, or
// a calibration file that does not exist yet.
func (c *Config) AutoAcceptNotes() []string {
	var out []string
	if c == nil {
		return nil
	}
	s := c.Suggest
	if s != nil && s.AutoAcceptPrecision > 0 && c.Policy != nil && c.Policy.SoftConfThreshold > 0 {
		out = append(out, fmt.Sprintf("policy.soft-conf-threshold %.2f is ignored: suggest.auto-accept-precision %.2f sets the threshold; remove one of them", c.Policy.SoftConfThreshold, s.AutoAcceptPrecision))
	}
	if s != nil && s.missing {
		out = append(out, fmt.Sprintf("suggest.calibration %s does not exist; the threshold is derived from the shipped calibration until gloss suggest --calibrate --write measures this repository", s.Calibration))
	}
	return out
}
