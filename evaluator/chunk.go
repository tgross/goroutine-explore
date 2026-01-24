package evaluator

import (
	"fmt"
	"strings"
)

type Chunk struct {
	ops       []Op
	constants []any
}

func NewChunk() *Chunk {
	return &Chunk{
		ops:       []Op{},
		constants: []any{},
	}
}

func (c *Chunk) disassemble(ip int) string {
	var b strings.Builder
	b.WriteString(".text\n")
	for instIndex, op := range c.ops {
		ipPrefix := "  "
		if instIndex == ip {
			ipPrefix = "* "
		}
		comment := "\t"
		code, idx := op.decode()
		switch code {
		case OpCodeAssignment, OpCodeLoadFieldAccessor,
			OpCodeLoadNumber, OpCodeLoadString, OpCodeLoadGoroutineDump:
			if idx <= uint(len(c.constants)) {
				if len(c.constants) > int(idx) {
					val := c.constants[idx]
					comment = fmt.Sprintf("\t// %v", val)
				}
			}
		}
		b.WriteString(fmt.Sprintf("%s[%02d] %s%s\n", ipPrefix, instIndex, op, comment))
	}
	b.WriteString(".data\n")
	if len(c.constants) == 0 {
		b.WriteString("  <none>\n")
	}
	for i, con := range c.constants {
		b.WriteString(fmt.Sprintf("  [%02d] %v\n", i, con))
	}

	return b.String()
}

var opCodeMask = Op(0xffff000000000000)

type OpCode uint

type Op uint

func encode(op OpCode, val uint) Op {
	return Op((op << 55) | OpCode(val))
}

func (code Op) decode() (OpCode, uint) {
	prefix := OpCode(code >> 55)
	val := uint(code &^ opCodeMask)
	return prefix, val
}

func (code Op) String() string {
	prefix, val := code.decode()
	return fmt.Sprintf("%s(%d)", prefix, val)
}

//go:generate stringer -type OpCode
const (
	OpCodeNoop OpCode = iota

	// loads (TODO: should we have a single untyped load op?)
	OpCodeLoadGoroutineDump
	OpCodeLoadFieldAccessor
	OpCodeLoadNumber
	OpCodeLoadString

	// stores
	OpCodeAssignment
	OpCodePushBool
	OpCodePushDump

	// filter iteration
	OpCodeAddGoroutine
	OpCodeNextGoroutine
	OpCodeTempDump

	// comparisons
	OpCodeContains
	OpCodeEqual
	OpCodeGreater
	OpCodeGreaterEqual // TODO: split into multiple op codes?
	OpCodeLess
	OpCodeLessEqual // TODO: split into multiple op codes?
	OpCodeNotEqual  // TODO: split into multiple op codes?

	// control flow
	OpCodeJumpIfFalse
	OpCodeJumpIfTrue
	OpCodeJumpTo
	OpCodePipe

	// functions
	OpCodeFuncDiff
	OpCodeFuncIntersect
	OpCodeFuncLoad
	OpCodeFuncSave
	OpCodeFuncShowDump
	OpCodeFuncUnion

	// commands
	OpCodeCommandChangeDir
	OpCodeCommandEmpty
	OpCodeCommandGetWorkingDir
	OpCodeCommandHelp
	OpCodeCommandListDir
	OpCodeCommandPragma
	OpCodeCommandQuit
	OpCodeCommandVars

	OpCodePatchPlaceholder = 0x000000000000ffff
)

// MultiAssignment is stored as a constant and itself contains indexes into the
// constants table. This lets us multi-assign by emitting an OpCodeAssignment
// that the VM will then dereference into these values and pop items off the
// stack. Like Go, you can use the _ sigil, which maps to a -1 xindex. In that
// case, the VM will pop a value off the stack and drop it.
type MultiAssignment []int

func newMultiAssignment() MultiAssignment {
	return []int{}
}
