// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

func TestCompiler_SimplePipeline(t *testing.T) {
	t.Parallel()
	src := `g2 = g.where(.state == "select") | where(.duration > 10)`

	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 3, code.chunks)
	must.Eq(t, []any{"g2", "g", ".state", "select", ".duration", 10},
		code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g1
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 9),     // 02 addr when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 jump to next goroutine
		encode(OpCodeCall, 2),              // 05 call 2
		encode(OpCodeJumpIfFalse, 2),       // 06 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 07 keep
		encode(OpCodeJumpTo, 2),            // 08 jump to next goroutine
		encode(OpCodePushDump, 0),          // 09 push to stack
		encode(OpCodeAssignment, 0),        // 10 assign to g2
	},
		code.chunks[0].ops,
		must.Sprintf("%s", code.disassemble(0, 0)),
	)
	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 2), // 00 load .state
		encode(OpCodeLoadString, 3),        // 01 load "select"
		encode(OpCodeEqual, 0),             // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[1].ops,
		must.Sprintf("%s", code.disassemble(1, 0)),
	)
	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 4), // 00 load .duration
		encode(OpCodeLoadNumber, 5),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[2].ops,
		must.Sprintf("%s", code.disassemble(2, 0)),
	)
}

func TestCompiler_MultiPipeline(t *testing.T) {
	t.Parallel()
	src := `g3 = g1.where(.state == "select") |
                    where(.duration > 10 and .trace contains "keepAlive") |
                    delete(.trace contains "gRPC")`

	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 4, code.chunks)
	must.Eq(t, []any{
		"g3", "g1", ".state", "select", ".duration", 10,
		".trace", "keepAlive", "gRPC"},
		code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g1
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 11),    // 02 addr when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 jump to next goroutine
		encode(OpCodeCall, 2),              // 05 call 3
		encode(OpCodeJumpIfFalse, 2),       // 06 jump to next goroutine
		encode(OpCodeCall, 3),              // 07 call 3
		encode(OpCodeJumpIfTrue, 2),        // 08 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 09 add goroutine
		encode(OpCodeJumpTo, 2),            // 10 next goroutine
		encode(OpCodePushDump, 0),          // 11 push to stack
		encode(OpCodeAssignment, 0),        // 12 assign to g3
	},
		code.chunks[0].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 2), // 00 load .state
		encode(OpCodeLoadString, 3),        // 01 load "select"
		encode(OpCodeEqual, 0),             // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[1].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 4), // 00 load .duration
		encode(OpCodeLoadNumber, 5),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeJumpIfTrue, 6),        // 03 jump to next expr in "and"
		encode(OpCodePushBool, 0),          // 04 push false
		encode(OpCodeJumpTo, 9),            // 05 jump to end of "and"
		encode(OpCodeLoadFieldAccessor, 6), // 06 load .trace
		encode(OpCodeLoadString, 7),        // 07 load "keepAlive"
		encode(OpCodeContains, 0),          // 08 compare push bool to stack
		encode(OpCodeReturn, 0),            // 09 return
	},
		code.chunks[2].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 6), // 00 load .trace
		encode(OpCodeLoadString, 8),        // 01 load "gRPC"
		encode(OpCodeContains, 0),          // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[3].ops,
	)
}

func TestCompiler_SimpleWhere(t *testing.T) {
	t.Parallel()
	src := `g1 = g.where(.duration > 10)`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(code.disassemble(0, 0))
	must.Eq(t, []any{"g1", "g", ".duration", 10}, code.constants)
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 setup scratch register
		encode(OpCodeNextGoroutine, 7),     // 02 addr when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 addr if false
		encode(OpCodeAddGoroutine, 0),      // 05 keep
		encode(OpCodeJumpTo, 2),            // 06 unconditional jump to addr
		encode(OpCodePushDump, 0),          // 07 push temp dump to stack
		encode(OpCodeAssignment, 0),        // 08 assign to g1
	},
		code.chunks[0].ops,
	)
	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 2), // 00 load .duration
		encode(OpCodeLoadNumber, 3),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 04 return
	},
		code.chunks[1].ops,
	)
}

