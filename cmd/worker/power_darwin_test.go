package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseLid checks ioreg's output for a closed lid, an open one, and a
// Mac without one.
func TestParseLid(t *testing.T) {
	open := "+-o IOPMrootDomain  <class IOPMrootDomain>\n    {\n      \"AppleClamshellState\" = No\n      \"AppleClamshellCausesSleep\" = Yes\n    }\n"
	closed := "    {\n  |   \"AppleClamshellState\" = Yes\n    }\n"
	for _, c := range []struct {
		out            string
		hasLid, closed bool
	}{{open, true, false}, {closed, true, true}, {"", false, false}} {
		hasLid, isClosed := parseLid(c.out)
		if hasLid != c.hasLid || isClosed != c.closed {
			t.Errorf("parseLid(%q) = %v, %v, want %v, %v", c.out, hasLid, isClosed, c.hasLid, c.closed)
		}
	}
}

// TestParseSleepDisabled checks pmset -g's output with the setting and
// without it, which is how pmset says it is off.
func TestParseSleepDisabled(t *testing.T) {
	with := "System-wide power settings:\n SleepDisabled\t\t1\nCurrently in use:\n standby              1\n"
	without := "System-wide power settings:\nCurrently in use:\n sleep                1\n"
	if !parseSleepDisabled(with) {
		t.Error("SleepDisabled 1 was not seen")
	}
	if parseSleepDisabled(without) {
		t.Error("SleepDisabled was seen where there is none")
	}
}

// TestRule checks that the rule parses, with visudo, and names only pmset.
func TestRule(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/visudo"); err != nil {
		t.Skip("no visudo")
	}
	file := filepath.Join(t.TempDir(), "wake")
	if err := os.WriteFile(file, []byte(rule("someone")), 0o440); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/usr/sbin/visudo", "-cf", file).CombinedOutput()
	if err != nil {
		t.Fatalf("visudo: %v\n%s", err, out)
	}
	for _, line := range strings.Split(rule("someone"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, command := range strings.Split(strings.SplitN(line, "NOPASSWD: ", 2)[1], ", ") {
			if !strings.HasPrefix(command, "/usr/bin/pmset ") {
				t.Errorf("the rule allows %q, which is not pmset", command)
			}
		}
	}
}

// TestWriteRule writes a rule to a folder of its own, as the user, and a
// rule that does not parse, which visudo must refuse. The rule's text goes
// through osascript and the shell on its way, and has to come out whole.
func TestWriteRule(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/visudo"); err != nil {
		t.Skip("no visudo")
	}
	file := filepath.Join(t.TempDir(), "sudoers.d", "wake-501")
	text := rule("someone")
	if err := writeRule(file, text, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != text {
		t.Errorf("the file says %q, want %q", got, text)
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0o440 {
		t.Errorf("mode %v, want 0440", info.Mode().Perm())
	}
	bad := filepath.Join(filepath.Dir(file), "bad")
	if err := writeRule(bad, "someone ALL=(root NOPASSWD: /usr/bin/pmset\n", false); err == nil {
		t.Error("a rule that does not parse was written")
	}
	if _, err := os.Stat(bad); err == nil {
		t.Error("the bad rule was moved into place")
	}
}
