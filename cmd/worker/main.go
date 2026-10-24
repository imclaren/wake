// Command worker is wake's worker. It keeps a MacBook awake after its
// lid is closed, as caffeinate+ does (github.com/clashman/caffeinate-plus),
// and the menu bar turns that on and off.
//
// The setting that stops a closing lid putting the Mac to sleep,
// SleepDisabled, may be changed only by an administrator. The first time
// it is turned on, the worker writes a rule for sudo, through osascript's
// "with administrator privileges", which asks for the administrator's
// password once. The rule lets this account run the three pmset commands
// that change the setting and put the Mac to sleep, and nothing else,
// without a password, and it stays until the file is removed, so the
// password is not asked for again, on this launch or any other. Then the
// worker runs a second copy of itself, the helper in helper.go, which
// disables sleep while the lid is open and enables it again as soon as the
// lid closes. That is caffeinate+'s trick: closing the lid is what puts a
// Mac to sleep, and once the lid is closed the Mac stays awake. The helper
// ends when the file the worker made for it goes away, or when the worker
// does. If the lid is closed when it is turned off, it puts the Mac to
// sleep, as caffeinate+ does when it is stopped.
//
// While it is on, the worker also holds an assertion against idle sleep,
// through kit/awake, which runs caffeinate, so that the Mac does not sleep
// from idleness once the lid is closed and sleep is enabled again.
//
// The switch can be remembered across logins. Then the worker keeps where
// it was left in settings.json in the profile's folder, and turns it on
// again when it starts, so a Mac on which both are left on never sleeps.
//
// The flags come from vero.WorkerOptions: -json puts the event stream on
// stdout, -quiet silences the log, and -version is what the front end asks
// before replacing the copy of this that it has on disk. -helper and
// -parent are the helper's, and are never given by hand.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"github.com/imclaren/vero"
	"github.com/imclaren/vero/kit/awake"
	"github.com/imclaren/vero/kit/profile"
)

// version has to increase on every release, so that the front end can tell
// which of two copies is newer. The release sets it.
var version = "0.1.0"

// The phases, which the menu describes in words.
const (
	phaseOff      = "off"      // the Mac sleeps when the lid is closed, as usual
	phaseStarting = "starting" // the password is being asked for, or the helper is starting
	phaseOpen     = "open"     // sleep is disabled, and the lid is open
	phaseClosed   = "closed"   // the lid is closed and the Mac is awake
	phaseStopping = "stopping" // turned off, and the helper is ending
	phaseNoLid    = "nolid"    // this Mac has no lid
)

// Status is the state: everything the menu shows.
type Status struct {
	// On is whether it has been turned on at the menu bar.
	On bool `json:"on"`
	// Phase is one of the phases above.
	Phase string `json:"phase"`
	// LidClosed is whether the lid is closed now.
	LidClosed bool `json:"lidClosed"`
	// SleepDisabled is the system's own SleepDisabled setting.
	SleepDisabled bool `json:"sleepDisabled"`
	// HasLid is whether this Mac has a lid at all.
	HasLid bool `json:"hasLid"`
	// Since is when it was turned on, as HH:MM.
	Since string `json:"since"`
	// Problem is why the last attempt to turn it on failed, in a sentence.
	Problem string `json:"problem"`
	// Remember is whether the switch is remembered across logins: turned
	// on again when the worker starts, if it was on when it last ran.
	Remember bool `json:"remember"`

	// armed is whether sleep has been seen disabled since it was turned on,
	// which is how the worker knows that the password was given.
	armed bool
}

// setPhase works the phase out from the rest.
func (s *Status) setPhase(helperRunning bool) {
	switch {
	case !s.HasLid:
		s.Phase = phaseNoLid
	case s.On && !s.armed:
		s.Phase = phaseStarting
	case s.On && s.LidClosed:
		s.Phase = phaseClosed
	case s.On:
		s.Phase = phaseOpen
	case helperRunning:
		s.Phase = phaseStopping
	default:
		s.Phase = phaseOff
	}
}

// Remember is what the "remember" request carries.
type Remember struct {
	// On is whether to remember the switch across logins.
	On bool `json:"on"`
}

// settings is what the worker keeps between launches, in settings.json in
// the profile's folder: whether the switch is remembered, and where it was
// left.
type settings struct {
	Remember bool `json:"remember"`
	On       bool `json:"on"`
}