func TestCompiler_JumpPatch(t *testing.T) {
	t.Parallel()
	compiler := NewCompiler()
	compiler.chunk = NewChunk()
	compiler.tokenizer = NewTokenizer()
	addr := compiler.emitBytes(OpCodeJumpIfTrue, 0)
	compiler.emitByte(OpCodeNoop)
	compiler.emitByte(OpCodeNoop)
	compiler.emitByte(OpCodeNoop)
	compiler.patchJump(addr, 0)

	fmt.Println(compiler.code.disassemble(0, 0))
	jumpOp, jumpAddr := compiler.chunk.ops[0].decode()
	must.Eq(t, OpCodeJumpIfTrue, jumpOp)
	must.Eq(t, 3, jumpAddr)
	must.Len(t, 4, compiler.chunk.ops)
}

func TestCompiler_CompoundWhere(t *testing.T) {
	t.Parallel()
	src := `g.where(.duration > 10 and .state == "select")`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 2, code.chunks)
	must.Eq(t, []any{"g", ".duration", 10, ".state", "select"}, code.constants)
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 7),     // 02 next w/ addr to jump when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 skip + jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 05 keep
		encode(OpCodeJumpTo, 2),            // 06 jump to next goroutine
		encode(OpCodePushDump, 0),          // 07 push to stack
	},
		code.chunks[0].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 1), // 00 load .duration
		encode(OpCodeLoadNumber, 2),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeJumpIfTrue, 6),        // 03 jump to next expr in "and"
		encode(OpCodePushBool, 0),          // 04 push false
		encode(OpCodeJumpTo, 9),            // 05 jump to end of "and"
		encode(OpCodeLoadFieldAccessor, 3), // 06 load .state
		encode(OpCodeLoadString, 4),        // 07 load "select"
		encode(OpCodeEqual, 0),             // 08 compare push bool to stack
		encode(OpCodeReturn, 0),            // 09 return
	},
		code.chunks[1].ops,
	)
}

func TestCompiler_ParentheticalWhere(t *testing.T) {
	t.Parallel()
	src := `g.where((.duration > 10 and .state == "select")
                    or .state == "running")`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 2, code.chunks)
	must.Eq(t, []any{
		"g", ".duration", 10, ".state", "select", "running"},
		code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 7),     // 02 addr to jump to when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 skip + goto next goroutine
		encode(OpCodeAddGoroutine, 0),      // 05 keep this goroutine
		encode(OpCodeJumpTo, 2),            // 06 unconditional jump
		encode(OpCodePushDump, 0),          // 07 push to stack
	},
		code.chunks[0].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 1), // 00 load .duration
		encode(OpCodeLoadNumber, 2),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeJumpIfTrue, 6),        // 03 jump to next expr in "and"
		encode(OpCodePushBool, 0),          // 04 push false
		encode(OpCodeJumpTo, 9),            // 05 jump to end of "and"
		encode(OpCodeLoadFieldAccessor, 3), // 06 load .state
		encode(OpCodeLoadString, 4),        // 07 load "select"
		encode(OpCodeEqual, 0),             // 08 compare push bool to stack
		encode(OpCodeJumpIfFalse, 12),      // 09 jump to next expr in "or"
		encode(OpCodePushBool, 1),          // 10 push true
		encode(OpCodeJumpTo, 15),           // 11 jump to end of "or"
		encode(OpCodeLoadFieldAccessor, 3), // 12 load .state
		encode(OpCodeLoadString, 5),        // 13 load "running"
		encode(OpCodeEqual, 0),             // 14 compare push bool to stack
		encode(OpCodeReturn, 0),            // 15 return

	}, code.chunks[1].ops)
}

func TestCompiler_NestedExpressions(t *testing.T) {
	t.Parallel()
	src := `g1.union(g2.where(.duration > 10)) | show()`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 2, code.chunks)
	must.Eq(t, []any{"g1", "g2", ".duration", 10, 0}, code.constants)
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
		encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
		encode(OpCodeTempDump, 0),          // 02 setup scratch register
		encode(OpCodeNextGoroutine, 8),     // 03 addr when done
		encode(OpCodeCall, 1),              // 04 call 1
		encode(OpCodeJumpIfFalse, 3),       // 05 addr if false
		encode(OpCodeAddGoroutine, 0),      // 06 keep
		encode(OpCodeJumpTo, 3),            // 07 unconditional jump to addr
		encode(OpCodePushDump, 0),          // 08 push temp dump to stack
		encode(OpCodeFuncUnion, 0),         // 09 union
		encode(OpCodeLoadNumber, 4),        // 10 load 0
		encode(OpCodeLoadNumber, 4),        // 11 load 0
		encode(OpCodeFuncShowDump, 0),      // 12 show
	},
		code.chunks[0].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 2), // 00 load .duration
		encode(OpCodeLoadNumber, 3),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[1].ops,
	)
}

