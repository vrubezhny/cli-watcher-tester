# CLI Watcher Tester Makefile
#
# Builds a standalone tester for che-machine-exec's timeout/cli-watcher.go.
# The watcher source is copied from $CHE_MACHINE_EXEC_DIR (defaults to
# ../che-machine-exec) on every build so the tester always exercises
# the current source.

CHE_MACHINE_EXEC_DIR ?= ../che-machine-exec
WATCHER_SRC          := $(CHE_MACHINE_EXEC_DIR)/timeout/cli-watcher.go
WATCHER_DST          := internal/timeout/cli-watcher.go

BIN                  := bin/cli-watcher-tester
PKG                  := ./...

.PHONY: all sync build run run-enabled run-verbose run-fulllog test clean

all: build

# Copy cli-watcher.go from che-machine-exec into our internal package.
# Cheap (~1ms), so build depends on it.
# Hard precondition: the source file MUST exist. If not, fail with a
# clear message rather than producing a half-built state.
sync:
	@if [ -z "$(CHE_MACHINE_EXEC_DIR)" ]; then \
		echo "ERROR: CHE_MACHINE_EXEC_DIR is not set" >&2; \
		echo "  Set it to the path of your che-machine-exec checkout, e.g.:" >&2; \
		echo "    make build CHE_MACHINE_EXEC_DIR=/path/to/che-machine-exec" >&2; \
		exit 1; \
	fi
	@if [ ! -f "$(WATCHER_SRC)" ]; then \
		echo "ERROR: che-machine-exec source not found at: $(WATCHER_SRC)" >&2; \
		echo "  Set CHE_MACHINE_EXEC_DIR to the directory containing timeout/cli-watcher.go, e.g.:" >&2; \
		echo "    make build CHE_MACHINE_EXEC_DIR=/path/to/che-machine-exec" >&2; \
		exit 1; \
	fi
	@mkdir -p $(dir $(WATCHER_DST))
	cp $(WATCHER_SRC) $(WATCHER_DST)
	@echo "synced $(WATCHER_SRC) -> $(WATCHER_DST)"

# Compile binary
build: sync
	go build -o $(BIN) .

# Run variants. All depend on build.
# - make run                 : respect watcher's own defaults (no-op until --enabled is set)
# - make run ARGS="..."      : pass arbitrary flags through
# - make run-enabled         : enable the watcher, keep watcher's other defaults
# - make run-verbose         : enable + watcher activity logs at info; tester stays at info
# - make run-fulllog         : enable + watcher activity logs at info + tester at debug
run: build
	./$(BIN) $(ARGS)

run-enabled: build
	./$(BIN) --enabled=true $(ARGS)

# Only the watcher's activity-detection logs are promoted to info.
# Tester's own logLevel stays at 'info' (no debug noise).
run-verbose: build
	./$(BIN) --enabled=true --verbose=true --logLevel=info $(ARGS)

# Everything noisy: watcher verbose + tester logLevel=debug.
run-fulllog: build
	./$(BIN) --enabled=true --verbose=true --logLevel=debug $(ARGS)

# Unit tests (placeholder)
test: sync
	go test $(PKG)

clean:
	rm -rf bin/
