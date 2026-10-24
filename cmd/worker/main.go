// Command worker is wake's worker. It keeps a Mac awake, in one of two
// modes, and the menu bar turns that on and off.
//
// In the "open" mode, the default, it keeps the Mac awake while its lid is
// open and lets it sleep when the lid closes, as usual. That needs nothing
// but caffeinate, which Apple ships and which any account may run.
//
// In the "closed" mode it keeps a MacBook awake after its lid is closed,
// as caffeinate+ does (github.com/clashman/caffeinate-plus). The setting
// that stops a closing lid putting the Mac to sleep, SleepDisabled, may be
// changed only by an administrator. The first time this mode is turned on,
// the worker writes a rule for sudo, through osascript's "with
// administrator privileges", which asks for the administrator's password
// once. The rule lets this account run the three pmset commands that
// change the setting and put the Mac to sleep, and nothing else, without a
// password, and it stays until the file is removed. Then the worker runs a
// second copy of itself, the helper in helper.go, which disables sleep
// while the lid is open and enables it again as soon as the lid closes.
// That is caffeinate+'s trick: closing the lid is what puts a Mac to
// sleep, and once the lid is closed the Mac stays awake. The helper ends
// when the file the worker made for it goes away, or when the worker does.
// If the lid is closed when it is turned off, it puts the Mac to sleep, as
// caffeinate+ does when it is stopped. When the mode changes to "open"
// while it runs, the worker writes "handover" into the file instead, and
// the helper ends without putting the Mac to sleep.
//
// In both modes the worker holds an assertion against idle sleep, through
// caffeinate, and against display sleep as well when the display is to stay
// on. A timer can turn it off after a number of minutes.
//
// The switch can be remembered across logins. Then the worker keeps where
// it was left in settings.json in the profile's folder, with the mode and
// the display choice, and turns it on again when it starts, without the
// timer, so a Mac on which both are left on never sleeps.
//
// The flags come from vero.WorkerOptions: -json puts the event stream on
// stdout, -quiet silences the log, and -version is what the front end asks
// before replacing the copy of this that it has on disk. -helper and
// -parent are the helper's, and are never given by hand.
package main

import (
	"bytes"
	"encoding/json"
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

	"github.com/imclaren/vero"
	"github.com/imclaren/vero/kit/profile"
)

// version has to increase on every release, so that the front end can tell
// which of two copies is newer. The release sets it.
var version = "0.1.0"

// The modes.
const (
	modeOpen   = "open"   // awake while the lid is open, asleep when it closes
	modeClosed = "closed" // awake even after the lid is closed
)

// longest is the longest timer, in minutes.
const longest = 24 * 60

// The phases, which the menu describes in words.
const (
	phaseOff      = "off"      // the Mac sleeps as usual
	phaseAwake    = "awake"    // on in the open mode: awake while the lid is open
	phaseStarting = "starting" // the closed mode: the password is being asked for, or the helper is starting
	phaseOpen     = "open"     // the closed mode: sleep is disabled, and the lid is open
	phaseClosed   = "closed"   // the closed mode: the lid is closed and the Mac is awake
	phaseStopping = "stopping" // turned off, and the helper is ending
	phaseNoLid    = "nolid"    // off, on a Mac that has no lid
)