func TestCompiler_ChainedWhere(t *testing.T) {
	t.Parallel()
	src := `g.where(.duration > 10).where(.state == "select")`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 3, code.chunks)
	must.Eq(t, []any{"g", ".duration", 10, ".state", "select"}, code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 9),     // 02 next w/ addr to jump when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 jump to next goroutine
		encode(OpCodeCall, 2),              // 05 call 2
		encode(OpCodeJumpIfFalse, 2),       // 06 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 07 keep
		encode(OpCodeJumpTo, 2),            // 08 jump to next goroutine
		encode(OpCodePushDump, 0),          // 09 push to stack
	},
		code.chunks[0].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 1), // 00 load .duration
		encode(OpCodeLoadNumber, 2),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[1].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 3), // 00 load .state
		encode(OpCodeLoadString, 4),        // 01 load "select"
		encode(OpCodeEqual, 0),             // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[2].ops,
	)
}

func TestCompiler_Labels(t *testing.T) {
	t.Parallel()
	src := `g1 = g.where("foo" in labels and labels.worker_id == "bar")`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	must.NoError(t, err, must.Sprint(code.disassemble(0, 0)))

	fmt.Println(code.disassemble(0, 0))
	must.Eq(t, []any{"g1", "g", "foo", ".labels", ".labels.worker_id", "bar"},
		code.constants)
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 setup scratch register
		encode(OpCodeNextGoroutine, 7),     // 02 addr when done
		encode(OpCodeCall, 1),              // 03 call 1
		encode(OpCodeJumpIfFalse, 2),       // 04 addr if false
		encode(OpCodeAddGoroutine, 0),      // 05 keep
		encode(OpCodeJumpTo, 2),            // 06 unconditional jump to addr
		encode(OpCodePushDump, 0),          // 07 push temp dump to stack
		encode(OpCodeAssignment, 0),        // 08 assign to g1
	},
		code.chunks[0].ops,
	)
	must.Eq(t, []Op{
		encode(OpCodeLoadString, 2),        // 00 load "foo"
		encode(OpCodeLoadFieldAccessor, 3), // 01 load .labels
		encode(OpCodeIn, 0),                // 02 compare in, push bool to stack
		encode(OpCodeJumpIfTrue, 6),        // 03 skip to next clause if true
		encode(OpCodePushBool, 0),          // 04 push false
		encode(OpCodeJumpTo, 9),            // 05 unconditional jump to return
		encode(OpCodeLoadFieldAccessor, 4), // 06 load .labels.worker_id
		encode(OpCodeLoadString, 5),        // 07 load "bar"
		encode(OpCodeEqual, 0),             // 08 compare push bool to stack
		encode(OpCodeReturn, 0),            // 09 return
	},
		code.chunks[1].ops,
	)
}

func TestCompiler_InGraph(t *testing.T) {
	t.Parallel()
	src := `g1 = g.graph(.duration > 10)`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	fmt.Println(code.disassemble(0, 0))
	must.NoError(t, err)

	must.Len(t, 2, code.chunks)
	must.Eq(t, []any{"g1", "g", ".duration", 10}, code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g
		encode(OpCodeDup, 0),               // 01 dup g on stack
		encode(OpCodeTempDump, 0),          // 02 setup scratch register
		encode(OpCodeNextGoroutine, 8),     // 03 addr when done
		encode(OpCodeCall, 1),              // 04 call 1
		encode(OpCodeJumpIfFalse, 3),       // 05 addr if false
		encode(OpCodeAddGoroutine, 0),      // 06 keep
		encode(OpCodeJumpTo, 3),            // 07 unconditional jump to addr
		encode(OpCodePushDump, 0),          // 08 push temp dump to stack
		encode(OpCodeFuncGraph, 0),         // 09 generate graph and push to stack
		encode(OpCodeAssignment, 0),        // 10 assign to g1
	},
		code.chunks[0].ops,
	)

	must.Eq(t, []Op{
		encode(OpCodeLoadFieldAccessor, 2), // 00 load .duration
		encode(OpCodeLoadNumber, 3),        // 01 load 10
		encode(OpCodeGreater, 0),           // 02 compare push bool to stack
		encode(OpCodeReturn, 0),            // 03 return
	},
		code.chunks[1].ops,
	)
}

