// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
)

// testGoroutine takes a header and optionally a list of lines, and returns a
// new goroutine.
func testGoroutine(header string, lines ...string) *Goroutine {
	g, err := NewGoroutine(header)
	if err != nil {
		panic(fmt.Sprintf("invalid header %q: %v", header, err))
	}
	for _, line := range lines {
		g.AddLine(line)
	}
	g.Freeze()

	// fake a hash if there are no lines
	if len(lines) == 0 {
		g.hash = base64.StdEncoding.EncodeToString(
			[]byte(header))
	}
	return g
}

// testGoroutineFromStack takes the stack trace as a string blob and returns a
// new goroutine with the hash set.
func testGoroutineFromStack(src string) *Goroutine {
	r := bytes.NewBufferString(src)
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	header := scanner.Text()
	goroutine, err := NewGoroutine(header)
	if err != nil {
		panic(fmt.Sprintf("invalid header %q: %v", header, err))
	}

	for scanner.Scan() {
		line := scanner.Text()
		goroutine.AddLine(line)
	}
	goroutine.Freeze()
	return goroutine
}
