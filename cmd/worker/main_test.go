package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSettings checks that the settings come back as they were saved, that
// a missing or broken file is the open mode, and that a file written before
// there were modes is the closed mode, which was the only one.
func TestSettings(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings.json")
	if got := loadSettings(file); got != (settings{Mode: modeOpen}) {
		t.Errorf("a missing file loads as %+v", got)
	}
	want := settings{Remember: true, On: true, Mode: modeOpen, Display: true, Minutes: 120}
	if err := saveSettings(file, want); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(file); got != want {
		t.Errorf("loaded %+v, want %+v", got, want)
	}
	os.WriteFile(file, []byte(`{"remember":true,"on":true}`), 0o644)
	if got := loadSettings(file); got != (settings{Remember: true, On: true, Mode: modeClosed}) {
		t.Errorf("a file from before the modes loads as %+v", got)
	}
	os.WriteFile(file, []byte(`{"mode":"sideways","minutes":-5}`), 0o644)
	if got := loadSettings(file); got != (settings{Mode: modeOpen}) {
		t.Errorf("a file with a strange mode and timer loads as %+v", got)
	}
	os.WriteFile(file, []byte("{broken"), 0o644)
	if got := loadSettings(file); got != (settings{Mode: modeOpen}) {
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
		{Status{Mode: modeOpen}, false, phaseNoLid},
		{Status{Mode: modeOpen, On: true, armed: true}, false, phaseAwake},
		{Status{HasLid: true, Mode: modeOpen}, false, phaseOff},
		{Status{HasLid: true, Mode: modeOpen, On: true, armed: true}, false, phaseAwake},
		{Status{HasLid: true, Mode: modeOpen, On: true, armed: true}, true, phaseAwake},
		{Status{HasLid: true, Mode: modeClosed}, false, phaseOff},
		{Status{HasLid: true, Mode: modeClosed}, true, phaseStopping},
		{Status{HasLid: true, Mode: modeClosed, On: true}, true, phaseStarting},
		{Status{HasLid: true, Mode: modeClosed, On: true, armed: true}, true, phaseOpen},
		{Status{HasLid: true, Mode: modeClosed, On: true, armed: true, LidClosed: true}, true, phaseClosed},
	}
	for _, c := range cases {
		c.s.setPhase(c.running)
		if c.s.Phase != c.want {
			t.Errorf("%+v running=%v: phase %q, want %q", c.s, c.running, c.s.Phase, c.want)
		}
	}
}

// TestSetOptionsWhileOff checks that the options are checked and kept
// while it is off, without anything being started.
func TestSetOptionsWhileOff(t *testing.T) {
	a := &app{settings: filepath.Join(t.TempDir(), "settings.json")}
	s := &Status{HasLid: true, Mode: modeOpen}
	if err := a.setOptions(s, Options{Mode: modeClosed, Display: true, Minutes: 60}); err != nil {
		t.Fatal(err)
	}
	if s.Mode != modeClosed || !s.Display || s.Minutes != 60 || s.On || s.Until != "" {
		t.Errorf("after setOptions: %+v", s)
	}
	if got := loadSettings(a.settings); got != (settings{Mode: modeClosed, Display: true, Minutes: 60}) {
		t.Errorf("saved %+v", got)
	}
	for _, o := range []Options{{Mode: "sideways"}, {Mode: modeOpen, Minutes: -1}, {Mode: modeOpen, Minutes: longest + 1}} {
		if err := a.setOptions(s, o); err == nil {
			t.Errorf("setOptions(%+v) was allowed", o)
		}
	}
	desk := &Status{Mode: modeOpen}
	if err := a.setOptions(desk, Options{Mode: modeClosed}); err != noLid {
		t.Errorf("the closed mode on a Mac without a lid: %v", err)
	}
	if err := a.turnOn(&Status{Mode: modeClosed}); err != noLid {
		t.Errorf("turning on the closed mode without a lid: %v", err)
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