func TestCompiler_Paths(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		src        string
		expect     []Op
		expectPath string
		expectErr  string
	}{
		{
			name:      "unquoted with spaces",
			src:       `cd(/path to directory)`,
			expectErr: `invalid argument for cd`,
		},
		{
			name:      "unquoted without spaces",
			src:       `cd(/path/to/direct.ory)`,
			expectErr: `invalid argument for cd`,
		},
		{
			name: "quoted with spaces",
			src:  `cd("/path to directory")`,
			expect: []Op{
				encode(OpCodeLoadString, 0),
				encode(OpCodeCommandChangeDir, 0), // cd
			},
			expectPath: `/path to directory`,
		},
		{
			name: "quoted piped",
			src:  `load("/path/to/dump.txt") | show(100, 10)`,
			expect: []Op{
				encode(OpCodeLoadString, 0),
				encode(OpCodeFuncLoad, 0),     // load
				encode(OpCodeLoadNumber, 1),   // 100
				encode(OpCodeLoadNumber, 2),   // 10
				encode(OpCodeFuncShowDump, 0), // show
			},
			expectPath: `/path/to/dump.txt`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(t.Context(), body)
			compiler := NewCompiler()
			code, err := compiler.Compile(tokenizer)
			if tc.expectErr != "" {
				must.ErrorContains(t, err, tc.expectErr)
				return
			}

			must.NoError(t, err)

			chunk := code.chunks[0]
			fmt.Println(code.disassemble(0, 0))
			must.Eq(t, tc.expect, chunk.ops)
			must.Eq(t, tc.expectPath,
				code.constants[0].(string)) //nolint:errcheck
		})
	}
}

func TestCompiler_DiffMultiAssign(t *testing.T) {
	t.Parallel()
	src := `g3, g4, g5 = g1.diff(g2)`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(code.disassemble(0, 0))

	must.Eq(t, []any{
		"g3", "g4", "g5", "g1", "g2", MultiAssignment{0, 1, 2}},
		code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 3), // 00 load g1
		encode(OpCodeLoadGoroutineDump, 4), // 01 load g2
		encode(OpCodeFuncDiff, 0),          // 02 diff func
		encode(OpCodeAssignment, 5),        // 03 multi-assign g3, g4, g5
	}, code.chunks[0].ops)
}

func TestCompiler_NoAssign(t *testing.T) {
	t.Parallel()
	src := `g1, g2`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	compiler := NewCompiler()
	code, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(code.disassemble(0, 0))

	must.Eq(t, []any{"g1", "g2"}, code.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
		encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
	}, code.chunks[0].ops)
}

func TestCompiler_Show(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name         string
		src          string
		expectLimit  int
		expectOffset int
	}{
		{
			name:         "no args",
			src:          `g1.show()`,
			expectLimit:  0,
			expectOffset: 0,
		},
		{
			name:         "offset only",
			src:          `g1 | show(0, 3)`,
			expectLimit:  0,
			expectOffset: 3,
		},
		{
			name:         "limit only",
			src:          `g1.show(3)`,
			expectLimit:  3,
			expectOffset: 0,
		},
		{
			name:         "limit and offset",
			src:          `g1 | show(10, 3)`,
			expectLimit:  10,
			expectOffset: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(t.Context(), body)
			compiler := NewCompiler()
			code, err := compiler.Compile(tokenizer)
			must.NoError(t, err)

			fmt.Println(code.disassemble(0, 0))
			chunk := code.chunks[0]
			must.Len(t, 4, chunk.ops)

			_, operand := chunk.ops[1].decode()
			must.Eq(t, tc.expectLimit,
				code.constants[operand].(int)) //nolint:errcheck

			_, operand = chunk.ops[2].decode()
			must.Eq(t, tc.expectOffset,
				code.constants[operand].(int)) //nolint:errcheck
		})
	}

}

