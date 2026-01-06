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
