package evaluator

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shoenig/test/must"
)

func TestVM_BasicStackOps(t *testing.T) {
	vm, _ := NewVM(&vmConfig{cwd: t.TempDir()})

	vm.Push(Value{Tag: TagNumber, Data: 1})
	vm.Push(Value{Tag: TagNumber, Data: 2})
	vm.Push(Value{Tag: TagNumber, Data: 3})

	val, err := vm.Peek()
	must.NoError(t, err)
	must.Eq(t, 3, val.Data)

	val, err = vm.Pop()
	must.NoError(t, err)
	must.Eq(t, 3, val.Data)

	val, err = vm.Peek()
	must.NoError(t, err)
	must.Eq(t, 2, val.Data)
}

func TestVM_SimpleWhere(t *testing.T) {

	// source: `g1 = g where duration > 10`
	chunk := &Chunk{
		ops: []Op{
			encode(OpCodeLoadGoroutineDump, 1), // load g
			encode(OpCodeTempDump, 0),          // start
			encode(OpCodeNextGoroutine, 9),     // addr when done
			encode(OpCodeLoadFieldAccessor, 2), // load .duration
			encode(OpCodeLoadNumber, 3),        // load 10
			encode(OpCodeGreater, 0),           // compare
			encode(OpCodeJumpIfFalse, 2),       // addr if false
			encode(OpCodeAddGoroutine, 0),      // keep
			encode(OpCodeJumpTo, 2),            // unconditional jump to addr
			encode(OpCodePushDump, 0),          // push temp dump to stack
			encode(OpCodeAssignment, 0),
		},
		constants: []any{"g1", "g", ".duration", 10},
	}
	vm, _ := NewVM(&vmConfig{cwd: t.TempDir()})
	vm.reset(chunk)

	gd := &GoroutineDump{}
	gd.Add(&Goroutine{ID: 1, Duration: 20, State: "select"})
	gd.Add(&Goroutine{ID: 2, Duration: 0, State: "running"})

	vm.env = map[string]Value{
		"g": {Tag: TagDump, Data: gd},
	}

	result, err := vm.run()
	vm.debug()
	must.NoError(t, err)

	g1, ok := vm.env["g1"]
	must.True(t, ok, must.Sprint("g1 was not written to env"))

	must.Eq(t, g1, result)

	must.Eq(t, TagDump, g1.Tag)
	gd1, ok := g1.Data.(*GoroutineDump)
	must.True(t, ok)
	must.Eq(t, 1, gd1.Len())
	g := gd1.Next()
	must.NotNil(t, g)
	must.Eq(t, 1, g.ID)
	must.Eq(t, "select", g.State)
}

func TestVM_NestedBinaryExpression(t *testing.T) {
	// source: `g1 union (g2 where .duration > 10)
	chunk := &Chunk{
		ops: []Op{
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
		},
		constants: []any{"g1", "g2", ".duration", 10},
	}
	vm, _ := NewVM(&vmConfig{cwd: t.TempDir()})
	vm.reset(chunk)

	g1 := &GoroutineDump{}
	g1.Add(&Goroutine{ID: 1, Duration: 20, State: "select"})
	g1.Add(&Goroutine{ID: 2, Duration: 0, State: "running"})
	g2 := &GoroutineDump{}
	g2.Add(&Goroutine{ID: 3, Duration: 20, State: "IO wait"})
	g2.Add(&Goroutine{ID: 4, Duration: 0, State: "chan receive"})
	g2.Add(&Goroutine{ID: 5, Duration: 0, State: "running"})

	vm.env = map[string]Value{
		"g1": {Tag: TagDump, Data: g1},
		"g2": {Tag: TagDump, Data: g2},
	}

	result, err := vm.run()
	vm.debug()
	must.NoError(t, err)

	must.Eq(t, TagDump, result.Tag)
	g3, ok := result.Data.(*GoroutineDump)
	must.True(t, ok)
	must.Eq(t, 3, g3.Len())
	g3.StartIter()
	must.Eq(t, 3, g3.Next().ID)
	must.Eq(t, 1, g3.Next().ID)
	must.Eq(t, 2, g3.Next().ID)
}

func TestVM_DemoFunction(t *testing.T) {
	vm, _ := NewVM(&vmConfig{cwd: t.TempDir()})
	vm.stack = []Value{
		{Tag: TagBool, Data: nil},
	}

	for {
		val, err := vm.Pop()
		if err != nil {
			if errors.Is(ErrEmptyStack, err) {
				break
			}
			must.NoError(t, err)
		}
		fmt.Printf("%s: %#v\n", val.Tag, val.Data)
	}

}
