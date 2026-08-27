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

// Package tick is the stub for "report activity" that replaces
// che-machine-exec's workspace-stopping callback. It just logs.
package tick

import (
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger is a tick callback for the CLI Watcher. Each Fire() invocation
// logs a single info-level line and increments an internal counter.
//
// The watcher invokes the callback with no arguments, so the log line
// only includes context that was captured at startup: idleTimeout,
// logLevel, and a snapshot of CLI_ACTIVITY_TRACKER_* env vars.
type Logger struct {
	n           atomic.Uint64
	idleTimeout time.Duration
	logLevel    string
	envSnapshot map[string]string
	startedAt   time.Time
}

func New(idleTimeout time.Duration, logLevel string, envSnapshot map[string]string) *Logger {
	return &Logger{
		idleTimeout: idleTimeout,
		logLevel:    logLevel,
		envSnapshot: envSnapshot,
		startedAt:   time.Now(),
	}
}

func (l *Logger) Fire() {
	n := l.n.Add(1)
	uptime := time.Since(l.startedAt).Round(time.Second)
	logrus.Infof(
		"[tick #%d] Reporting activity tick at %s | uptime=%s, idleTimeout=%v, logLevel=%s, env=%s",
		n,
		time.Now().Format(time.RFC3339),
		uptime,
		l.idleTimeout,
		l.logLevel,
		formatEnv(l.envSnapshot),
	)
}

func (l *Logger) Count() uint64 { return l.n.Load() }

func formatEnv(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
	}
	b.WriteByte('}')
	return b.String()
}