// Status is the state: everything the menu shows.
type Status struct {
	// On is whether it has been turned on at the menu bar.
	On bool `json:"on"`
	// Phase is one of the phases above.
	Phase string `json:"phase"`
	// Mode is "open", to keep the Mac awake while its lid is open, or
	// "closed", to keep it awake even after the lid is closed.
	Mode string `json:"mode"`
	// Display is whether the display is kept on as well.
	Display bool `json:"display"`
	// Minutes is how long it stays on once turned on, and 0 is until it is
	// turned off.
	Minutes int `json:"minutes"`
	// Until is when the timer turns it off, as HH:MM, and empty without one.
	Until string `json:"until"`
	// LidClosed is whether the lid is closed now.
	LidClosed bool `json:"lidClosed"`
	// SleepDisabled is the system's own SleepDisabled setting.
	SleepDisabled bool `json:"sleepDisabled"`
	// HasLid is whether this Mac has a lid at all.
	HasLid bool `json:"hasLid"`
	// RuleInstalled is whether the rule for sudo is in place, so that the
	// closed mode can be turned on without asking for a password.
	RuleInstalled bool `json:"ruleInstalled"`
	// Since is when it was turned on, as HH:MM.
	Since string `json:"since"`
	// Problem is why the last attempt to turn it on failed, in a sentence.
	Problem string `json:"problem"`
	// Remember is whether the switch is remembered across logins: turned
	// on again when the worker starts, if it was on when it last ran.
	Remember bool `json:"remember"`

	// armed is whether it is fully on: in the closed mode, whether sleep has
	// been seen disabled since it was turned on, which is how the worker
	// knows that the password was given.
	armed bool
}

// setPhase works the phase out from the rest.
func (s *Status) setPhase(helperRunning bool) {
	switch {
	case s.On && s.Mode != modeClosed:
		s.Phase = phaseAwake
	case s.On && !s.armed:
		s.Phase = phaseStarting
	case s.On && s.LidClosed:
		s.Phase = phaseClosed
	case s.On:
		s.Phase = phaseOpen
	case helperRunning:
		s.Phase = phaseStopping
	case !s.HasLid:
		s.Phase = phaseNoLid
	default:
		s.Phase = phaseOff
	}
}

// Remember is what the "remember" request carries.
type Remember struct {
	// On is whether to remember the switch across logins.
	On bool `json:"on"`
}

// Options is what the "setOptions" request carries. When it is on, a
// change of mode takes effect at once, and a change of minutes starts the
// timer again from now.
type Options struct {
	// Mode is "open" or "closed".
	Mode string `json:"mode"`
	// Display is whether to keep the display on as well.
	Display bool `json:"display"`
	// Minutes is how long to stay on, and 0 is until it is turned off.
	Minutes int `json:"minutes"`
}

// settings is what the worker keeps between launches, in settings.json in
// the profile's folder.
type settings struct {
	Remember bool   `json:"remember"`
	On       bool   `json:"on"`
	Mode     string `json:"mode,omitempty"`
	Display  bool   `json:"display,omitempty"`
	Minutes  int    `json:"minutes,omitempty"`
}

// loadSettings reads the settings file. A missing or broken one is the
// default, the open mode. A file with no mode was written by a version that
// had only the closed mode, so that is its mode.
func loadSettings(file string) settings {
	st := settings{Mode: modeOpen}
	data, err := os.ReadFile(file)
	if err != nil {
		return st
	}
	var read settings
	if json.Unmarshal(data, &read) != nil {
		return st
	}
	switch read.Mode {
	case "":
		read.Mode = modeClosed
	case modeOpen, modeClosed:
	default:
		read.Mode = modeOpen
	}
	if read.Minutes < 0 || read.Minutes > longest {
		read.Minutes = 0
	}
	return read
}

// saveSettings writes the settings file.
func saveSettings(file string, st settings) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(data, '\n'), 0o644)
}

// settingsOf is what the settings file keeps of the state.
func settingsOf(s *Status) settings {
	return settings{Remember: s.Remember, On: s.On, Mode: s.Mode, Display: s.Display, Minutes: s.Minutes}
}

