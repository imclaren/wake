package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// runHelper is the copy of the worker that runs as root. It is
// caffeinate+'s main loop: it disables sleep, waits for the lid to close,
// and enables sleep again the moment it does, which leaves the Mac awake
// with its lid closed. Unlike caffeinate+, it carries on after that: if
// the lid is opened again, sleep is disabled again, until it is turned off.
//
// It ends when the flag file goes, which is a turn off at the menu bar,
// when the worker that started it has gone, or on SIGINT or SIGTERM. On
// the way out it enables sleep if it had disabled it; and if the lid is
// closed and it was turned off, it puts the Mac to sleep, as caffeinate+
// does, since that is what closing the lid would have done.
func runHelper(flag string, parent int) int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	if err := setSleepDisabled(true); err != nil {
		fmt.Fprintf(os.Stderr, "could not disable sleep: %v\n", err)
		return 1
	}

	turnedOff := false
loop:
	for {
		select {
		case <-signals:
			break loop
		case <-time.After(500 * time.Millisecond):
		}
		if _, err := os.Stat(flag); err != nil {
			turnedOff = true
			break
		}
		if parent > 0 && !processAlive(parent) {
			break
		}
		_, closed := lid()
		switch disabled := sleepDisabled(); {
		case closed && disabled:
			// Sleep can be enabled again as soon as the lid is closed.
			setSleepDisabled(false)
		case !closed && !disabled:
			setSleepDisabled(true)
		}
	}

	if sleepDisabled() {
		if err := setSleepDisabled(false); err != nil {
			fmt.Fprintf(os.Stderr, "could not enable sleep again: %v\n", err)
			return 1
		}
	} else if _, closed := lid(); turnedOff && closed {
		if err := sleepNow(); err != nil {
			fmt.Fprintf(os.Stderr, "could not put the Mac to sleep: %v\n", err)
			return 1
		}
	}
	return 0
}
