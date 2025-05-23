package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"

	"github.com/spf13/pflag"
)

var (
	profileName   string
	profileOutput string
)

func addProfilingFlags(flags *pflag.FlagSet) {
	flags.StringVar(&profileName, "profile", "none", "Name of profile to capture. One of (none|cpu|heap|goroutine|threadcreate|block|mutex)")
	flags.StringVar(&profileOutput, "profile-output", "profile.pprof", "Name of the file to write the profile to")
}

func initProfiling() error {
	var (
		f   *os.File
		err error
	)
	switch profileName {
	case "none":
		return nil
	case "cpu":
		f, err = os.Create(profileOutput)
		if err != nil {
			return err
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			return err
		}
	// Block and mutex profiles need a call to Set{Block,Mutex}ProfileRate to
	// output anything. We choose to sample all events.
	case "block":
		runtime.SetBlockProfileRate(1)
	case "mutex":
		runtime.SetMutexProfileFraction(1)
	default:
		// Check the profile name is valid.
		if profile := pprof.Lookup(profileName); profile == nil {
			return fmt.Errorf("unknown profile '%s'", profileName)
		}
	}

	// If the command is interrupted before the end (ctrl-c), flush the
	// profiling files
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		err := f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error closing profile file: %v\n", err)
		}
		err = flushProfiling()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing profile file: %v\n", err)
		}
		os.Exit(0)
	}()

	return nil
}

func flushProfiling() error {
	switch profileName {
	case "none":
		return nil
	case "cpu":
		pprof.StopCPUProfile()
	case "heap":
		runtime.GC()
		fallthrough
	default:
		profile := pprof.Lookup(profileName)
		if profile == nil {
			return nil
		}
		f, err := os.Create(profileOutput)
		if err != nil {
			return err
		}
		defer func() {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Error closing profile file: %v\n", err)
			}
		}()
		err = profile.WriteTo(f, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing profile file: %v\n", err)
		}
	}

	return nil
}
