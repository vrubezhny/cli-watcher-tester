//
// Copyright (c) 2025 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//
// Standalone tester for che-machine-exec's CLI Watcher.
// Runs the watcher in isolation, logging activity ticks to stdout
// instead of stopping a DevWorkspace.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/vrubezhny/cli-watcher-tester/internal/tick"
	"github.com/vrubezhny/cli-watcher-tester/internal/timeout"
)

// envMapping binds a CLI flag to the watcher's environment variable.
// If the user did not pass the flag, the env var is left untouched
// and the watcher uses its own default.
type envMapping struct {
	envName string
	parse   func(string) (string, error)
}

func main() {
	var (
		idleTimeout     time.Duration
		logLevel        string
		enabled         string
		checkPeriod     string
		activityWindow  string
		gracePeriod     string
		maxProcessAge   string
		verbose         string
		config          string
		activitySources string
	)

	// Use a local FlagSet so we control parse-error UX: when an unknown
	// flag is passed, the global flag.CommandLine would print "flag
	// provided but not defined" and then dump usage, which is easy to
	// miss. With ContinueOnError we get a clean error back, prefix it
	// prominently, and exit with status 2.
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.DurationVar(&idleTimeout, "idleTimeout", 30*time.Minute,
		"Idle timeout passed to NewCliWatcher (drives adaptive defaults). "+
			"Mirrors che-machine-exec's default of 30m. Format: Go duration (30m, 1h) or -1 to disable.")
	fs.StringVar(&logLevel, "logLevel", "info",
		"Log level for the tester itself: debug, info, warn, error. "+
			"Independent of --verbose, which controls cli-watcher's activity-detection logs.")

	fs.StringVar(&enabled, "enabled", "",
		"Sets CLI_ACTIVITY_TRACKER_ENABLED. Values: true, false. "+
			"Empty = watcher's own default (currently false).")
	fs.StringVar(&checkPeriod, "checkPeriod", "",
		"Sets CLI_ACTIVITY_TRACKER_CHECK_PERIOD. "+
			"Empty = adaptive default.")
	fs.StringVar(&activityWindow, "activityWindow", "",
		"Sets CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW. "+
			"Empty = adaptive default (idleTimeout - gracePeriod - buffer).")
	fs.StringVar(&gracePeriod, "gracePeriod", "",
		"Sets CLI_ACTIVITY_TRACKER_GRACE_PERIOD. "+
			"Empty = adaptive default.")
	fs.StringVar(&maxProcessAge, "maxProcessAge", "",
		"Sets CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE. "+
			"Empty = 6h default.")
	fs.StringVar(&verbose, "verbose", "",
		"Sets CLI_ACTIVITY_TRACKER_VERBOSE. "+
			"Promotes activity-detection logs from debug to info. "+
			"Independent of --logLevel.")
	fs.StringVar(&config, "config", "",
		"Sets CLI_ACTIVITY_TRACKER_CONFIG (path to .noidle). "+
			"Empty = upward search from $PROJECT_SOURCE / $PROJECTS_ROOT / cwd, then $HOME/.noidle.")
	fs.StringVar(&activitySources, "activitySources", "",
		"Sets CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES. "+
			"Comma-separated list, e.g. 'tty:disabled,codex-app-server'. "+
			"Empty = watcher's default (tty).")

	// Hidden shell-completion entry point (see completions/).
	// Must run after flags are registered so candidates match the FlagSet.
	if len(os.Args) > 1 && os.Args[1] == "__complete" {
		runComplete(fs, os.Args[2:])
	}

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage: %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Standalone tester for che-machine-exec's CLI Watcher.\n\n")
		fmt.Fprintf(os.Stderr, "Each flag (except --idleTimeout, --logLevel) sets the corresponding\n")
		fmt.Fprintf(os.Stderr, "CLI_ACTIVITY_TRACKER_* env var before constructing the watcher.\n")
		fmt.Fprintf(os.Stderr, "Empty flag values mean 'do not set the env var; use watcher's default'.\n\n")
		printFlagsGNU(fs)
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		// flag already printed usage for -h/--help; don't repeat it.
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		// flag already printed the error and usage; don't repeat them.
		os.Exit(2)
	}

	if err := configureLogLevel(logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "invalid --logLevel: %v\n", err)
		os.Exit(2)
	}

	mappings := map[string]envMapping{
		"enabled":         {"CLI_ACTIVITY_TRACKER_ENABLED", parseBool},
		"checkPeriod":     {"CLI_ACTIVITY_TRACKER_CHECK_PERIOD", parseDurationString},
		"activityWindow":  {"CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW", parseDurationString},
		"gracePeriod":     {"CLI_ACTIVITY_TRACKER_GRACE_PERIOD", parseDurationString},
		"maxProcessAge":   {"CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE", parseDurationString},
		"verbose":         {"CLI_ACTIVITY_TRACKER_VERBOSE", parseBool},
		"config":          {"CLI_ACTIVITY_TRACKER_CONFIG", passthrough},
		"activitySources": {"CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES", passthrough},
	}
	fs.Visit(func(f *flag.Flag) {
		m, ok := mappings[f.Name]
		if !ok {
			return
		}
		parsed, err := m.parse(f.Value.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid --%s %q: %v\n", f.Name, f.Value.String(), err)
			os.Exit(2)
		}
		if err := os.Setenv(m.envName, parsed); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set %s: %v\n", m.envName, err)
			os.Exit(2)
		}
		logrus.Infof("tester: %s = %q (from --%s)", m.envName, parsed, f.Name)
	})

	counter := tick.New(idleTimeout, logLevel, snapshotEnv(mappings))
	watcher := timeout.NewCliWatcher(counter.Fire, idleTimeout)
	watcher.Start()
	defer watcher.Stop()

	logrus.Infof("tester: watcher started. idleTimeout=%v, logLevel=%s", idleTimeout, logLevel)
	logrus.Infof("tester: open another terminal in the same TTY and run a process to see ticks. Ctrl-C to exit.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logrus.Infof("tester: received signal, exiting. total ticks reported: %d", counter.Count())
}

