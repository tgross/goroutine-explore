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
	startLinePattern = regexp.MustCompile(`^goroutine\s+(\d+)\s+.*\[(.*)\]:$`)
)

func load(fn string) (*GoroutineDump, error) {
	fn = strings.Trim(fn, "\"")

	if strings.HasPrefix(fn, "~") {
		home, _ := os.UserHomeDir()
		fn = filepath.Join(home, fn[1:])
	}
	fn = os.ExpandEnv(fn)

	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return loadFrom(f, startLinePattern)
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

		switch {
		case startPattern.MatchString(line):
			// Freeze any previous goroutine to tolerate dumps without line
			// breaks
			if goroutine != nil {
				goroutine.Freeze()
			}

			goroutine, err = NewGoroutine(line)
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
			goroutine.AddLine(line)
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
