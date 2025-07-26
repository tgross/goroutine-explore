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
		encode(OpCodeNextGoroutine, 22),    // 11 addr when done
		encode(OpCodeLoadFieldAccessor, 4), // 12 load .duration
		encode(OpCodeLoadNumber, 5),        // 13 load 10
		encode(OpCodeGreater, 0),           // 14 compare push bool to stack
		encode(OpCodeLoadFieldAccessor, 6), // 15 load .trace
		encode(OpCodeLoadString, 7),        // 16 load "keepAlive"
		encode(OpCodeContains, 0),          // 17 compare push bool to stack
		encode(OpCodeAnd, 0),               // 18 compare 2 from stack
		encode(OpCodeJumpIfFalse, 11),      // 19 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 20 keep
		encode(OpCodeJumpTo, 11),           // 21 jump to next goroutine
		encode(OpCodePushDump, 0),          // 22 push to stack
		encode(OpCodeTempDump, 0),          // 23 refresh scratch register
		encode(OpCodeNextGoroutine, 31),    // 24 addr when done
		encode(OpCodeLoadFieldAccessor, 8), // 25 load .trace
		encode(OpCodeLoadString, 9),        // 26 load "gRPC"
		encode(OpCodeContains, 0),          // 27 compare push bool to stack
		encode(OpCodeJumpIfTrue, 24),       // 28 jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 29 keep
		encode(OpCodeJumpTo, 24),           // 30 jump to next goroutine
		encode(OpCodePushDump, 0),          // 31 push to stack
		encode(OpCodeAssignment, 0),        // 31 push to stack
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
	addr := compiler.emitJump(OpCodeJumpIfTrue)
	compiler.emitByte(OpCode(0))
	compiler.emitByte(OpCode(0))
	compiler.emitByte(OpCode(0))
	compiler.patchJump(addr)

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
		encode(OpCodeNextGoroutine, 13),    // 02 next w/ addr to jump when done
		encode(OpCodeLoadFieldAccessor, 1), // 03 load .duration
		encode(OpCodeLoadNumber, 2),        // 04 load 2
		encode(OpCodeGreater, 0),           // 05 compare push bool to stack
		encode(OpCodeLoadFieldAccessor, 3), // 06 load .state
		encode(OpCodeLoadString, 4),        // 07 load "select"
		encode(OpCodeEqual, 0),             // 08 compare push bool to stack
		encode(OpCodeAnd, 0),               // 09 compare 2 on stack
		encode(OpCodeJumpIfFalse, 2),       // 10 skip and jump to next goroutine
		encode(OpCodeAddGoroutine, 0),      // 11 keep
		encode(OpCodeJumpTo, 2),            // 12 jump to next goroutine
		encode(OpCodePushDump, 0),          // 13 push to stack
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
		encode(OpCodeNextGoroutine, 17),    // 02 addr to jump to when done
		encode(OpCodeLoadFieldAccessor, 1), // 03 load .duration
		encode(OpCodeLoadNumber, 2),        // 04 load 10
		encode(OpCodeGreater, 0),           // 05 compare push bool to stack
		encode(OpCodeLoadFieldAccessor, 3), // 06 load .state
		encode(OpCodeLoadString, 4),        // 07 load "select"
		encode(OpCodeEqual, 0),             // 08 compare push bool to stack
		encode(OpCodeAnd, 0),               // 09 compare 2 from stack
		encode(OpCodeLoadFieldAccessor, 5), // 10 load .state
		encode(OpCodeLoadString, 6),        // 11 load "running"
		encode(OpCodeEqual, 0),             // 12 compare push bool to stack
		encode(OpCodeOr, 0),                // 13 compare previous comparisons
		encode(OpCodeJumpIfFalse, 2),       // 14 skip, jump back to top of loop
		encode(OpCodeAddGoroutine, 0),      // 15 keep this goroutine
		encode(OpCodeJumpTo, 2),            // 16 unconditional jump
		encode(OpCodePushDump, 0),          // 17 push to stack
	},
		chunk.ops,
	)
}
