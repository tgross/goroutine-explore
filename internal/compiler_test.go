// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

func TestCompiler_SimplePipeline(t *testing.T) {
	src := `g2 = g.where(.state == "select") | where(.duration > 10)`

	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(chunk.disassemble(0))
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g1
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 9),     // 02 addr when done
		encode(OpCodeLoadFieldAccessor, 2), // 03 load .state
		encode(OpCodeLoadString, 3),        // 04 load "select"
		encode(OpCodeEqual, 0),             // 05 compare push bool to stack
		encode(OpCodeJumpIfFalse, 2),       // 06 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 07 keep
		encode(OpCodeJumpTo, 2),            // 08 jump to next goroutine
		encode(OpCodePushDump, 0),          // 09 push to stack
		encode(OpCodeTempDump, 0),          // 10 refresh scratch register
		encode(OpCodeNextGoroutine, 18),    // 11 addr when done
		encode(OpCodeLoadFieldAccessor, 4), // 12 load .duration
		encode(OpCodeLoadNumber, 5),        // 13 load 10
		encode(OpCodeGreater, 0),           // 14 compare push bool to stack
		encode(OpCodeJumpIfFalse, 11),      // 15 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 16 keep
		encode(OpCodeJumpTo, 11),           // 17 jump to next goroutine
		encode(OpCodePushDump, 0),          // 18 push to stack
		encode(OpCodeAssignment, 0),        // 19 push to stack
	},
		chunk.ops,
	)
}

func TestCompiler_MultiPipeline(t *testing.T) {

	src := `g3 = g1.where(.state == "select") |
                    where(.duration > 10 and .trace contains "keepAlive") |
                    delete(.trace contains "gRPC")`

	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(chunk.disassemble(0))

	must.Eq(t, []any{
		"g3", "g1", ".state", "select", ".duration", 10,
		".trace", "keepAlive", "gRPC"}, chunk.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g1
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 9),     // 02 addr when done
		encode(OpCodeLoadFieldAccessor, 2), // 03 load .state
		encode(OpCodeLoadString, 3),        // 04 load "select"
		encode(OpCodeEqual, 0),             // 05 compare push bool to stack
		encode(OpCodeJumpIfFalse, 2),       // 06 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 07 keep
		encode(OpCodeJumpTo, 2),            // 08 jump to next goroutine
		encode(OpCodePushDump, 0),          // 09 push to stack
		encode(OpCodeTempDump, 0),          // 10 refresh scratch register
		encode(OpCodeNextGoroutine, 24),    // 11 addr when done
		encode(OpCodeLoadFieldAccessor, 4), // 12 load .duration
		encode(OpCodeLoadNumber, 5),        // 13 load 10
		encode(OpCodeGreater, 0),           // 14 compare push bool to stack
		encode(OpCodeJumpIfTrue, 18),       // 15 jump to next expr in "and"
		encode(OpCodePushBool, 0),          // 16 push false
		encode(OpCodeJumpTo, 21),           // 17 jump to end of "and"
		encode(OpCodeLoadFieldAccessor, 6), // 18 load .trace
		encode(OpCodeLoadString, 7),        // 19 load "keepAlive"
		encode(OpCodeContains, 0),          // 20 compare push bool to stack
		encode(OpCodeJumpIfFalse, 11),      // 21 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 22 keep
		encode(OpCodeJumpTo, 11),           // 23 jump to next goroutine
		encode(OpCodePushDump, 0),          // 24 push to stack
		encode(OpCodeTempDump, 0),          // 25 refresh scratch register
		encode(OpCodeNextGoroutine, 33),    // 26 addr when done
		encode(OpCodeLoadFieldAccessor, 6), // 27 load .trace
		encode(OpCodeLoadString, 8),        // 28 load "gRPC"
		encode(OpCodeContains, 0),          // 29 compare push bool to stack
		encode(OpCodeJumpIfTrue, 26),       // 30 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 31 keep
		encode(OpCodeJumpTo, 26),           // 32 jump to next goroutine
		encode(OpCodePushDump, 0),          // 33 push to stack
		encode(OpCodeAssignment, 0),        // 34 push to stack
	},
		chunk.ops,
	)
}

func TestCompiler_SimpleWhere(t *testing.T) {
	src := `g1 = g.where(.duration > 10)`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(chunk.disassemble(0))
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 1), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 setup scratch register
		encode(OpCodeNextGoroutine, 9),     // 02 addr when done
		encode(OpCodeLoadFieldAccessor, 2), // 03 load .duration
		encode(OpCodeLoadNumber, 3),        // 04 load 10
		encode(OpCodeGreater, 0),           // 05 compare push bool to stack
		encode(OpCodeJumpIfFalse, 2),       // 06 addr if false
		encode(OpCodeAddGoroutine, 0),      // 07 keep
		encode(OpCodeJumpTo, 2),            // 08 unconditional jump to addr
		encode(OpCodePushDump, 0),          // 09 push temp dump to stack
		encode(OpCodeAssignment, 0),        // 10 assign to g1
	},
		chunk.ops,
	)
}

