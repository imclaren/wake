package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// The Mac's power management, through the programs Apple ships: ioreg for
// the lid, and pmset for the SleepDisabled setting and for sleeping now.
// pmset's disablesleep and sleepnow need root, and the helper runs them
// through sudo, which the rule below lets it do without a password. The
// rest needs nothing, and the worker reads them to show what is going on.

// ruleFile is where the rule goes. sudo reads every file in the folder
// whose name has no dot in it, and the file is this account's own, by
// its user id, so that each account that turns wake on is asked once.
func ruleFile() string {
	return "/private/etc/sudoers.d/wake-" + strconv.Itoa(os.Getuid())
}

// rule is what the file says: that this account may run the three pmset
// commands the helper needs, and nothing else, without a password.
func rule(username string) string {
	return "# Written by wake, so that it can keep this Mac awake after its lid is closed\n" +
		"# without asking for an administrator's password each time. Remove this file to undo it.\n" +
		username + " ALL=(root) NOPASSWD: /usr/bin/pmset -a disablesleep 1, /usr/bin/pmset -a disablesleep 0, /usr/bin/pmset sleepnow\n"
}

// ruleInstalled says whether sudo lets this account run pmset's
// disablesleep without a password, which is what the rule gives.
func ruleInstalled() bool {
	return exec.Command("/usr/bin/sudo", "-n", "-l", "/usr/bin/pmset", "-a", "disablesleep", "1").Run() == nil
}

// installRule writes the rule, as root, through osascript, which shows the
// system's own dialog for the administrator's password. The rule goes to a
// file with a dot in its name first, which sudo ignores, is checked by
// visudo, and is moved into place only if it parses. The rule's text goes
// to the script as an argument, and quoted form keeps it whole.
func installRule() error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	file := ruleFile()
	if err := writeRule(file, rule(u.Username), true); err != nil {
		return err
	}
	if !ruleInstalled() {
		return fmt.Errorf("the rule was written to %s, but sudo does not follow it", file)
	}
	return nil
}

// writeRule writes a rule to a file through osascript, as root when asRoot
// is set, which is the only way it works for the real file. The tests
// write to a folder of their own as the user.
func writeRule(file, text string, asRoot bool) error {
	dir := filepath.Dir(file)
	script := `set f to quoted form of ` + strconv.Quote(file) + "\n" +
		`do shell script "umask 077; mkdir -p " & quoted form of ` + strconv.Quote(dir) +
		` & " && printf '%s' " & quoted form of item 1 of argv & " > " & f & ".tmp && chmod 0440 " & f & ".tmp && /usr/sbin/visudo -cf " & f & ".tmp && mv " & f & ".tmp " & f`
	if asRoot {
		script += " with administrator privileges"
	}
	cmd := exec.Command("/usr/bin/osascript",
		"-e", "on run argv",
		"-e", script,
		"-e", "end run",
		text)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)) + " (" + err.Error() + ")")
	}
	return nil
}

// lid says whether this Mac has a lid, and whether it is closed: the
// AppleClamshellState property of IOPMrootDomain, which a Mac without a
// lid does not have.
func lid() (hasLid, closed bool) {
	out, err := exec.Command("/usr/sbin/ioreg", "-r", "-k", "AppleClamshellState", "-d", "4").Output()
	if err != nil {
		return false, false
	}
	return parseLid(string(out))
}

func parseLid(out string) (hasLid, closed bool) {
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.Contains(key, `"AppleClamshellState"`) {
			continue
		}
		return true, strings.TrimSpace(value) == "Yes"
	}
	return false, false
}

// sleepDisabled is the system's SleepDisabled setting, which pmset -g
// lists only when it is set.
func sleepDisabled() bool {
	out, err := exec.Command("/usr/bin/pmset", "-g").Output()
	if err != nil {
		return false
	}
	return parseSleepDisabled(string(out))
}

func parseSleepDisabled(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "SleepDisabled" {
			return fields[1] == "1"
		}
	}
	return false
}

// setSleepDisabled sets SleepDisabled, as root through the rule.
func setSleepDisabled(disabled bool) error {
	value := "0"
	if disabled {
		value = "1"
	}
	return pmset("-a", "disablesleep", value)
}

// sleepNow puts the Mac to sleep, as root through the rule.
func sleepNow() error {
	return pmset("sleepnow")
}

// pmset runs pmset as root through sudo, which the rule lets it do without
// a password. sudo is told not to ask for one, so that a missing rule
// fails at once instead of waiting for a password that nobody can give.
func pmset(args ...string) error {
	args = append([]string{"-n", "/usr/bin/pmset"}, args...)
	out, err := exec.Command("/usr/bin/sudo", args...).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)) + " (" + err.Error() + ")")
	}
	return nil
}

// processAlive says whether a process is still there. The helper runs as
// the same account as the worker, so it may signal it, and only a process
// that has gone fails.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// startHelper runs the helper, which is this program again, as this
// account. The command it returns is the helper's life.
func startHelper(exe, flag string, parent int) (*exec.Cmd, *bytes.Buffer, error) {
	cmd := exec.Command(exe, "-helper", flag, "-parent", strconv.Itoa(parent))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, &stderr, nil
}

// holdAwake holds off idle sleep, and display sleep as well when display is
// set, through caffeinate, which any account may run. It ends when the
// function it returns is called, or when this process does, which -w
// watches.
func holdAwake(display bool) (func(), error) {
	args := []string{"-i"}
	if display {
		args = append(args, "-d")
	}
	args = append(args, "-w", strconv.Itoa(os.Getpid()))
	cmd := exec.Command("/usr/bin/caffeinate", args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			cmd.Process.Kill()
			cmd.Wait()
		})
	}, nil
}
