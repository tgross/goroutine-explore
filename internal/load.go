// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

// Copyright (c) 2017-2021 linuxerwang and goroutine-inspect contributors
// SPDX-License-Identifier: BSD-2-Clause

package internal

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	headerStartPatt  = `goroutine\s+(\d+)`
	headerGPPatt     = `(?:\sgp=0x[a-f0-9]+)*`
	headerMPatt      = `(?:\sm=\d+)*`
	headerMPPatt     = `(?:\smp=0x[a-f0-9]+)*`
	headerStatusPatt = `\s\[(.*?)(?:, (\d+) minutes*)*\]`
	headerLabelPatt  = `(?:\s*\{(.*)\})*`

	headerPatt = regexp.MustCompile(
		`^` +
			headerStartPatt +
			headerGPPatt +
			headerMPatt +
			headerMPPatt +
			headerStatusPatt +
			headerLabelPatt +
			`:$`,
	)
)

func load(path string) (*GoroutineDump, error) {
	// special case for when -e is used without a tty for stdin
	if path == "STDIN" {
		reader := bufio.NewReader(os.Stdin)
		return loadFrom(reader, headerPatt)
	}

	path = expandPath(path)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return loadFrom(f, headerPatt)
}

// expandPath handles homedir and environment variable expansion on a path
func expandPath(path string) string {
	path = strings.Trim(path, `"`)
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	path = os.ExpandEnv(path)
	return path
}

func loadFrom(r io.Reader, startPattern *regexp.Regexp) (*GoroutineDump, error) {
	dump := NewGoroutineDump()

	var goroutine *Goroutine
	var err error

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// ensure there are no control characters from an untrusted dump
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\t' {
				return -1
			}
			return r
		}, line)

		matches := startPattern.FindStringSubmatch(line)
		switch {
		case len(matches) > 0:
			// Freeze any previous goroutine to tolerate dumps without line
			// breaks
			if goroutine != nil {
				goroutine.Freeze()
			}
			goroutine, err = NewGoroutineFromMatch(matches)
			if err != nil {
				return nil, err
			}
			dump.Add(goroutine)
		case line == "":
			// End of a goroutine section.
			if goroutine != nil {
				goroutine.Freeze()
			}
			goroutine = nil
		case goroutine != nil:
			err := goroutine.AddLine(line)
			if err != nil {
				return nil, err
			}
		}
	}

	if goroutine != nil {
		goroutine.Freeze()
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return dump, nil
}
