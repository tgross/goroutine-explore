// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"io"
	"maps"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// TestEvaluator is "end-to-end" tests of the evaluator, asserting expected
// output given an environment
func TestEvaluator(t *testing.T) {

	tempDir := t.TempDir()
	env := map[string]Value{}

	g1 := NewGoroutineDump()
	g1.Add(mockGoroutine(1, "chan receive, 5 minutes"))
	g1.Add(mockGoroutine(2, "syscall"))
	g1.Add(mockGoroutine(3, "select, 1 minutes"))

	g2 := NewGoroutineDump()
	g2.Add(mockGoroutine(20, "runnable"))

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
				must.Eq(t, 2, diff.Common.Len())
				must.Eq(t, 1, diff.Common.Next().ID)
				must.Eq(t, 1, diff.Common.Next().ID) // not indexed yet
				must.Eq(t, 1, diff.Right.Len())
				must.Eq(t, 2, diff.Right.Next().ID)
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
			err := e.Eval(t.Context(), tc.src)
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

func TestEvaluator_PipelineEquivalence(t *testing.T) {
	src0 := `g2 = g1.where(.state == "select" and .duration > 1)`
	src1 := `g2 = g1.where(.state == "select").where(.duration > 1)`
	src2 := `g2 = g1 | where(.state == "select") | where(.duration > 1)`

	g1 := NewGoroutineDump()
	g1.Add(mockGoroutine(1, "chan receive, 5 minutes"))
	g1.Add(mockGoroutine(3, "select, 1 minutes"))
	g1.Add(mockGoroutine(4, "select, 4 minutes"))
	g1.Add(mockGoroutine(5, "select, 10 minutes"))

	env0 := map[string]Value{}
	env0["g1"] = Value{Tag: TagDump, Data: g1}
	env1 := maps.Clone(env0)
	env2 := maps.Clone(env1)

	e := NewEvaluator(&Config{
		WorkDir: t.TempDir(),
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	})

	e.vm.env = env0
	err := e.Eval(t.Context(), src0)
	must.NoError(t, err)
	got0, _ := e.vm.pop()
	must.NotNil(t, got0)

	e.vm.env = env1
	err = e.Eval(t.Context(), src1)
	must.NoError(t, err)
	got1, _ := e.vm.pop()
	e.vm.debug()
	must.NotNil(t, got1)

	e.vm.env = env2
	err = e.Eval(t.Context(), src2)
	must.NoError(t, err)
	got2, _ := e.vm.pop()
	must.NotNil(t, got2)

	dump0, _ := got0.Data.(*GoroutineDump)
	must.Eq(t, 2, dump0.Len(), must.Sprintf("%s", dump0.String()))
	dump1, _ := got1.Data.(*GoroutineDump)
	must.Eq(t, 2, dump1.Len(), must.Sprintf("%s", dump1.String()))
	dump2, _ := got2.Data.(*GoroutineDump)
	must.Eq(t, 2, dump2.Len(), must.Sprintf("%s", dump2.String()))
}

func BenchmarkEvaluator(b *testing.B) {

	env := map[string]Value{}

	// TODO: this would be nicer if we had testdata files we could read in
	g1 := NewGoroutineDump()
	g1.Add(mockGoroutine(1, "chan receive 5 min"))
	g1.Add(mockGoroutine(2, "syscall"))
	g1.Add(mockGoroutine(3, "running"))

	env["g1"] = Value{Tag: TagDump, Data: g1}

	src := `g1 where .duration > 0 and .state == "select"`

	length := 0
	e := NewEvaluator(&Config{
		WorkDir: b.TempDir(),
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	})
	e.vm.env = env // simulate earlier expressions
	ctx := b.Context()

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
