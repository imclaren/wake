//go:build !darwin

package main

import (
	"bytes"
	"errors"
	"os/exec"
)

// Only a Mac is kept awake. Elsewhere the worker builds and runs, says
// that there is no lid, and refuses to turn on.

var errUnsupported = errors.New("only a Mac can be kept awake with its lid closed")

func lid() (hasLid, closed bool)  { return false, false }
func sleepDisabled() bool         { return false }
func setSleepDisabled(bool) error { return errUnsupported }
func sleepNow() error             { return errUnsupported }
func processAlive(int) bool       { return false }
func startHelper(string, string, int) (*exec.Cmd, *bytes.Buffer, error) {
	return nil, nil, errUnsupported
}
func holdAwake(bool) (func(), error) { return nil, errUnsupported }
func ruleInstalled() bool            { return false }
func installRule() error             { return errUnsupported }
func ruleFile() string               { return "" }