func configureLogLevel(level string) error {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logrus.SetLevel(l)
	return nil
}

// flagValueCandidates lists fixed values for flags that take a small
// enum of arguments (used by __complete only).
var flagValueCandidates = map[string][]string{
	"logLevel": {"debug", "info", "warn", "error"},
	"enabled":  {"true", "false"},
	"verbose":  {"true", "false"},
}

// runComplete implements the hidden __complete subcommand for shell
// completion. Contract:
//
//	cli-watcher-tester __complete <word>...
//
// where <word>... is every argument after the command word, including
// the partial word being completed as the last element. Candidates are
// printed one per line on stdout. Always exits 0.
func runComplete(fs *flag.FlagSet, args []string) {
	defer os.Exit(0)

	if len(args) == 0 {
		return
	}
	cur := args[len(args)-1]
	prev := ""
	if len(args) >= 2 {
		prev = args[len(args)-2]
	}

	// Completing --flag=value (only the value part after '=').
	if name, valPrefix, ok := strings.Cut(cur, "="); ok && strings.HasPrefix(name, "-") {
		fname := strings.TrimLeft(name, "-")
		for _, cand := range flagValueCandidates[fname] {
			if strings.HasPrefix(cand, valPrefix) {
				fmt.Printf("%s=%s\n", name, cand)
			}
		}
		return
	}

	// Completing a flag name. Accepts --prefix and also -prefix
	// (e.g. -e → --enabled), since shells often show -x style too.
	if strings.HasPrefix(cur, "-") {
		singleDash := strings.HasPrefix(cur, "-") && !strings.HasPrefix(cur, "--") && len(cur) > 1
		fs.VisitAll(func(f *flag.Flag) {
			long := "--" + f.Name
			if strings.HasPrefix(long, cur) ||
				(singleDash && strings.HasPrefix(f.Name, cur[1:])) {
				fmt.Println(long)
			}
		})
		return
	}

	// Completing a value for the previous flag word (filtered by prefix).
	if prev != "" && strings.HasPrefix(prev, "-") && !strings.Contains(prev, "=") {
		fname := strings.TrimLeft(prev, "-")
		for _, cand := range flagValueCandidates[fname] {
			if cur == "" || strings.HasPrefix(cand, cur) {
				fmt.Println(cand)
			}
		}
	}
}

// printFlagsGNU prints flag defaults with GNU-style --name prefixes,
// wrapping the usage text. It replaces flag.PrintDefaults, which only
// emits single-dash -name.
func printFlagsGNU(fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		var line strings.Builder
		fmt.Fprintf(&line, "  --%s", f.Name)

		isBool := false
		if bv, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
			isBool = true
		}
		if !isBool {
			fmt.Fprintf(&line, " %s", valueTypeName(f))
		}

		if d := f.DefValue; d != "" && d != "false" && d != "0" {
			fmt.Fprintf(&line, " (default %s)", d)
		}

		// Indent wrapped usage lines under the flag header.
		const width = 72
		words := strings.Fields(f.Usage)
		col := 0
		line.WriteString("\n")
		for _, w := range words {
			switch {
			case col == 0:
				line.WriteString("    ")
				col = 4
			case col+1+len(w) > width:
				line.WriteString("\n    ")
				col = 4
			default:
				line.WriteString(" ")
				col++
			}
			line.WriteString(w)
			col += len(w)
		}
		line.WriteString("\n")
		fmt.Fprint(os.Stderr, line.String())
	})
}

// valueTypeName derives a flag.Value type name (e.g. "duration",
// "string") from the concrete type, mirroring flag.UnquoteUsage.
func valueTypeName(f *flag.Flag) string {
	t := reflect.TypeOf(f.Value)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return strings.TrimSuffix(strings.ToLower(t.Name()), "value")
}

// snapshotEnv records the current values of CLI_ACTIVITY_TRACKER_* env vars
// so the tick log line can show what config the watcher is actually using.
func snapshotEnv(mappings map[string]envMapping) map[string]string {
	out := make(map[string]string, len(mappings))
	for _, m := range mappings {
		if v, ok := os.LookupEnv(m.envName); ok {
			out[m.envName] = v
		}
	}
	return out
}

func passthrough(s string) (string, error) { return s, nil }

func parseBool(s string) (string, error) {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return "", fmt.Errorf("expected boolean, got %q", s)
	}
	return strconv.FormatBool(b), nil
}

// parseDurationString normalizes a Go duration or bare-seconds integer
// to a Go duration string, matching what cli-watcher's parseDuration expects.
func parseDurationString(s string) (string, error) {
	if _, err := time.ParseDuration(s); err == nil {
		return s, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return (time.Duration(n) * time.Second).String(), nil
	}
	return "", fmt.Errorf("expected Go duration (30s, 5m) or positive integer seconds, got %q", s)
}
