package evaluator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

func TestCompiler_MultiPipeline(t *testing.T) {

	src := `g3 = g1 where .state == "select" |
                    where .duration > 10 and .trace contains "keepAlive" |
                    delete .trace contains "gRPC"`

	body := strings.NewReader(src)

	tokenizer := NewTokenizer(body)
	compiler := newCompiler()
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
		encode(OpCodeLoadFieldAccessor, 8), // 27 load .trace
		encode(OpCodeLoadString, 9),        // 28 load "gRPC"
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
	src := `g1 = g where .duration > 10`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer(body)
	compiler := newCompiler()
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
	compiler := newCompiler()
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

	compiler := newCompiler()

	body := strings.NewReader(src1)
	tokenizer := NewTokenizer(body)
	chunk, err := compiler.Compile(tokenizer)
	must.NoError(t, err)
	fmt.Println(chunk.disassemble(0))

	expect := chunk.ops

	body = strings.NewReader(src2)
	tokenizer = NewTokenizer(body)
	chunk, err = compiler.Compile(tokenizer)
	must.NoError(t, err)
	fmt.Println(chunk.disassemble(0))
	must.Eq(t, expect, chunk.ops)
}

func TestCompiler_CompoundWhere(t *testing.T) {
	src := `g where .duration > 10 and .state == "select"`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer(body)
	compiler := newCompiler()
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
	src := `g where (.duration > 10 and .state == "select") or .state == "running"`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer(body)
	compiler := newCompiler()
	chunk, err := compiler.Compile(tokenizer)
	fmt.Println(chunk.disassemble(0))
	must.NoError(t, err)

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
		encode(OpCodeLoadFieldAccessor, 5), // 15 load .state
		encode(OpCodeLoadString, 6),        // 16 load "running"
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
	src := `g1 union (g2 where .duration > 10) | show`
	body := strings.NewReader(src)

	tokenizer := NewTokenizer(body)
	compiler := newCompiler()
	chunk, err := compiler.Compile(tokenizer)
	fmt.Println(chunk.disassemble(0))
	must.NoError(t, err)

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
	}, chunk.ops)
}

func TestCompiler_Paths(t *testing.T) {
	testCases := []struct {
		name       string
		src        string
		expect     []Op
		expectPath string
	}{
		{
			name: "unquoted with spaces",
			src:  `cd /path to directory`,
			expect: []Op{
				encode(OpCodeLoadString, 0),
				encode(OpCodeCommandChangeDir, 0), // cd
			},
			// TODO: having this parse rather than error kinda sucks
			expectPath: `/pathtodirectory`,
		},
		{
			name: "quoted with spaces",
			src:  `cd "/path to directory"`,
			expect: []Op{
				encode(OpCodeLoadString, 0),
				encode(OpCodeCommandChangeDir, 0), // cd
			},
			expectPath: `/path to directory`,
		},
		{
			name: "unquoted without spaces",
			src:  `cd /path/to/direct.ory`,
			expect: []Op{
				encode(OpCodeLoadString, 0),
				encode(OpCodeCommandChangeDir, 0), // cd
			},
			expectPath: `/path/to/direct.ory`,
		},
		{
			name: "unquoted piped",
			src:  `load /path/to/dump.txt | show 100 10`,
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
			tokenizer := NewTokenizer(body)
			compiler := newCompiler()
			chunk, err := compiler.Compile(tokenizer)
			must.NoError(t, err)

			fmt.Println(chunk.disassemble(0))
			must.Eq(t, tc.expect, chunk.ops)
			must.Eq(t, tc.expectPath, chunk.constants[0].(string))
		})
	}
}
