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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
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
		idleTimeout    time.Duration
		logLevel       string
		enabled        string
		checkPeriod    string
		activityWindow string
		gracePeriod    string
		maxProcessAge  string
		verbose        string
		config         string
	)

	flag.DurationVar(&idleTimeout, "idleTimeout", 30*time.Minute,
		"Idle timeout passed to NewCliWatcher (drives adaptive defaults). "+
			"Mirrors che-machine-exec's default of 30m. Format: Go duration (30m, 1h) or -1 to disable.")
	flag.StringVar(&logLevel, "logLevel", "info",
		"Log level for the tester itself: debug, info, warn, error. "+
			"Independent of --verbose, which controls cli-watcher's activity-detection logs.")

	flag.StringVar(&enabled, "enabled", "",
		"Sets CLI_ACTIVITY_TRACKER_ENABLED. Values: true, false. "+
			"Empty = watcher's own default (currently false).")
	flag.StringVar(&checkPeriod, "checkPeriod", "",
		"Sets CLI_ACTIVITY_TRACKER_CHECK_PERIOD. "+
			"Empty = adaptive default.")
	flag.StringVar(&activityWindow, "activityWindow", "",
		"Sets CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW. "+
			"Empty = adaptive default (idleTimeout - gracePeriod - buffer).")
	flag.StringVar(&gracePeriod, "gracePeriod", "",
		"Sets CLI_ACTIVITY_TRACKER_GRACE_PERIOD. "+
			"Empty = adaptive default.")
	flag.StringVar(&maxProcessAge, "maxProcessAge", "",
		"Sets CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE. "+
			"Empty = 6h default.")
	flag.StringVar(&verbose, "verbose", "",
		"Sets CLI_ACTIVITY_TRACKER_VERBOSE. "+
			"Promotes activity-detection logs from debug to info. "+
			"Independent of --logLevel.")
	flag.StringVar(&config, "config", "",
		"Sets CLI_ACTIVITY_TRACKER_CONFIG (path to .noidle). "+
			"Empty = upward search from $PROJECT_SOURCE / $PROJECTS_ROOT / cwd, then $HOME/.noidle.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Standalone tester for che-machine-exec's CLI Watcher.\n")
		fmt.Fprintf(os.Stderr, "Each flag (except --idleTimeout, --logLevel) sets the corresponding\n")
		fmt.Fprintf(os.Stderr, "CLI_ACTIVITY_TRACKER_* env var before constructing the watcher.\n")
		fmt.Fprintf(os.Stderr, "Empty flag values mean 'do not set the env var; use watcher's default'.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := configureLogLevel(logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "invalid --logLevel: %v\n", err)
		os.Exit(2)
	}

	mappings := map[string]envMapping{
		"enabled":        {"CLI_ACTIVITY_TRACKER_ENABLED", parseBool},
		"checkPeriod":    {"CLI_ACTIVITY_TRACKER_CHECK_PERIOD", parseDurationString},
		"activityWindow": {"CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW", parseDurationString},
		"gracePeriod":    {"CLI_ACTIVITY_TRACKER_GRACE_PERIOD", parseDurationString},
		"maxProcessAge":  {"CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE", parseDurationString},
		"verbose":        {"CLI_ACTIVITY_TRACKER_VERBOSE", parseBool},
		"config":         {"CLI_ACTIVITY_TRACKER_CONFIG", passthrough},
	}
	flag.Visit(func(f *flag.Flag) {
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
