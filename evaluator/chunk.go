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
		case OpCodeAssignment, OpCodeLoadAttr, OpCodeLoadFieldAccessor,
			OpCodeLoadNumber, OpCodeLoadString, OpCodeLoadGoroutineDump:
			if idx <= uint(len(c.constants)) {
				val := c.constants[idx]
				comment = fmt.Sprintf("\t// %v", val)
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
	OpCodeAnd OpCode = iota
	OpCodeOr
	OpCodeLoadNumber
	OpCodeLoadString
	OpCodeLoadIdentifier
	OpCodeLoadGoroutineDump
	OpCodeLoadEnv
	OpCodeStoreEnv
	OpCodeLoadFieldAccessor
	OpCodeEqual
	OpCodeGreater
	OpCodeLess
	OpCodePipe
	OpCodeFunction
	OpCodeNextGoroutine
	OpCodeInitDump
	OpCodeTempDump
	OpCodePushDump
	OpCodeJumpIfTrue
	OpCodeJumpIfFalse
	OpCodeJumpTo
	OpCodeLoadAttr
	OpCodeAddGoroutine
	OpCodeRemoveGoroutine
	OpCodeStartIter
	OpCodeAssignment
	OpCodeReturn
	OpCodeContains

	OpCodePatchPlaceholder = 0x000000000000ffff
)