func TestCompiler_Pragma(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		src           string
		expectSetting string
		expectValue   Op
	}{
		{
			name:          "boolean",
			src:           `pragma.empty.confirm = true`,
			expectSetting: "empty.confirm",
			expectValue:   encode(OpCodePushBool, 1),
		},
		{
			name:          "numeric",
			src:           `pragma.show.count = 100`,
			expectSetting: "show.count",
			expectValue:   encode(OpCodeLoadNumber, 0),
		},
		{
			name:          "enum",
			src:           `pragma.vars.display = summary`,
			expectSetting: "vars.display",
			expectValue:   encode(OpCodeLoadString, 0),
		},
		{
			name:          "get all",
			src:           `pragma`,
			expectSetting: "*.*",
		},
		{
			name:          "get some",
			src:           `pragma.limits`,
			expectSetting: "limits.*",
		},
		{
			name:          "get specific",
			src:           `pragma.limits.steps`,
			expectSetting: "limits.steps",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(t.Context(), body)
			compiler := NewCompiler()
			code, err := compiler.Compile(tokenizer)
			must.NoError(t, err)

			chunk := code.chunks[0]
			fmt.Println(code.disassemble(0, 0))

			_, operand := chunk.ops[1].decode()
			must.Eq(t, tc.expectSetting,
				code.constants[operand].(string)) //nolint:errcheck

			if tc.expectValue == Op(OpCodeNoop) {
				must.Len(t, 2, chunk.ops)
			} else {
				must.Len(t, 3, chunk.ops)
				must.Eq(t, tc.expectValue, chunk.ops[0])
			}
		})
	}

}

func TestCompiler_Errors(t *testing.T) {
	t.Parallel()

	tokenizer := NewTokenizer()
	compiler := NewCompiler()

	testCases := []struct {
		src       string
		expectPos int
		expectErr string
	}{
		{
			`!`, 1, // pos at !
			`expected expression to start with an identifier or open paren`,
		},
		{
			`g = load(1)`, 10, // pos at 1
			`expected string got number`,
		},
		{
			`g = load`, 9, // no pos
			`expected left paren, got error EOF`,
		},
		{
			`g.where(.)`, 9, // pos at .
			`invalid identifier`,
		},
		{
			"pragma.show.dedup = `foo`", 21, // pos foo
			`invalid pragma value: expected one of "ids", "number", or "none"`,
		},
	}

	for _, tc := range testCases {
		body := strings.NewReader(tc.src)
		tokenizer.Reset(t.Context(), body)
		_, err := compiler.Compile(tokenizer)
		must.NotNil(t, err)

		var cerr ErrorWithPosition
		must.True(t, errors.As(err, &cerr))
		must.EqError(t, cerr, tc.expectErr)
		must.Eq(t, tc.expectPos, cerr.pos.Column)
	}
}

func TestCompiler_PatchOut(t *testing.T) {
	t.Parallel()

	start := uint(3)
	by := uint(4)

	chunk := &Chunk{
		ops: []Op{
			encode(OpCodeNoop, 0),          // 00
			encode(OpCodeNoop, 1),          // 01
			encode(OpCodeNoop, 2),          // 02
			encode(OpCodeNoop, 3),          // 03 start of window
			encode(OpCodeNoop, 4),          // 04
			encode(OpCodeJumpTo, 1),        // 05 jump within window to before start
			encode(OpCodeNoop, 6),          // 06 end of window
			encode(OpCodeNoop, 7),          // 07
			encode(OpCodeNextGoroutine, 4), // 08 jump to within window
			encode(OpCodeJumpIfFalse, 1),   // 09 jump to before start
			encode(OpCodeJumpIfTrue, 11),   // 10 jump to after window
			encode(OpCodeNoop, 12),         // 11
		},
	}
	compiler := NewCompiler()
	err := compiler.patchOut(chunk, start, by)
	must.EqError(t, err, "patch out found address inside patched window (op=04 addr=4)")
	must.Len(t, 12, chunk.ops)
	must.Eq(t, encode(OpCodeNoop, 4), chunk.ops[4])

	// remove offending op
	chunk.ops[8] = encode(OpCodeNoop, 8)
	err = compiler.patchOut(chunk, start, by)
	must.NoError(t, err)

	code := NewCode()
	code.chunks = append(code.chunks, chunk)
	fmt.Println(code.disassemble(0, 0))

	must.Eq(t, []Op{
		encode(OpCodeNoop, 0),        // 00
		encode(OpCodeNoop, 1),        // 01
		encode(OpCodeNoop, 2),        // 02
		encode(OpCodeNoop, 7),        // 03 (was 07)
		encode(OpCodeNoop, 8),        // 04 (was 08)
		encode(OpCodeJumpIfFalse, 1), // 05 (was 09) jump to before start
		encode(OpCodeJumpIfTrue, 7),  // 06 (was 10) jump to after window
		encode(OpCodeNoop, 12),       // 07 (was 11)
	}, chunk.ops)
}
