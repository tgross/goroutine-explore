// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"bytes"
	"testing"

	"github.com/shoenig/test/must"
)

func TestVM_BasicStackOps(t *testing.T) {
	vm := NewVM(&Config{WorkDir: t.TempDir()})

	vm.push(Value{Tag: TagNumber, Data: 1})
	vm.push(Value{Tag: TagNumber, Data: 2})
	vm.push(Value{Tag: TagNumber, Data: 3})

	val, err := vm.peek()
	must.NoError(t, err)
	must.Eq(t, 3, val.Data)

	val, err = vm.pop()
	must.NoError(t, err)
	must.Eq(t, 3, val.Data)

	val, err = vm.peek()
	must.NoError(t, err)
	must.Eq(t, 2, val.Data)
}

func TestVM_SimpleWhere(t *testing.T) {

	testCases := []struct {
		name      string
		ops       []Op
		constants []any
		g1Fn      func(*GoroutineDump)
		expectFn  func(*testing.T, *GoroutineDump)
	}{
		{
			name: "numeric comparison",
			ops: []Op{ // source: `g2 = g1.where(duration > 10)`
				encode(OpCodeLoadGoroutineDump, 1), // load g1
				encode(OpCodeTempDump, 0),          // start
				encode(OpCodeNextGoroutine, 9),     // addr when done
				encode(OpCodeLoadFieldAccessor, 2), // load .duration
				encode(OpCodeLoadNumber, 3),        // load 10
				encode(OpCodeGreater, 0),           // compare
				encode(OpCodeJumpIfFalse, 2),       // addr if false
				encode(OpCodeAddGoroutine, 0),      // keep
				encode(OpCodeJumpTo, 2),            // unconditional jump to addr
				encode(OpCodePushDump, 0),          // push temp dump to stack
				encode(OpCodeAssignment, 0),        // assign g2
			},
			constants: []any{"g2", "g1", ".duration", 10},
			g1Fn: func(g1 *GoroutineDump) {
				g1.Add(mockMinGoroutine(`goroutine 1 [running]:`))
				g1.Add(mockMinGoroutine(`goroutine 2 [select, 20 minutes]:`))
				g1.Add(mockMinGoroutine(`goroutine 3 [IO wait, 5 minutes]:`))
			},
			expectFn: func(t *testing.T, g2 *GoroutineDump) {
				must.Eq(t, 1, g2.Len())
				must.Eq(t, 2, g2.Next().ID)
			},
		},
		{
			name: "not-equal comparison",
			ops: []Op{ // source: `g2 = g1.where(.state != "running")`
				encode(OpCodeLoadGoroutineDump, 1), // load g1
				encode(OpCodeTempDump, 0),          // start
				encode(OpCodeNextGoroutine, 9),     // addr when done
				encode(OpCodeLoadFieldAccessor, 2), // load .state
				encode(OpCodeLoadString, 3),        // load "running"
				encode(OpCodeNotEqual, 0),          // compare
				encode(OpCodeJumpIfFalse, 2),       // addr if false
				encode(OpCodeAddGoroutine, 0),      // keep
				encode(OpCodeJumpTo, 2),            // unconditional jump to addr
				encode(OpCodePushDump, 0),          // push temp dump to stack
				encode(OpCodeAssignment, 0),        // assign g2
			},
			constants: []any{"g2", "g1", ".state", "running"},
			g1Fn: func(g1 *GoroutineDump) {
				g1.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
				g1.Add(mockMinGoroutine(`goroutine 2 [running]:`))
				g1.Add(mockMinGoroutine(`goroutine 3 [IO wait, 5 minutes]:`))
			},
			expectFn: func(t *testing.T, g2 *GoroutineDump) {
				must.Eq(t, 2, g2.Len())
				must.Eq(t, 1, g2.Next().ID)
				must.Eq(t, 3, g2.Next().ID)
			},
		},
		{
			name: "contains comparison",
			ops: []Op{ // source: `g2 = g1.where(.trace contains "sdnotifying")`
				encode(OpCodeLoadGoroutineDump, 1), // load g1
				encode(OpCodeTempDump, 0),          // start
				encode(OpCodeNextGoroutine, 9),     // addr when done
				encode(OpCodeLoadFieldAccessor, 2), // load .trace
				encode(OpCodeLoadString, 3),        // load "running"
				encode(OpCodeContains, 0),          // compare
				encode(OpCodeJumpIfFalse, 2),       // addr if false
				encode(OpCodeAddGoroutine, 0),      // keep
				encode(OpCodeJumpTo, 2),            // unconditional jump to addr
				encode(OpCodePushDump, 0),          // push temp dump to stack
				encode(OpCodeAssignment, 0),        // assign g2
			},
			constants: []any{"g2", "g1", ".trace", "sdnotifying"},
			g1Fn: func(g1 *GoroutineDump) {
				g1.Add(mockGoroutine(1, "running"))
				g1.Add(mockGoroutine(2, "select, 1 minutes", `main.main()
	/src/sdnotifying/main.go:62 +0x1e5
`))
				g1.Add(mockGoroutine(3, "syscall", `os/signal.signal_recv()
	/usr/local/go/src/runtime/sigqueue.go:152 +0x29
				`))
			},
			expectFn: func(t *testing.T, g2 *GoroutineDump) {
				must.Eq(t, 1, g2.Len())
				must.Eq(t, 2, g2.Next().ID)
			},
		},
		{
			name: "regex comparison",
			ops: []Op{ // source: `g2 = g1.where(.trace ~= "sdn.*")`
				encode(OpCodeLoadGoroutineDump, 1), // load g1
				encode(OpCodeTempDump, 0),          // start
				encode(OpCodeNextGoroutine, 9),     // addr when done
				encode(OpCodeLoadFieldAccessor, 2), // load .trace
				encode(OpCodeLoadString, 3),        // load pattern
				encode(OpCodeRegexMatches, 0),      // compare
				encode(OpCodeJumpIfFalse, 2),       // addr if false
				encode(OpCodeAddGoroutine, 0),      // keep
				encode(OpCodeJumpTo, 2),            // unconditional jump to addr
				encode(OpCodePushDump, 0),          // push temp dump to stack
				encode(OpCodeAssignment, 0),        // assign g2
			},
			constants: []any{"g2", "g1", ".trace", "sdn.*"},
			g1Fn: func(g1 *GoroutineDump) {
				g1.Add(mockGoroutine(1, "running"))
				g1.Add(mockGoroutine(2, "select, 1 minutes", `main.main()
	/src/sdnotifying/main.go:62 +0x1e5
`))
				g1.Add(mockGoroutine(3, "syscall", `os/signal.signal_recv()
	/usr/local/go/src/runtime/sigqueue.go:152 +0x29
				`))
			},
			expectFn: func(t *testing.T, g2 *GoroutineDump) {
				must.Eq(t, 1, g2.Len())
				must.Eq(t, 2, g2.Next().ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunk := &Chunk{
				ops:       tc.ops,
				constants: tc.constants,
			}
			vm := NewVM(&Config{WorkDir: t.TempDir()})
			vm.Reset(chunk)

			g1 := NewGoroutineDump()
			tc.g1Fn(g1)
			vm.env = map[string]Value{
				"g1": {Tag: TagDump, Data: g1},
			}

			err := vm.Run(t.Context())
			vm.debug()
			must.NoError(t, err)

			result, _ := vm.pop()
			must.Eq(t, TagDump, result.Tag)
			got, ok := result.Data.(*GoroutineDump)
			must.True(t, ok)
			got.StartIter()
			tc.expectFn(t, got)
		})
	}
}

func TestVM_SetFunctions(t *testing.T) {

	testCases := []struct {
		name      string
		ops       []Op
		constants []any
		g1Fn      func() *GoroutineDump
		g2Fn      func() *GoroutineDump
		expectFn  func(*testing.T, *GoroutineDump)
	}{
		{
			// source: `g1.union(g2)`
			name: "simple union",
			ops: []Op{
				encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
				encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
				encode(OpCodeFuncUnion, 0),         // 02 union
			},
			constants: []any{"g1", "g2"},
			g1Fn: func() *GoroutineDump {
				g1 := NewGoroutineDump()
				g1.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
				g1.Add(mockMinGoroutine(`goroutine 2 [running]:`))
				return g1
			},
			g2Fn: func() *GoroutineDump {
				g2 := NewGoroutineDump()
				g2.Add(mockMinGoroutine(`goroutine 3 [select, 10 IO wait]:`))
				return g2
			},
			expectFn: func(t *testing.T, g *GoroutineDump) {
				must.Eq(t, 3, g.Len())
				must.Eq(t, 3, g.Next().ID)
				must.Eq(t, 1, g.Next().ID)
				must.Eq(t, 2, g.Next().ID)
			},
		},

		{
			// source: `g1.union(g2.where(.duration > 10))`
			name: "union nested on right",
			ops: []Op{
				encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
				encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
				encode(OpCodeTempDump, 0),          // 02 setup scratch register
				encode(OpCodeNextGoroutine, 10),    // 03 addr when done
				encode(OpCodeLoadFieldAccessor, 2), // 04 load .duration
				encode(OpCodeLoadNumber, 3),        // 05 load 10
				encode(OpCodeGreater, 0),           // 06 compare push bool
				encode(OpCodeJumpIfFalse, 3),       // 07 addr if false
				encode(OpCodeAddGoroutine, 0),      // 08 keep
				encode(OpCodeJumpTo, 3),            // 09 unconditional jump
				encode(OpCodePushDump, 0),          // 10 push temp dump to stack
				encode(OpCodeFuncUnion, 0),         // 11 union
			},
			constants: []any{"g1", "g2", ".duration", 10},
			g1Fn: func() *GoroutineDump {
				g1 := NewGoroutineDump()
				g1.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
				g1.Add(mockMinGoroutine(`goroutine 2 [running]:`))
				return g1
			},
			g2Fn: func() *GoroutineDump {
				g2 := NewGoroutineDump()
				g2.Add(mockMinGoroutine(`goroutine 3 [IO wait, 20 minutes]:`))
				g2.Add(mockMinGoroutine(`goroutine 98 [chan receive]:`))
				g2.Add(mockMinGoroutine(`goroutine 99 [running]:`))
				return g2
			},
			expectFn: func(t *testing.T, g *GoroutineDump) {
				must.Eq(t, 3, g.Len())
				must.Eq(t, 3, g.Next().ID)
				must.Eq(t, 1, g.Next().ID)
				must.Eq(t, 2, g.Next().ID)
			},
		},

		{
			// source: `union(g1.where(.duration > 10), g2)`
			name: "union nested on left",
			ops: []Op{
				encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
				encode(OpCodeTempDump, 0),          // 01 setup scratch register
				encode(OpCodeNextGoroutine, 9),     // 02 addr when done
				encode(OpCodeLoadFieldAccessor, 2), // 03 load .duration
				encode(OpCodeLoadNumber, 3),        // 04 load 10
				encode(OpCodeGreater, 0),           // 05 compare push bool
				encode(OpCodeJumpIfFalse, 2),       // 06 addr if false
				encode(OpCodeAddGoroutine, 0),      // 07 keep
				encode(OpCodeJumpTo, 2),            // 08 unconditional jump
				encode(OpCodePushDump, 0),          // 09 push temp dump to stack
				encode(OpCodeLoadGoroutineDump, 1), // 10 load g2
				encode(OpCodeFuncUnion, 0),         // 11 union
			},
			constants: []any{"g1", "g2", ".duration", 10},
			g1Fn: func() *GoroutineDump {
				g1 := NewGoroutineDump()
				g1.Add(mockMinGoroutine(`goroutine 2 [IO wait, 20 minutes]:`))
				g1.Add(mockMinGoroutine(`goroutine 98 [chan receive]:`))
				g1.Add(mockMinGoroutine(`goroutine 99 [running]:`))
				return g1
			},
			g2Fn: func() *GoroutineDump {
				g2 := NewGoroutineDump()
				g2.Add(mockMinGoroutine(`goroutine 3 [select, 20 minutes]:`))
				g2.Add(mockMinGoroutine(`goroutine 1 [running]:`))
				return g2
			},
			expectFn: func(t *testing.T, g *GoroutineDump) {
				must.Eq(t, 3, g.Len())
				must.Eq(t, 3, g.Next().ID)
				must.Eq(t, 1, g.Next().ID)
				must.Eq(t, 2, g.Next().ID)
			},
		},

		{
			// source: `g1.intersect(g2)`
			name: "simple intersect",
			ops: []Op{
				encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
				encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
				encode(OpCodeFuncIntersect, 0),     // 02 union
			},
			constants: []any{"g1", "g2"},
			g1Fn: func() *GoroutineDump {
				g1 := NewGoroutineDump()
				g1.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
				g1.Add(mockMinGoroutine(`goroutine 2 [running]:`))
				return g1
			},
			g2Fn: func() *GoroutineDump {
				g2 := NewGoroutineDump()
				g2.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
				g2.Add(mockMinGoroutine(`goroutine 3 [IO wait, 10 minutes]:`))
				return g2
			},
			expectFn: func(t *testing.T, g *GoroutineDump) {
				must.Eq(t, 1, g.Len())
				must.Eq(t, 1, g.Next().ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunk := &Chunk{
				ops:       tc.ops,
				constants: tc.constants,
			}
			vm := NewVM(&Config{WorkDir: t.TempDir()})
			vm.Reset(chunk)

			vm.env = map[string]Value{
				"g1": {Tag: TagDump, Data: tc.g1Fn()},
				"g2": {Tag: TagDump, Data: tc.g2Fn()},
			}

			err := vm.Run(t.Context())
			vm.debug()
			must.NoError(t, err)

			result, _ := vm.pop()
			must.Eq(t, TagDump, result.Tag)
			g3, ok := result.Data.(*GoroutineDump)
			must.True(t, ok)
			g3.StartIter()
			tc.expectFn(t, g3)
		})
	}
}

func TestVM_MultiAssignDiff(t *testing.T) {

	// source: `g3, g4, g5 = g1.diff(g2)`
	chunk := &Chunk{
		ops: []Op{
			encode(OpCodeLoadGoroutineDump, 4), // load g1
			encode(OpCodeLoadGoroutineDump, 5), // load g2
			encode(OpCodeFuncDiff, 0),          // diff
			encode(OpCodeAssignment, 3),        // assign to g3, g4, g5
		},
		constants: []any{
			"g3", "g4", "g5", MultiAssignment{0, 1, 2}, "g1", "g2"},
	}
	vm := NewVM(&Config{WorkDir: t.TempDir()})
	vm.Reset(chunk)

	g1 := NewGoroutineDump()
	g1.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
	g1.Add(mockMinGoroutine(`goroutine 2 [running]:`))

	g2 := NewGoroutineDump()
	g2.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
	g2.Add(mockMinGoroutine(`goroutine 3 [IO wait, 10 minutes]:`))

	vm.env = map[string]Value{
		"g1": {Tag: TagDump, Data: g1},
		"g2": {Tag: TagDump, Data: g2},
	}

	err := vm.Run(t.Context())
	vm.debug()
	must.NoError(t, err)

	gd3 := expectDumpFromEnv(t, vm.env, "g3")
	must.Eq(t, "[2]", gd3.String())

	gd4 := expectDumpFromEnv(t, vm.env, "g4")
	must.Eq(t, "[1 1]", gd4.String()) // not indexed so not de-duplicated

	gd5 := expectDumpFromEnv(t, vm.env, "g5")
	must.Eq(t, "[3]", gd5.String())
}

func TestVM_ShowFunctions(t *testing.T) {

	g1 := NewGoroutineDump()
	g1.Add(mockMinGoroutine("goroutine 1 [select, 20 minutes]:"))
	g1.Add(mockMinGoroutine("goroutine 2 [running]:"))
	g1.Add(mockMinGoroutine("goroutine 3 [IO wait, 10 minutes]:"))

	vm := NewVM(&Config{WorkDir: t.TempDir()})
	recorder := new(bytes.Buffer)
	vm.wOut = NewWriter(recorder)
	vm.env = map[string]Value{"g1": {Tag: TagDump, Data: g1}}

	testCases := []struct {
		name   string
		chunk  *Chunk
		expect string
	}{
		{ // source: `g1 | show(3, 1)`
			name: "show function",
			chunk: &Chunk{
				ops: []Op{
					encode(OpCodeLoadGoroutineDump, 0), // load g1
					encode(OpCodeLoadNumber, 1),        // load 1 (offset)
					encode(OpCodeLoadNumber, 2),        // load 3 (limit)
					encode(OpCodeFuncShowDump, 0),      // show
				},
				constants: []any{"g1", 1, 3},
			},
			expect: `goroutine 2 [running]:

goroutine 3 [IO wait, 10 minutes]:

`,
		},
		{ // source: `g1 | json()`
			name: "json function",
			chunk: &Chunk{
				ops: []Op{
					encode(OpCodeLoadGoroutineDump, 0), // load g1
					encode(OpCodeFuncToJSON, 0),        // show
				},
				constants: []any{"g1", 1, 3},
			},
			expect: `[
  {
    "id": 1,
    "duration": 20,
    "state": "select",
    "createdBy": 0,
    "trace": "",
    "lines": 1
  },
  {
    "id": 2,
    "duration": 0,
    "state": "running",
    "createdBy": 0,
    "trace": "",
    "lines": 1
  },
  {
    "id": 3,
    "duration": 10,
    "state": "IO wait",
    "createdBy": 0,
    "trace": "",
    "lines": 1
  }
]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vm.Reset(tc.chunk)
			recorder.Reset()
			err := vm.Run(t.Context())
			must.NoError(t, err)
			must.Eq(t, tc.expect, recorder.String())
		})
	}
}

func TestVM_CommandVars(t *testing.T) {
	// source: `vars`
	chunk := &Chunk{
		ops: []Op{encode(OpCodeCommandVars, 0)},
	}
	vm := NewVM(&Config{WorkDir: t.TempDir()})
	recorder := new(bytes.Buffer)
	vm.wOut = NewWriter(recorder)
	vm.Reset(chunk)

	g1 := NewGoroutineDump()
	g1.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
	g1.Add(mockMinGoroutine(`goroutine 2 [running]:`))
	g1.Add(mockMinGoroutine(`goroutine 3 [IO wait, 10 minutes]:`))

	g2 := NewGoroutineDump()
	g2.Add(mockMinGoroutine(`goroutine 1 [select, 20 minutes]:`))
	g2.Add(mockMinGoroutine(`goroutine 3 [IO wait, 10 minutes]:`))

	vm.env = map[string]Value{
		"g1": {Tag: TagDump, Data: g1},
		"g2": {Tag: TagDump, Data: g2},
	}

	err := vm.Run(t.Context())
	must.EqError(t, err, ErrCommandOk.Error())
	must.Eq(t, `g1: 3
g2: 2
`, recorder.String())

	vm.pragma.VarsDisplay = PragmaDisplaySummary
	vm.Reset(chunk)
	recorder.Reset()
	err = vm.Run(t.Context())
	must.EqError(t, err, ErrCommandOk.Error())
	must.Eq(t, `# of goroutines in "g1": 3
        IO wait: 1
        running: 1
         select: 1

# of goroutines in "g2": 2
        IO wait: 1
         select: 1

`, recorder.String())

}

func TestVM_CommandPragma(t *testing.T) {
	testCases := []struct {
		name      string
		ops       []Op
		constants []any
		expectFn  func(*testing.T, *Pragma)
	}{
		{
			name: "boolean",
			ops: []Op{ // src: `pragma empty.confirm true`
				encode(OpCodePushBool, 0),
				encode(OpCodeLoadString, 0),
				encode(OpCodeCommandSetPragma, 0),
			},
			constants: []any{"empty.confirm"},
			expectFn: func(t *testing.T, p *Pragma) {
				must.False(t, p.EmptyConfirm)
			},
		},
		{
			name: "numeric",
			ops: []Op{ // src: `pragma show.count 100`
				encode(OpCodeLoadNumber, 0),
				encode(OpCodeLoadString, 1),
				encode(OpCodeCommandSetPragma, 0),
			},
			constants: []any{100, "show.count"},
			expectFn: func(t *testing.T, p *Pragma) {
				must.Eq(t, 100, p.ShowCount)
			},
		},
		{
			name: "enum",
			ops: []Op{ // src: `pragma vars.display summary`
				encode(OpCodeLoadString, 0),
				encode(OpCodeLoadString, 1),
				encode(OpCodeCommandSetPragma, 0),
			},
			constants: []any{"summary", "vars.display"},
			expectFn: func(t *testing.T, p *Pragma) {
				must.Eq(t, PragmaDisplaySummary, p.VarsDisplay)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunk := &Chunk{
				ops:       tc.ops,
				constants: tc.constants,
			}
			vm := NewVM(&Config{WorkDir: t.TempDir()})
			vm.Reset(chunk)
			err := vm.Run(t.Context())
			vm.debug()
			must.EqError(t, err, ErrCommandOk.Error())
			tc.expectFn(t, vm.pragma)
		})
	}

}

func TestVM_Graph(t *testing.T) {
	gd := mockDumpForGraph()
	predicate := NewGoroutineDump()
	predicate.Add(gd.byID(6))
	predicate.Add(gd.byID(12))

	// source: `g1 = g.graph(id == 6 or id == 12)`
	chunk := &Chunk{
		ops: []Op{
			encode(OpCodeLoadGoroutineDump, 1), // 00 load g
			encode(OpCodeDup, 0),               // 01 dupe on stack
			encode(OpCodeTempDump, 0),          // 02 temp dump to reg
			encode(OpCodeNextGoroutine, 16),    // 03 next goroutine
			encode(OpCodeLoadFieldAccessor, 2), // 04 load .id
			encode(OpCodeLoadNumber, 3),        // 05 load 6
			encode(OpCodeEqual, 0),             // 06 compare, push bool to stack
			encode(OpCodeJumpIfFalse, 10),      // 07 jump to next condition
			encode(OpCodePushBool, 1),          // 08 push true to stack
			encode(OpCodeJumpTo, 13),           // 09 jump past next condition
			encode(OpCodeLoadFieldAccessor, 2), // 10 load .id
			encode(OpCodeLoadNumber, 4),        // 11 load 12
			encode(OpCodeEqual, 0),             // 12 compare, push bool to stack
			encode(OpCodeJumpIfFalse, 3),       // 13 next goroutine
			encode(OpCodeAddGoroutine, 0),      // 14 add goroutine
			encode(OpCodeJumpTo, 3),            // 15 next goroutine
			encode(OpCodePushDump, 0),          // 16 push
			encode(OpCodeFuncGraph, 0),         // 18 pop 2 dumps and graph
			encode(OpCodeAssignment, 0),        // 18 assign result to g1
		},
		constants: []any{"g1", "g", ".id", 6, 12},
	}

	vm := NewVM(&Config{WorkDir: t.TempDir()})
	vm.Reset(chunk)
	vm.env = map[string]Value{
		"g": {Tag: TagDump, Data: gd},
	}

	err := vm.Run(t.Context())
	vm.debug()
	must.NoError(t, err)

	result, _ := vm.pop()
	must.Eq(t, TagDump, result.Tag)
	g1, ok := result.Data.(*GoroutineDump)
	must.True(t, ok)
	must.Eq(t, "[1 2 4 6 10 12 14]", g1.String())
}

func expectDumpFromEnv(t *testing.T, env map[string]Value, name string) *GoroutineDump {
	t.Helper()
	val, ok := env[name]
	must.True(t, ok, must.Sprintf("%s was not written to env", name))
	must.Eq(t, TagDump, val.Tag)
	gd, ok := val.Data.(*GoroutineDump)
	must.True(t, ok)
	return gd
}