// app is the worker: the state, the helper while it runs, the hold against
// sleep, and the timer.
type app struct {
	w     *vero.Worker
	state *vero.State[Status]
	// flag is the file whose presence keeps the helper going.
	flag string
	// settings is the file that remembers the switch across logins.
	settings string

	// Under the state's lock.
	timer    *time.Timer
	timerGen int // which timer is current, so that one stopped too late does nothing

	// Under mu, which is taken inside the state's lock and never around it.
	mu          sync.Mutex
	starting    bool      // a turn on of the closed mode is waiting for the password or starting the helper
	switching   bool      // that turn on is a change from the open mode, which it falls back to if it fails
	handingOver bool      // the helper was told to end without putting the Mac to sleep
	helper      *exec.Cmd // the helper, while it runs
	display     bool      // whether the hold keeps the display on
	release     func()    // ends the hold against sleep
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
	if !hasLid {
		st.Mode = modeOpen
	}
	a.display = st.Display
	a.state = vero.NewState(w, Status{
		HasLid: hasLid, LidClosed: closed, SleepDisabled: sleepDisabled(), RuleInstalled: ruleInstalled(),
		Remember: st.Remember, Mode: st.Mode, Display: st.Display, Minutes: st.Minutes,
	})
	a.state.Do(func(s *Status) {
		s.setPhase(false)
		if st.Remember && st.On {
			// Left on, and remembered: turn it on again, without the timer.
			if err := a.turnOn(s); err != nil {
				s.Problem = err.Error()
			}
			a.stopTimer(s)
		}
	})

	// Update handles a request with no accompanying data. "status" returns
	// the state, "turnOn" keeps the Mac awake in the chosen mode, and
	// "turnOff" lets it sleep again. "setOptions" sets the mode, the display
	// and the timer, and takes an Options. "remember" sets whether the switch
	// is remembered across logins, and takes a Remember.
	vero.Update(a.state, "status", func(*Status) error { return nil })
	vero.Update(a.state, "turnOn", a.turnOn)
	vero.Update(a.state, "turnOff", a.turnOff)
	vero.UpdateWith(a.state, "setOptions", a.setOptions)
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
	a.dropHoldLocked()
	a.mu.Unlock()
}

// noLid is the refusal of the closed mode on a Mac without a lid.
var noLid = errors.New("This Mac has no lid, so it can be kept awake only while it is open.")

// turnOn keeps the Mac awake in the chosen mode. The open mode is on at
// once. The closed mode makes the flag file and sets start going, which may
// wait for the password. It runs under the state's lock.
func (a *app) turnOn(s *Status) error {
	if s.On {
		return nil
	}
	if s.Mode == modeClosed && !s.HasLid {
		return noLid
	}
	a.mu.Lock()
	if s.Mode == modeOpen {
		err := a.setHoldLocked()
		running := a.helper != nil
		a.mu.Unlock()
		if err != nil {
			return fmt.Errorf("The Mac could not be kept awake: %v.", err)
		}
		s.On, s.armed, s.Problem, s.Since = true, true, "", time.Now().Format("15:04")
		s.setPhase(running)
		a.armTimer(s)
		a.save(s)
		a.w.Log("turned on, while the lid is open")
		return nil
	}
	if a.helper != nil {
		a.mu.Unlock()
		return errors.New("It is still turning off. Try again in a moment.")
	}
	err := a.startClosedLocked(s)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	s.On, s.Since = true, time.Now().Format("15:04")
	s.setPhase(true)
	a.armTimer(s)
	a.save(s)
	a.w.Log("turned on, even with the lid closed")
	return nil
}

// startClosedLocked makes the flag file and sets start going, under a.mu.
// A turn on while an earlier one is still waiting for the password only
// puts the flag file back, and that start carries on.
func (a *app) startClosedLocked(s *Status) error {
	if err := os.WriteFile(a.flag, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return fmt.Errorf("The flag file could not be made: %v.", err)
	}
	s.armed, s.Problem = false, ""
	if !a.starting {
		a.starting = true
		go a.start()
	}
	return nil
}

