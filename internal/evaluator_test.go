// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// TestEvaluator is "end-to-end" tests of the evaluator, asserting expected
// output given an environment
func TestEvaluator(t *testing.T) {

	tempDir := t.TempDir()
	env := map[string]Value{}

	// TODO: this would be nicer if we had testdata files we could read in
	g1 := NewGoroutineDump()
	g1.Add(testGoroutineFromStack(`goroutine 1 [chan receive, 5 minutes]:
main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`))

	g1.Add(testGoroutineFromStack(`goroutine 2 [syscall]:
os/signal.signal_recv()
	/usr/local/go/src/runtime/sigqueue.go:152 +0x29
os/signal.loop()
	/usr/local/go/src/os/signal/signal_unix.go:23 +0x13
created by os/signal.Notify.func1.1 in goroutine 1
	/usr/local/go/src/os/signal/signal.go:151 +0x1f
`))

	g1.Add(testGoroutineFromStack(`goroutine 3 [select, 1 minutes]:
		main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`))

	g2 := NewGoroutineDump()
	g2.Add(testGoroutineFromStack(`goroutine 20 [runnable]:
net/http.(*connReader).startBackgroundRead.gowrap2()
	/usr/local/go/src/net/http/server.go:677
runtime.goexit({})
	/usr/local/go/src/runtime/asm_amd64.s:1695 +0x1
created by net/http.(*connReader).startBackgroundRead in goroutine 37
	/usr/local/go/src/net/http/server.go:677 +0xba
`))

	env["g1"] = Value{Tag: TagDump, Data: g1}
	env["g2"] = Value{Tag: TagDump, Data: g2}

	testCases := []struct {
		name         string
		src          string
		expect       func(*testing.T, *GoroutineDump)
		expectDiff   func(*testing.T, *Diff)
		expectErrMsg string
	}{
		{
			name:         "empty src",
			src:          ``,
			expectErrMsg: "EOF",
		},
		{
			name: "simple where string comparison",
			src:  `g1.where(.state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "simple where broken across newline",
			src: `g1.where(
				 .state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "simple where numeric comparison",
			src:  `g1.where(.duration > 0)`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 2, dump.Len())
				must.Eq(t, 1, dump.Next().ID)
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "simple where with binding",
			src:  `g3 = g1.where(.state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "compound where",
			src:  `g1.where(.duration > 0 and .state == "select")`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "diffed expressions",
			src: `l, c, r = diff(
				g1.where(.duration > 0),
				g1.delete(.state == "select"))`,
			expectDiff: func(t *testing.T, diff *Diff) {
				must.Eq(t, 1, diff.Left.Len())
				must.Eq(t, 3, diff.Left.Next().ID)
				must.Eq(t, 1, diff.Right.Len())
				must.Eq(t, 2, diff.Right.Next().ID)
				must.Eq(t, 1, diff.Common.Len())
				must.Eq(t, 1, diff.Common.Next().ID)
			},
		},
		{
			name: "unioned expressions",
			src: `union(
				g1.where(.duration > 0 and .lines > 0),
				g2.where(.state == "runnable"))`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 3, dump.Len())
				must.Eq(t, 20, dump.Next().ID)
				must.Eq(t, 1, dump.Next().ID)
				must.Eq(t, 3, dump.Next().ID)
			},
		},
		{
			name: "intersecting expressions with binding",
			src: `g3 = intersect(
				g1.where(.duration > 0 and .lines > 0),
				g1.where(.state == "chan receive"))`,
			expect: func(t *testing.T, dump *GoroutineDump) {
				must.Eq(t, 1, dump.Len())
				must.Eq(t, 1, dump.Next().ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errRecorder := new(bytes.Buffer)

			e := NewEvaluator(&Config{
				WorkDir: tempDir,
				Stdout:  io.Discard, // TODO: snapshot tests would be cool
				Stderr:  errRecorder,
			})
			e.vm.env = env // simulate earlier expressions
			err := e.Eval(context.TODO(), tc.src)
			if tc.expectErrMsg != "" {
				test.EqError(t, err, tc.expectErrMsg)
			} else {
				must.NoError(t, err)
				got, _ := e.vm.pop()
				must.NotNil(t, got)
				if tc.expect != nil {
					dump, ok := got.Data.(*GoroutineDump)
					must.True(t, ok, must.Sprintf("did not return dump: %+v", got))
					tc.expect(t, dump)
				} else if tc.expectDiff != nil {
					diff, ok := got.Data.(*Diff)
					must.True(t, ok, must.Sprintf("did not return diff: %+v", got))
					tc.expectDiff(t, diff)
				}
			}
		})
	}
}

func BenchmarkEvaluator(b *testing.B) {

	env := map[string]Value{}

	// TODO: this would be nicer if we had testdata files we could read in
	g1 := NewGoroutineDump()
	g1.Add(testGoroutineFromStack(`goroutine 1 [chan receive 5 min]:
main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`))
	g1.Add(testGoroutineFromStack(`goroutine 2 [syscall]:
os/signal.signal_recv()
	/usr/local/go/src/runtime/sigqueue.go:152 +0x29
os/signal.loop()
	/usr/local/go/src/os/signal/signal_unix.go:23 +0x13
created by os/signal.Notify.func1.1 in goroutine 1
	/usr/local/go/src/os/signal/signal.go:151 +0x1f
`))
	g1.Add(testGoroutineFromStack(`main.main()
	/home/tim/src/tgross/sdnotifying/main.go:62 +0x1e5
`))

	env["g1"] = Value{Tag: TagDump, Data: g1}

	src := `g1 where .duration > 0 and .state == "select"`

	length := 0
	e := NewEvaluator(&Config{
		WorkDir: b.TempDir(),
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	})
	e.vm.env = env // simulate earlier expressions
	ctx := context.TODO()

	for b.Loop() {
		err := e.Eval(ctx, src)
		must.NoError(b, err)
		got, err := e.vm.pop()
		must.NoError(b, err)
		dump, ok := got.Data.(*GoroutineDump)
		must.True(b, ok, must.Sprintf("did not return dump: %v", dump))
		length += dump.Len()
	}
}