func TestCompiler_JumpPatch(t *testing.T) {
	compiler := NewCompiler()
	compiler.chunk = NewChunk()
	addr := compiler.emitBytes(OpCodeJumpIfTrue, 0)
	compiler.emitByte(OpCodeNoop)
	compiler.emitByte(OpCodeNoop)
	compiler.emitByte(OpCodeNoop)
	compiler.patchJump(addr, 0)

	fmt.Println(compiler.chunk.disassemble(0))
	jumpOp, jumpAddr := compiler.chunk.ops[0].decode()
	must.Eq(t, OpCodeJumpIfTrue, jumpOp)
	must.Eq(t, 3, jumpAddr)
	must.Len(t, 4, compiler.chunk.ops)
}

func TestCompiler_PipelineEquivalence(t *testing.T) {
	t.Skip("TODO: these are not actually equivalent but we should probably have a test that shows a pipeline and a series of 'ands' have the equivalent *output*")
	src1 := `g2 = g1 where .state == "select" where .duration > 1`
	src2 := `g2 = g1 | where .state == "select" | where .duration > 1`

	compiler := NewCompiler()

	body := strings.NewReader(src1)
	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)
	fmt.Println(chunk.disassemble(0))

	expect := chunk.ops

	body = strings.NewReader(src2)
	tokenizer.Reset(context.TODO(), body)
	chunk, err = compiler.Compile(tokenizer)
	must.NoError(t, err)
	fmt.Println(chunk.disassemble(0))
	must.Eq(t, expect, chunk.ops)
}

func TestCompiler_CompoundWhere(t *testing.T) {
	src := `g.where(.duration > 10 and .state == "select")`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(chunk.disassemble(0))
	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 15),    // 02 next w/ addr to jump when done
		encode(OpCodeLoadFieldAccessor, 1), // 03 load .duration
		encode(OpCodeLoadNumber, 2),        // 04 load 2
		encode(OpCodeGreater, 0),           // 05 compare push bool to stack
		encode(OpCodeJumpIfTrue, 9),        // 06 jump to next expr in "and"
		encode(OpCodePushBool, 0),          // 07 push false
		encode(OpCodeJumpTo, 12),           // 08 jump to end of "and"
		encode(OpCodeLoadFieldAccessor, 3), // 09 load .state
		encode(OpCodeLoadString, 4),        // 10 load "select"
		encode(OpCodeEqual, 0),             // 11 compare push bool to stack
		encode(OpCodeJumpIfFalse, 2),       // 12 skip + jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 13 keep
		encode(OpCodeJumpTo, 2),            // 14 jump to next goroutine
		encode(OpCodePushDump, 0),          // 15 push to stack
	},
		chunk.ops,
	)
}

func TestCompiler_ParentheticalWhere(t *testing.T) {
	src := `g.where((.duration > 10 and .state == "select")
                    or .state == "running")`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	fmt.Println(chunk.disassemble(0))
	must.NoError(t, err)

	must.Eq(t, []any{
		"g", ".duration", 10,
		".state", "select", "running"}, chunk.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g
		encode(OpCodeTempDump, 0),          // 01 scratch register
		encode(OpCodeNextGoroutine, 21),    // 02 addr to jump to when done
		encode(OpCodeLoadFieldAccessor, 1), // 03 load .duration
		encode(OpCodeLoadNumber, 2),        // 04 load 10
		encode(OpCodeGreater, 0),           // 05 compare push bool to stack
		encode(OpCodeJumpIfTrue, 9),        // 06 jump to next expr in "and"
		encode(OpCodePushBool, 0),          // 07 push false
		encode(OpCodeJumpTo, 12),           // 08 jump to end of "and"
		encode(OpCodeLoadFieldAccessor, 3), // 09 load .state
		encode(OpCodeLoadString, 4),        // 10 load "select"
		encode(OpCodeEqual, 0),             // 11 compare push bool to stack
		encode(OpCodeJumpIfFalse, 15),      // 12 jump to next expr in "or"
		encode(OpCodePushBool, 1),          // 13 push true
		encode(OpCodeJumpTo, 18),           // 14 jump to end of "or"
		encode(OpCodeLoadFieldAccessor, 3), // 15 load .state
		encode(OpCodeLoadString, 5),        // 16 load "running"
		encode(OpCodeEqual, 0),             // 17 compare push bool to stack
		encode(OpCodeJumpIfFalse, 2),       // 18 skip + goto next goroutine
		encode(OpCodeAddGoroutine, 0),      // 19 keep this goroutine
		encode(OpCodeJumpTo, 2),            // 20 unconditional jump
		encode(OpCodePushDump, 0),          // 21 push to stack
	},
		chunk.ops,
	)
}

