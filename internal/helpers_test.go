// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
)

// mockMinGoroutine takes a header returns a new goroutine with a hash from just
// the header, so that we can more easily compare outputs and control
// deduplication
func mockMinGoroutine(header string) *Goroutine {
	g, err := NewGoroutine(header)
	if err != nil {
		panic(fmt.Sprintf("invalid header %q: %v", header, err))
	}
	// fake a hash because there are no lines
	g.hash = base64.StdEncoding.EncodeToString(
		[]byte(header))
	return g
}

// mockGoroutine returns a Goroutine with the provided ID and state. If a stack
// isn't provided, it will have a randomly-generated one.
func mockGoroutine(id int, state string, stacks ...string) *Goroutine {
	stack := strings.Join(stacks, "\n")
	if stack == "" {
		stack = mockStack(5)
	}
	src := fmt.Sprintf("goroutine %d [%s]:\n%s", id, state, stack)
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

// mockStack generates a random fake stack trace
func mockStack(depth int) string {
	out := ""
	names := []string{
		"net", "http", "Reader", "server", "poll", "Manager",
		"block", "sync", "Splines", "Worker", "poke"}
	meta := []string{"foo", "bar", "baz", "quux"}

	for range depth {
		out += fmt.Sprintf("%s/%s.(*%s).%s.%s()\n",
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
		)
		out += fmt.Sprintf("\t/src/%s/%s/%s/%s.go:%d\n",
			meta[rand.Intn(len(meta))],
			meta[rand.Intn(len(meta))],
			meta[rand.Intn(len(meta))],
			names[rand.Intn(len(names))],
			rand.Intn(300),
		)
	}
	return out
}
