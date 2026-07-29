// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"fmt"
	"strconv"
	"strings"
	"text/scanner"
)

type Code struct {
	chunks    []*Chunk
	constants []any
}

func NewCode() *Code {
	return &Code{
		chunks:    []*Chunk{NewChunk()},
		constants: []any{},
	}
}

func (co *Code) disassemble(index, ip int) string {
	var b strings.Builder
	b.WriteString(".text\n")

	for cindex, c := range co.chunks {
		b.WriteString("(chunk ")
		b.WriteString(strconv.Itoa(cindex))
		b.WriteString(")\n")

		for instIndex, op := range c.ops {
			ipPrefix := "  "
			if instIndex == ip && cindex == index {
				ipPrefix = "* "
			}
			comment := "\t"
			code, idx := op.decode()
			switch code {
			case OpCodeAssignment, OpCodeLoadFieldAccessor,
				OpCodeLoadNumber, OpCodeLoadString, OpCodeLoadGoroutineDump:
				if idx <= uint(len(co.constants)) {
					if len(co.constants) > int(idx) {
						val := co.constants[idx]
						comment = fmt.Sprintf("\t// %v", val)
					}
				}
			}
			fmt.Fprintf(&b, "%s[%02d] %s%s\n", ipPrefix, instIndex, op, comment)
		}
	}

	b.WriteString(".data\n")
	if len(co.constants) == 0 {
		b.WriteString("  <none>\n")
	}
	for i, con := range co.constants {
		fmt.Fprintf(&b, "  [%02d] %v\n", i, con)
	}

	return b.String()
}

type Chunk struct {
	ops  []Op
	locs []scanner.Position
}

func NewChunk() *Chunk {
	return &Chunk{
		ops:  []Op{},
		locs: []scanner.Position{},
	}
}

func (c *Chunk) locForInstruction(ip int) scanner.Position {
	if ip >= len(c.locs) {
		return scanner.Position{}
	}
	return c.locs[ip]
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

	// loads
	OpCodeLoadGoroutineDump
	OpCodeLoadFieldAccessor
	OpCodeLoadNumber
	OpCodeLoadString

	// stores
	OpCodeAssignment
	OpCodePushBool
	OpCodePushDump
	OpCodeDup

	// filter iteration
	OpCodeAddGoroutine
	OpCodeNextGoroutine
	OpCodeTempDump
	OpCodeResetDump

	// comparisons
	OpCodeContains
	OpCodeIn // opContains
	OpCodeRegexMatches
	OpCodeRegexNotMatches // opRegexMatches
	OpCodeEqual           // opComparison
	OpCodeGreater         // opComparison
	OpCodeGreaterEqual    // opComparison
	OpCodeLess            // opComparison
	OpCodeLessEqual       // opComparison
	OpCodeNotEqual        // opComparison

	// control flow
	OpCodeJumpIfFalse // opConditionalJump
	OpCodeJumpIfTrue  // opConditionalJump
	OpCodeJumpTo
	OpCodeCall
	OpCodeReturn

	// functions
	OpCodeFuncDiff
	OpCodeFuncIntersect
	OpCodeFuncLoad
	OpCodeFuncSave
	OpCodeFuncShowDump
	OpCodeFuncToJSON
	OpCodeFuncToDot
	OpCodeFuncUnion
	OpCodeFuncGraph

	// commands
	OpCodeCommandChangeDir
	OpCodeCommandEmpty
	OpCodeCommandGetWorkingDir
	OpCodeCommandHelp
	OpCodeCommandListDir
	OpCodeCommandQuit
	OpCodeCommandVars
	OpCodeCommandSetPragma
	OpCodeCommandGetPragma

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