func TestCompiler_NestedExpressions(t *testing.T) {
	src := `g1.union(g2.where(.duration > 10)) | show()`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	fmt.Println(chunk.disassemble(0))
	must.NoError(t, err)

	must.Eq(t, []any{"g1", "g2", ".duration", 10, 0}, chunk.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
		encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
		encode(OpCodeTempDump, 0),          // 02 setup scratch register
		encode(OpCodeNextGoroutine, 10),    // 03 addr when done
		encode(OpCodeLoadFieldAccessor, 2), // 04 load .duration
		encode(OpCodeLoadNumber, 3),        // 05 load 10
		encode(OpCodeGreater, 0),           // 06 compare push bool to stack
		encode(OpCodeJumpIfFalse, 3),       // 07 addr if false
		encode(OpCodeAddGoroutine, 0),      // 08 keep
		encode(OpCodeJumpTo, 3),            // 09 unconditional jump to addr
		encode(OpCodePushDump, 0),          // 10 push temp dump to stack
		encode(OpCodeFuncUnion, 0),         // 11 union
		encode(OpCodeLoadNumber, 4),        // 12 load 0
		encode(OpCodeLoadNumber, 4),        // 13 load 0
		encode(OpCodeFuncShowDump, 0),      // 14 show
	}, chunk.ops)
}

func TestCompiler_Paths(t *testing.T) {
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
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(context.TODO(), body)
			compiler := NewCompiler()
			chunk, err := compiler.Compile(tokenizer)
			if tc.expectErr != "" {
				must.ErrorContains(t, err, tc.expectErr)
				return
			}

			must.NoError(t, err)

			fmt.Println(chunk.disassemble(0))
			must.Eq(t, tc.expect, chunk.ops)
			must.Eq(t, tc.expectPath,
				chunk.constants[0].(string)) //nolint:errcheck
		})
	}
}

func TestCompiler_DiffMultiAssign(t *testing.T) {

	src := `g3, g4, g5 = g1.diff(g2)` //`| l, r, c = diff g2`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(chunk.disassemble(0))

	must.Eq(t, []any{
		"g3", "g4", "g5", "g1", "g2", MultiAssignment{0, 1, 2}},
		chunk.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 3), // 00 load g1
		encode(OpCodeLoadGoroutineDump, 4), // 01 load g2
		encode(OpCodeFuncDiff, 0),          // 02 diff func
		encode(OpCodeAssignment, 5),        // 03 multi-assign g3, g4, g5
	}, chunk.ops)
}

func TestCompiler_NoAssign(t *testing.T) {

	src := `g1, g2`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer()
	tokenizer.Reset(context.TODO(), body)
	compiler := NewCompiler()
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)

	fmt.Println(chunk.disassemble(0))

	must.Eq(t, []any{"g1", "g2"}, chunk.constants)

	must.Eq(t, []Op{
		encode(OpCodeLoadGoroutineDump, 0), // 00 load g1
		encode(OpCodeLoadGoroutineDump, 1), // 01 load g2
	}, chunk.ops)
}

func TestCompiler_Show(t *testing.T) {
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
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(context.TODO(), body)
			compiler := NewCompiler()
			chunk, err := compiler.Compile(tokenizer)
			must.NoError(t, err)

			fmt.Println(chunk.disassemble(0))
			must.Len(t, 4, chunk.ops)

			_, operand := chunk.ops[1].decode()
			must.Eq(t, tc.expectLimit,
				chunk.constants[operand].(int)) //nolint:errcheck

			_, operand = chunk.ops[2].decode()
			must.Eq(t, tc.expectOffset,
				chunk.constants[operand].(int)) //nolint:errcheck
		})
	}

}

func TestCompiler_Pragma(t *testing.T) {
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
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(context.TODO(), body)
			compiler := NewCompiler()
			chunk, err := compiler.Compile(tokenizer)
			must.NoError(t, err)

			fmt.Println(chunk.disassemble(0))

			_, operand := chunk.ops[1].decode()
			must.Eq(t, tc.expectSetting,
				chunk.constants[operand].(string)) //nolint:errcheck

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

	tokenizer := NewTokenizer()
	compiler := NewCompiler()

	testCases := []struct {
		src          string
		expectLexeme string
		expectPos    int
		expectErr    string
	}{
		{
			`!`, `!`, 1,
			`expected expression to start with an identifier or open paren`,
		},
		{
			`g = load(1)`, `1`, 10,
			`expected string got number`,
		},
		{
			`g = load`, ``, 9,
			`expected left paren, got error EOF`,
		},
		{
			`g.where(.)`, `.`, 9,
			`invalid identifier`,
		},
		{
			"pragma.show.dedup = `foo`", `foo`, 21,
			`invalid pragma value: expected one of "ids", "number", or "none"`,
		},
	}

	for _, tc := range testCases {
		body := strings.NewReader(tc.src)
		tokenizer.Reset(context.TODO(), body)
		_, err := compiler.Compile(tokenizer)
		must.NotNil(t, err)

		var cerr CompilerError
		must.True(t, errors.As(err, &cerr))
		must.EqError(t, cerr, tc.expectErr)
		must.Eq(t, tc.expectLexeme, cerr.tok.Lexeme)
		must.Eq(t, tc.expectPos, cerr.tok.Pos.Column)
	}
}