// loadSettings reads the settings file, and a missing one is the default.
func loadSettings(file string) settings {
	var st settings
	if data, err := os.ReadFile(file); err == nil {
		json.Unmarshal(data, &st)
	}
	return st
}

// saveSettings writes the settings file.
func saveSettings(file string, st settings) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(data, '\n'), 0o644)
}

// app is the worker: the state, the helper while it runs, and the hold
// against idle sleep.
type app struct {
	w     *vero.Worker
	state *vero.State[Status]
	// flag is the file whose presence keeps the helper going.
	flag string
	// settings is the file that remembers the switch across logins.
	settings string

	mu       sync.Mutex
	starting bool      // a turn on is waiting for the password or starting the helper
	helper   *exec.Cmd // the helper, while it runs
	release  func()    // ends the hold against idle sleep
}

func main() {
	var opts vero.WorkerOptions
	opts.Version = version
	opts.RegisterFlags(flag.CommandLine)
	helperFlag := flag.String("helper", "", "run as the helper, kept going by this file (the worker runs this as root)")
	parent := flag.Int("parent", 0, "the helper's worker, by process id")
	flag.Parse()
	if *helperFlag != "" {
		os.Exit(runHelper(*helperFlag, *parent))
	}
	opts.PrintVersionAndExit()

	w := vero.NewWorker(opts)
	dir, err := profile.Dir("wake")
	if err != nil {
		w.Log("no folder for the flag file: %v", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.Log("could not make %s: %v", dir, err)
		os.Exit(1)
	}
	a := &app{w: w, flag: filepath.Join(dir, "awake"), settings: filepath.Join(dir, "settings.json")}

	hasLid, closed := lid()
	st := loadSettings(a.settings)
	a.state = vero.NewState(w, Status{HasLid: hasLid, LidClosed: closed, SleepDisabled: sleepDisabled(), Remember: st.Remember})
	a.state.Do(func(s *Status) {
		s.setPhase(false)
		if st.Remember && st.On {
			// Left on, and remembered: turn it on again.
			if err := a.turnOn(s); err != nil {
				s.Problem = err.Error()
			}
		}
	})

	// Update handles a request with no accompanying data. "status" returns
	// the state, "turnOn" keeps the Mac awake after its lid is closed, and
	// "turnOff" lets it sleep again. "remember" sets whether the switch is
	// remembered across logins, and takes a Remember.
	vero.Update(a.state, "status", func(*Status) error { return nil })
	vero.Update(a.state, "turnOn", a.turnOn)
	vero.Update(a.state, "turnOff", a.turnOff)
	vero.UpdateWith(a.state, "remember", a.remember)

	done := make(chan struct{})
	go a.watch(done)

	if err := w.Serve(); err != nil {
		w.Log("stopped: %v", err)
	}
	close(done)
	// The flag file is left, on purpose: the helper ends by itself when it
	// sees that this process has gone, without putting the Mac to sleep,
	// which only a turn off at the menu bar does.
	a.mu.Lock()
	if a.release != nil {
		a.release()
	}
	a.mu.Unlock()
}

// turnOn makes the flag file and sets start going. It runs under the
// state's lock, so it does not wait for the password or the helper. A turn
// on while an earlier one is still waiting for the password only puts the
// flag file back, and that start carries on.
func (a *app) turnOn(s *Status) error {
	if !s.HasLid {
		return errors.New("This Mac has no lid to close.")
	}
	if s.On {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.helper != nil {
		return errors.New("It is still turning off. Try again in a moment.")
	}
	if err := os.WriteFile(a.flag, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return fmt.Errorf("The flag file could not be made: %v.", err)
	}
	s.On, s.armed, s.Problem, s.Since = true, false, "", time.Now().Format("15:04")
	s.setPhase(true)
	a.save(s)
	a.w.Log("turned on")
	if !a.starting {
		a.starting = true
		go a.start()
	}
	return nil
}

// start writes the rule for sudo if it is not there yet, which asks for
// the administrator's password, and then starts the helper. It runs
// outside the state's lock, since the password takes as long as the
// person takes. A turn off in the meantime removes the flag file, and then
// no helper is started.
func (a *app) start() {
	fail := func(problem string) {
		os.Remove(a.flag)
		a.state.Do(func(s *Status) {
			if s.On {
				s.Problem = problem
				a.w.Log("could not turn on: %s", problem)
			}
			s.On, s.armed = false, false
			s.setPhase(false)
			a.save(s)
		})
	}
	if !ruleInstalled() {
		if err := installRule(); err != nil {
			a.mu.Lock()
			a.starting = false
			a.mu.Unlock()
			fail(ruleProblem(err))
			return
		}
		a.w.Log("wrote the rule for sudo to %s", ruleFile())
	}
	// turnOn takes a.mu under the state's lock, and fail takes the
	// state's lock, so fail is never called with a.mu held.
	exe, err := os.Executable()
	a.mu.Lock()
	a.starting = false
	if _, statErr := os.Stat(a.flag); statErr != nil {
		// Turned off while the password was being asked for.
		a.mu.Unlock()
		return
	}
	if err != nil {
		a.mu.Unlock()
		fail(fmt.Sprintf("The worker could not find itself: %v.", err))
		return
	}
	cmd, stderr, err := startHelper(exe, a.flag, os.Getpid())
	if err != nil {
		a.mu.Unlock()
		fail(fmt.Sprintf("The helper could not be started: %v.", err))
		return
	}
	a.helper = cmd
	a.release, _ = awake.Hold("wake is keeping this Mac awake")
	a.mu.Unlock()
	go a.waitForHelper(cmd, stderr)
}

// ruleProblem says, in a sentence, why the rule could not be written.
func ruleProblem(err error) string {
	if strings.Contains(err.Error(), "-128") {
		return "The administrator's password was not given."
	}
	return "The rule for sudo could not be written. " + lastLine(err.Error())
}

// turnOff removes the flag file. The helper sees that within half a second
// and ends, putting the Mac to sleep if the lid is closed.
func (a *app) turnOff(s *Status) error {
	if !s.On {
		return nil
	}
	os.Remove(a.flag)
	a.mu.Lock()
	running := a.helper != nil
	a.mu.Unlock()
	s.On, s.armed = false, false
	s.setPhase(running)
	a.save(s)
	a.w.Log("turned off")
	return nil
}

// remember sets whether the switch is remembered across logins, and
// writes the settings file either way, so that a switch that is on when
// it is turned on is remembered as on.
func (a *app) remember(s *Status, r Remember) error {
	s.Remember = r.On
	if err := saveSettings(a.settings, settings{Remember: s.Remember, On: s.On}); err != nil {
		return fmt.Errorf("The settings could not be saved: %v.", err)
	}
	a.w.Log("remember across logins: %v", s.Remember)
	return nil
}

// save writes the settings file after the switch has changed, so that the
// next launch finds where it was left. A failure is logged, since the
// switch itself has worked.
func (a *app) save(s *Status) {
	if err := saveSettings(a.settings, settings{Remember: s.Remember, On: s.On}); err != nil {
		a.w.Log("could not save the settings: %v", err)
	}
}

// waitForHelper waits for the helper to end, which is how a turn off
// completes and how a refused password shows.
func (a *app) waitForHelper(cmd *exec.Cmd, stderr *bytes.Buffer) {
	err := cmd.Wait()
	a.mu.Lock()
	if a.helper == cmd {
		a.helper = nil
	}
	if a.release != nil {
		a.release()
		a.release = nil
	}
	a.mu.Unlock()
	os.Remove(a.flag)
	a.state.Do(func(s *Status) {
		if s.On {
			// It ended without being turned off.
			s.Problem = helperProblem(err, stderr.String())
			a.w.Log("the helper ended: %s", s.Problem)
		}
		s.On, s.armed = false, false
		s.setPhase(false)
		a.save(s)
	})
}

// helperProblem says, in a sentence, why the helper ended by itself.
func helperProblem(err error, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	switch {
	case strings.Contains(stderr, "could not disable sleep"):
		return "Sleep could not be disabled. " + lastLine(stderr)
	case err != nil && stderr != "":
		return "It stopped. " + lastLine(stderr)
	case err != nil:
		return "It stopped: " + err.Error() + "."
	default:
		return "It stopped by itself."
	}
}

// lastLine is the last line of some output, as a sentence.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if line != "" && !strings.HasSuffix(line, ".") {
		line += "."
	}
	return line
}

// watch reads the lid and the sleep setting once a second, outside the
// state's lock, and writes what it finds into the state.
func (a *app) watch(done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		hasLid, closed := lid()
		disabled := sleepDisabled()
		a.mu.Lock()
		running := a.helper != nil
		a.mu.Unlock()
		a.state.Do(func(s *Status) {
			s.HasLid, s.LidClosed, s.SleepDisabled = hasLid, closed, disabled
			if s.On && disabled {
				s.armed = true
			}
			s.setPhase(running)
		})
	}
}