// start writes the rule for sudo if it is not there yet, which asks for
// the administrator's password, and then starts the helper. It runs
// outside the state's lock, since the password takes as long as the
// person takes. A turn off, or a change to the open mode, in the meantime
// removes the flag file, and then no helper is started.
func (a *app) start() {
	if !ruleInstalled() {
		if err := installRule(); err != nil {
			a.mu.Lock()
			a.starting = false
			a.mu.Unlock()
			a.fail(ruleProblem(err))
			return
		}
		a.w.Log("wrote the rule for sudo to %s", ruleFile())
		a.state.Do(func(s *Status) { s.RuleInstalled = true })
	}
	exe, err := os.Executable()
	a.mu.Lock()
	a.starting = false
	if _, statErr := os.Stat(a.flag); statErr != nil {
		// Turned off, or changed to the open mode, while the password was
		// being asked for.
		a.switching = false
		a.mu.Unlock()
		return
	}
	if err != nil {
		a.mu.Unlock()
		a.fail(fmt.Sprintf("The worker could not find itself: %v.", err))
		return
	}
	cmd, stderr, err := startHelper(exe, a.flag, os.Getpid())
	if err != nil {
		a.mu.Unlock()
		a.fail(fmt.Sprintf("The helper could not be started: %v.", err))
		return
	}
	a.helper, a.switching = cmd, false
	if err := a.setHoldLocked(); err != nil {
		a.w.Log("could not hold off idle sleep: %v", err)
	}
	a.mu.Unlock()
	go a.waitForHelper(cmd, stderr)
}

// fail undoes a turn on of the closed mode that did not complete, and says
// why. A change from the open mode falls back to it, still on. A.mu must
// not be held.
func (a *app) fail(problem string) {
	a.state.Do(func(s *Status) {
		a.mu.Lock()
		switching := a.switching
		a.switching = false
		running := a.helper != nil
		if !s.On || s.Mode != modeClosed {
			// Turned off, or changed to the open mode, while it waited.
			a.mu.Unlock()
			s.setPhase(running)
			return
		}
		os.Remove(a.flag)
		s.Problem = problem
		a.w.Log("could not turn on: %s", problem)
		if switching {
			s.Mode, s.armed = modeOpen, true
		} else {
			a.dropHoldLocked()
			s.On, s.armed = false, false
			a.stopTimer(s)
		}
		a.mu.Unlock()
		s.setPhase(running)
		a.save(s)
	})
}

// ruleProblem says, in a sentence, why the rule could not be written.
func ruleProblem(err error) string {
	if strings.Contains(err.Error(), "-128") {
		return "The administrator's password was not given."
	}
	return "The rule for sudo could not be written. " + lastLine(err.Error())
}

// turnOff lets the Mac sleep again. In the open mode the hold ends at once.
// In the closed mode the flag file goes, and the helper sees that within
// half a second and ends, putting the Mac to sleep if the lid is closed.
func (a *app) turnOff(s *Status) error {
	if !s.On {
		return nil
	}
	a.stopTimer(s)
	a.mu.Lock()
	if s.Mode == modeOpen {
		a.dropHoldLocked()
	} else {
		os.Remove(a.flag)
	}
	running := a.helper != nil
	a.mu.Unlock()
	s.On, s.armed = false, false
	s.setPhase(running)
	a.save(s)
	a.w.Log("turned off")
	return nil
}

// setOptions sets the mode, the display and the timer. When it is on, each
// change takes effect at once: the display through a new hold, the timer
// from now, and the mode through a handover to the open mode or a start of
// the closed one, which may ask for the password.
func (a *app) setOptions(s *Status, o Options) error {
	if o.Mode != modeOpen && o.Mode != modeClosed {
		return fmt.Errorf("There is no mode called %q.", o.Mode)
	}
	if o.Mode == modeClosed && !s.HasLid {
		return noLid
	}
	if o.Minutes < 0 || o.Minutes > longest {
		return fmt.Errorf("The timer can be up to %d hours.", longest/60)
	}
	minutesChanged := o.Minutes != s.Minutes
	s.Display, s.Minutes = o.Display, o.Minutes
	a.mu.Lock()
	displayChanged := o.Display != a.display
	a.display = o.Display
	var err error
	if s.On && displayChanged && a.release != nil {
		if herr := a.setHoldLocked(); herr != nil {
			a.w.Log("could not change the hold: %v", herr)
		}
	}
	if s.On && o.Mode != s.Mode {
		switch o.Mode {
		case modeOpen:
			// Held awake from now, and the helper, if it has started, ends
			// without putting the Mac to sleep. One still waiting for the
			// password is not started at all.
			if herr := a.setHoldLocked(); herr != nil {
				err = fmt.Errorf("The Mac could not be kept awake: %v.", herr)
			}
			if a.helper != nil {
				a.handingOver = true
				os.WriteFile(a.flag, []byte("handover\n"), 0o644)
			} else {
				os.Remove(a.flag)
			}
			a.switching = false
			s.Mode, s.armed = modeOpen, true
			a.w.Log("changed to awake while the lid is open")
		case modeClosed:
			if a.helper != nil {
				err = errors.New("It is still changing over. Try again in a moment.")
				break
			}
			s.Mode = modeClosed
			if err = a.startClosedLocked(s); err != nil {
				s.Mode = modeOpen
				break
			}
			a.switching = true
			a.w.Log("changed to awake even with the lid closed")
		}
	} else if err == nil {
		s.Mode = o.Mode
	}
	running := a.helper != nil
	a.mu.Unlock()
	if minutesChanged {
		a.armTimer(s)
	}
	s.setPhase(running)
	a.save(s)
	return err
}

