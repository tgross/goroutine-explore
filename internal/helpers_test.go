// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/fnv"
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
	hasher := fnv.New64()
	hasher.Write([]byte(header))
	g.hash = hasher.Sum64()
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
		_ = goroutine.AddLine(line)
	}
	goroutine.Freeze()
	return goroutine
}

// mockStack generates a random fake stack trace
func mockStack(depth int) string {
	out := strings.Builder{}
	names := []string{
		"net", "http", "Reader", "server", "poll", "Manager",
		"block", "sync", "Splines", "Worker", "poke"}
	meta := []string{"foo", "bar", "baz", "quux"}

	for range depth {
		out.WriteString(fmt.Sprintf("%s/%s.(*%s).%s.%s()\n",
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
			names[rand.Intn(len(names))],
		))
		out.WriteString(fmt.Sprintf("\t/src/%s/%s/%s/%s.go:%d\n",
			meta[rand.Intn(len(meta))],
			meta[rand.Intn(len(meta))],
			meta[rand.Intn(len(meta))],
			names[rand.Intn(len(names))],
			rand.Intn(300),
		))
	}
	return out.String()
}

func mockDumpForGraph() *GoroutineDump {
	gd := NewGoroutineDump()
	for i := 1; i < 30; i++ {
		gd.Add(mockMinGoroutine(fmt.Sprintf("goroutine %d [running]:", i)))
	}

	// 1->3->5->8->13->21
	gd.byID(3).CreatedBy = 1
	gd.byID(5).CreatedBy = 3
	gd.byID(8).CreatedBy = 5
	gd.byID(13).CreatedBy = 8
	gd.byID(21).CreatedBy = 13

	// 1->2->4->6->10->12->14
	gd.byID(2).CreatedBy = 1
	gd.byID(4).CreatedBy = 2
	gd.byID(6).CreatedBy = 4
	gd.byID(10).CreatedBy = 6
	gd.byID(12).CreatedBy = 10
	gd.byID(14).CreatedBy = 12

	// 7->15->27
	gd.byID(15).CreatedBy = 7
	gd.byID(27).CreatedBy = 15

	return gd
}
