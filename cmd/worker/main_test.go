package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSettings checks that the settings come back as they were saved, and
// that a missing or broken file is the default.
func TestSettings(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings.json")
	if got := loadSettings(file); got != (settings{}) {
		t.Errorf("a missing file loads as %+v", got)
	}
	want := settings{Remember: true, On: true}
	if err := saveSettings(file, want); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(file); got != want {
		t.Errorf("loaded %+v, want %+v", got, want)
	}
	os.WriteFile(file, []byte("{broken"), 0o644)
	if got := loadSettings(file); got != (settings{}) {
		t.Errorf("a broken file loads as %+v", got)
	}
}

// TestSetPhase checks each phase against the state that means it.
func TestSetPhase(t *testing.T) {
	cases := []struct {
		s       Status
		running bool
		want    string
	}{
		{Status{}, false, phaseNoLid},
		{Status{HasLid: true}, false, phaseOff},
		{Status{HasLid: true}, true, phaseStopping},
		{Status{HasLid: true, On: true}, true, phaseStarting},
		{Status{HasLid: true, On: true, armed: true}, true, phaseOpen},
		{Status{HasLid: true, On: true, armed: true, LidClosed: true}, true, phaseClosed},
	}
	for _, c := range cases {
		c.s.setPhase(c.running)
		if c.s.Phase != c.want {
			t.Errorf("%+v running=%v: phase %q, want %q", c.s, c.running, c.s.Phase, c.want)
		}
	}
}

// TestRuleProblem checks the sentences for the ways the rule is not written.
func TestRuleProblem(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{errors.New("execution error: User canceled. (-128)"), "The administrator's password was not given."},
		{errors.New("visudo: >>> wake-501.tmp: syntax error near line 3 <<< (exit status 1)"), "The rule for sudo could not be written. visudo: >>> wake-501.tmp: syntax error near line 3 <<< (exit status 1)."},
	}
	for _, c := range cases {
		if got := ruleProblem(c.err); got != c.want {
			t.Errorf("ruleProblem(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

// TestHelperProblem checks the sentences for the ways the helper ends.
func TestHelperProblem(t *testing.T) {
	failed := errors.New("exit status 1")
	cases := []struct {
		err    error
		stderr string
		want   string
	}{
		{failed, "could not disable sleep: pmset: no permission (exit status 1)", "Sleep could not be disabled. could not disable sleep: pmset: no permission (exit status 1)."},
		{failed, "something else\nthe last line", "It stopped. the last line."},
		{failed, "", "It stopped: exit status 1."},
		{nil, "", "It stopped by itself."},
	}
	for _, c := range cases {
		if got := helperProblem(c.err, c.stderr); got != c.want {
			t.Errorf("helperProblem(%v, %q) = %q, want %q", c.err, c.stderr, got, c.want)
		}
	}
}