// remember sets whether the switch is remembered across logins, and
// writes the settings file either way, so that a switch that is on when
// it is turned on is remembered as on.
func (a *app) remember(s *Status, r Remember) error {
	s.Remember = r.On
	if err := saveSettings(a.settings, settingsOf(s)); err != nil {
		return fmt.Errorf("The settings could not be saved: %v.", err)
	}
	a.w.Log("remember across logins: %v", s.Remember)
	return nil
}

// save writes the settings file after the switch has changed, so that the
// next launch finds where it was left. A failure is logged, since the
// switch itself has worked.
func (a *app) save(s *Status) {
	if err := saveSettings(a.settings, settingsOf(s)); err != nil {
		a.w.Log("could not save the settings: %v", err)
	}
}

// setHoldLocked holds off sleep, and display sleep when a.display says so,
// replacing any hold there was. The new one starts before the old one
// ends, so that there is no moment without one. Under a.mu.
func (a *app) setHoldLocked() error {
	release, err := holdAwake(a.display)
	if err != nil {
		return err
	}
	a.dropHoldLocked()
	a.release = release
	return nil
}

// dropHoldLocked ends the hold, if there is one. Under a.mu.
func (a *app) dropHoldLocked() {
	if a.release != nil {
		a.release()
		a.release = nil
	}
}

// armTimer starts the timer from now, if it is on and has one, and stops
// any timer there was. Under the state's lock.
func (a *app) armTimer(s *Status) {
	a.stopTimer(s)
	if !s.On || s.Minutes <= 0 {
		return
	}
	d := time.Duration(s.Minutes) * time.Minute
	s.Until = time.Now().Add(d).Format("15:04")
	gen := a.timerGen
	a.timer = time.AfterFunc(d, func() {
		a.state.Do(func(s *Status) {
			if a.timerGen != gen || !s.On {
				return
			}
			a.w.Log("the timer ran out")
			a.turnOff(s)
		})
	})
}

// stopTimer stops the timer. Under the state's lock.
func (a *app) stopTimer(s *Status) {
	a.timerGen++
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	s.Until = ""
}

// waitForHelper waits for the helper to end, which is how a turn off or a
// handover completes and how a refused password shows.
func (a *app) waitForHelper(cmd *exec.Cmd, stderr *bytes.Buffer) {
	err := cmd.Wait()
	a.state.Do(func(s *Status) {
		a.mu.Lock()
		if a.helper == cmd {
			a.helper = nil
			// Removed here, under a.mu, so that it cannot remove the flag
			// file of a turn on that came after.
			os.Remove(a.flag)
		}
		handover := a.handingOver
		a.handingOver = false
		if handover || (s.On && s.Mode == modeOpen) {
			// Handed over to the open mode, which keeps its hold.
			a.mu.Unlock()
			s.setPhase(false)
			return
		}
		a.dropHoldLocked()
		a.mu.Unlock()
		if s.On {
			// It ended without being turned off.
			s.Problem = helperProblem(err, stderr.String())
			a.w.Log("the helper ended: %s", s.Problem)
			a.stopTimer(s)
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
			if s.On && s.Mode == modeClosed && disabled {
				s.armed = true
			}
			s.setPhase(running)
		})
	}
}
