// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
)

type VM struct {
	chunk *Chunk
	ip    int // instruction pointer in chunk
	stack []Value

	// gas is how many instructions can be retired before halting (prevents
	// infinite loops)
	gas int

	// the Eval call copied this before each call to Run
	env    map[string]Value
	pragma *Pragma
	cwd    string
	wOut   *Writer // writer for output
	wErr   *Writer // writer for errors

	regGoroutine *Goroutine
	regDumpDst   *GoroutineDump
}

func NewVM(cfg *Config) *VM {
	wOut, wErr := NewWritersFrom(cfg)
	return &VM{
		stack:  make([]Value, 0, defaultStackLimit),
		env:    make(map[string]Value),
		pragma: NewPragma(),
		cwd:    cfg.WorkDir,
		gas:    defaultGas,
		wOut:   wOut,
		wErr:   wErr,
	}
}

func (vm *VM) Reset(chunk *Chunk) {
	vm.ip = -1
	vm.chunk = chunk
	vm.stack = make([]Value, 0, vm.pragma.StackSize)
	vm.gas = defaultGas
}

func (vm *VM) readByte() (Op, error) {
	vm.ip++
	if vm.ip >= len(vm.chunk.ops) {
		return 0, ErrEOF
	}
	op := vm.chunk.ops[vm.ip]
	return op, nil
}

func (vm *VM) Peek() (Value, error) {
	return vm.peekN(1)
}

func (vm *VM) peekN(i int) (Value, error) {
	if len(vm.stack) == 0 {
		return NoValue, ErrEmptyStack
	}
	return vm.stack[len(vm.stack)-i], nil
}

func (vm *VM) Push(val Value) {
	if len(vm.stack) >= vm.pragma.StackSize {
		panic(ErrOutOfStackBounds)
	}
	vm.stack = append(vm.stack, val)
}

func (vm *VM) Pop() (Value, error) {
	if len(vm.stack) == 0 {
		return NoValue, ErrEmptyStack
	}
	val := vm.stack[len(vm.stack)-1]
	vm.stack = vm.stack[:len(vm.stack)-1]
	return val, nil
}

func (vm *VM) Env(key string) (Value, error) {
	val, ok := vm.env[key]
	if !ok {
		return NoValue, fmt.Errorf("%w %q", ErrNoSuchEnv, key)
	}
	return val, nil
}

//go:generate go run ../tools/jumptable chunk.go jumptable.go
type dispatchFn func(*VM, OpCode, uint) error

func (vm *VM) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		vm.gas--
		if vm.gas <= 0 {
			return fmt.Errorf("%w (%d)", ErrOutOfGas, vm.pragma.Gas)
		}

		op, err := vm.readByte()
		if err != nil {
			break // only error this returns is EOF
		}

		instruction, operand := op.decode()
		if int(instruction) > len(jumpTable) {
			return fmt.Errorf("%w %s", ErrNoSuchOpCode, instruction)
		}

		err = jumpTable[instruction](vm, instruction, operand)
		if err != nil {
			return err
		}

	}

	// TODO: would be nice to account for calls to show() so that summaries
	// match the show() value, or just leave the summary off in that case
	val, err := vm.Peek()
	if err == nil {
		switch val.Tag {
		case TagDump:
			val.Data.(*GoroutineDump).Summary(vm.wOut, "")
		case TagDiff:
			val.Data.(*Diff).Left.Summary(vm.wOut, "left")
			val.Data.(*Diff).Right.Summary(vm.wOut, "right")
			val.Data.(*Diff).Common.Summary(vm.wOut, "shared")
		case TagString:
			fmt.Fprintln(vm.wOut, val.Data.(string)) //nolint:errcheck
		case TagBool:
			fmt.Fprintf(vm.wOut, "%v\n", val.Data.(bool)) //nolint:errcheck
		case TagNumber:
			fmt.Fprintf(vm.wOut, "%d\n", val.Data.(int)) //nolint:errcheck
		}
	}

	return nil
}

var (
	ErrOutOfGas                  = errors.New("processed more than maximum number of instructions")
	ErrEmptyStack                = errors.New("tried to pop off empty stack")
	ErrUnexpectedStackState      = errors.New("unexpected stack state")
	ErrUnexpectedRegisterState   = errors.New("unexpected register state")
	ErrOutOfStackBounds          = errors.New("jump outside of stack bounds")
	ErrInvalidType               = errors.New("invalid type for operation")
	ErrInvalidOpArg              = errors.New("invalid argument for operation")
	ErrArgumentUnset             = errors.New("expected argument is unset")
	ErrNoSuchOpCode              = errors.New("no such op code")
	ErrExpectedConstantValueByte = errors.New("expected value after constant load byte")
	ErrExpectedJumpAddress       = errors.New("expected address for jump")
	ErrExpectedCommand           = errors.New("expected valid command after command byte")
	ErrNoSuchEnv                 = errors.New("no identifer with name")
	ErrNoSuchPragma              = errors.New("no pragma with name")
	ErrExpectedDiffAssign        = errors.New("assigning a diff must have 3 identifiers or \"_\"")

	ErrCommandQuit = errors.New("user quit")
	ErrCommandOk   = errors.New("command ok")
)

func opNoop(_ *VM, _ OpCode, _ uint) error { return nil }

func opComparison(vm *VM, instruction OpCode, _ uint) error {
	right, err := vm.Pop()
	if err != nil {
		return err
	}
	left, err := vm.Pop()
	if err != nil {
		return err
	}
	if left.Tag != right.Tag {
		return fmt.Errorf(
			"expected matching types, got %+v and %+v", left.Tag, right.Tag)
	}

	var val bool
	switch left.Data.(type) {
	case int:
		val = compare(left.Data.(int), right.Data.(int), instruction)
	case string:
		val = compare(left.Data.(string), right.Data.(string), instruction)
	}
	vm.Push(Value{Tag: TagBool, Data: val})
	return nil
}

type ordered interface {
	~int | ~string
}

func compare[T ordered](left, right T, instruction OpCode) bool {
	switch instruction {
	case OpCodeLess:
		return left < right
	case OpCodeLessEqual:
		return left <= right
	case OpCodeGreater:
		return left > right
	case OpCodeGreaterEqual:
		return left >= right
	case OpCodeEqual:
		return left == right
	case OpCodeNotEqual:
		return left != right
	}
	return false
}

func opContains(vm *VM, instruction OpCode, _ uint) error {
	right, err := vm.Pop()
	if err != nil {
		return err
	}
	left, err := vm.Pop()
	if err != nil {
		return err
	}
	if left.Tag != right.Tag {
		return fmt.Errorf(
			"expected matching types, got %+v and %+v", left.Data, right.Data)
	}

	var val bool
	switch left.Data.(type) {
	case string:
		val = strings.Contains(left.Data.(string), right.Data.(string))
	default:
		// TODO: actually needs to be a goroutine on one side and a string on
		// the other, I think?
		return fmt.Errorf("%w: expected string for contains", ErrInvalidType)
	}
	vm.Push(Value{Tag: TagBool, Data: val})
	return nil
}

func (vm *VM) popDump() (*GoroutineDump, error) {
	val, err := vm.Pop()
	if err != nil {
		return nil, err
	}
	if b, ok := val.Data.(*GoroutineDump); !ok {
		return nil, fmt.Errorf("%w: expected a goroutine dump", ErrInvalidType)
	} else {
		return b, nil
	}
}

func (vm *VM) peekDump() (*GoroutineDump, error) {
	val, err := vm.Peek()
	if err != nil {
		return nil, err
	}
	if b, ok := val.Data.(*GoroutineDump); !ok {
		return nil, fmt.Errorf("%w: expected a goroutine dump", ErrInvalidType)
	} else {
		return b, nil
	}
}

func opLoadNumber(vm *VM, _ OpCode, index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	num := con.(int)
	vm.Push(Value{Tag: TagNumber, Data: num})
	return nil
}

func opLoadString(vm *VM, _ OpCode, index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	str := con.(string)
	vm.Push(Value{Tag: TagString, Data: str})
	return nil
}

func (vm *VM) fetchConstant(index uint) (any, error) {
	if index > uint(len(vm.chunk.constants)) {
		return nil, ErrExpectedConstantValueByte
	}
	con := vm.chunk.constants[index]
	return con, nil
}

func (vm *VM) fetchString(index uint) (string, error) {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return "", err
	}
	val, ok := con.(string)
	if !ok {
		return "", ErrInvalidType
	}
	return val, nil
}

func opLoadGoroutineDump(vm *VM, _ OpCode, index uint) error {
	name, err := vm.fetchString(index)
	if err != nil {
		return fmt.Errorf("%w: expected name of a variable or constant", err)
	}

	val, ok := vm.env[name]
	if !ok {
		return fmt.Errorf("%w %q", ErrNoSuchEnv, name)
	}
	if val.Tag != TagDump {
		return fmt.Errorf("%w: expected dump, got %s", ErrInvalidType, val.Tag)
	}
	vm.Push(val)
	return nil
}

func (vm *VM) debug() {
	fmt.Printf("chunk (ip=%d)\n", vm.ip)
	fmt.Println(vm.chunk.disassemble(vm.ip))

	fmt.Printf("env\n")
	for k, v := range vm.env {
		fmt.Printf("  %s => %v\n", k, v)
	}

	fmt.Printf("stack\n")
	for i := len(vm.stack) - 1; i >= 0; i-- {
		fmt.Printf("  [%02d] %v\n", i, vm.stack[i])
	}

	fmt.Printf("registers\n")
	fmt.Printf("  goroutine: %s\n", vm.regGoroutine.Debug())
	fmt.Printf("  dstDump: %d\n", vm.regDumpDst.Len())
}

func (vm *VM) newInvalidTypeErr(expected, got Tag) error {
	vm.debug()
	return fmt.Errorf("%w: expected %s got %s", ErrInvalidType, expected, got)
}

func opFuncUnion(vm *VM, _ OpCode, _ uint) error {

	left, err := vm.popDump()
	if err != nil {
		return err
	}
	right, err := vm.popDump()
	if err != nil {
		return err
	}

	// TODO: obviously we need to make sure these get de-duplicated in the Add
	// method
	g := NewGoroutineDump()
	for _, lg := range left.goroutines {
		g.Add(lg)
	}
	for _, rg := range right.goroutines {
		g.Add(rg)
	}

	vm.pushDump(g)
	return nil
}

func opFuncIntersect(vm *VM, _ OpCode, _ uint) error {
	left, err := vm.popDump()
	if err != nil {
		return err
	}
	right, err := vm.popDump()
	if err != nil {
		return err
	}

	g := NewGoroutineDump()
	for _, lg := range left.goroutines {
		if right.Has(lg) {
			g.Add(lg)
		}
	}

	vm.pushDump(g)
	return nil
}

func opFuncDiff(vm *VM, _ OpCode, _ uint) error {
	inRight, err := vm.popDump()
	if err != nil {
		return err
	}

	inLeft, err := vm.popDump()
	if err != nil {
		return err
	}

	left := NewGoroutineDump()
	right := NewGoroutineDump()
	common := NewGoroutineDump()
	for _, lg := range inLeft.goroutines {
		if inRight.Has(lg) {
			common.Add(lg)
		} else {
			left.Add(lg)
		}
	}
	for _, rg := range inRight.goroutines {
		if inLeft.Has(rg) {
			common.Add(rg)
		} else {
			right.Add(rg)
		}
	}

	// we push a Diff and not a stack of three values because we want to be able
	// to return a single item off the stack when the VM exits
	vm.Push(Value{
		Tag: TagDiff,
		Data: &Diff{
			Left:   left,
			Right:  right,
			Common: common,
		},
	})
	return nil
}

func opLoadFieldAccessor(vm *VM, _ OpCode, index uint) error {
	name, err := vm.fetchString(index)
	if err != nil {
		return fmt.Errorf("%w: expected name field", err)
	}
	g := vm.regGoroutine
	if g == nil {
		return fmt.Errorf("%w: no goroutine", ErrUnexpectedRegisterState)
	}

	switch name {
	case "id", ".id":
		vm.Push(Value{Tag: TagNumber, Data: g.ID})
	case "header", ".header":
		vm.Push(Value{Tag: TagString, Data: g.Header})
	case "trace", ".trace":
		vm.Push(Value{Tag: TagString, Data: g.Trace})
	case "lines", ".lines":
		vm.Push(Value{Tag: TagNumber, Data: g.LineCount})
	case "duration", ".duration":
		vm.Push(Value{Tag: TagNumber, Data: g.Duration})
	case "state", ".state":
		vm.Push(Value{Tag: TagString, Data: g.State})
	}

	return nil
}

func opAddGoroutine(vm *VM, _ OpCode, _ uint) error {
	vm.regDumpDst.Add(vm.regGoroutine)
	return nil
}

// starting stack:
// - dump
// ending stack:
// - dump
// - goroutine
// OR (when complete)
// - dump
func opNextGoroutine(vm *VM, _ OpCode, addr uint) error {
	var dump *GoroutineDump

	val, err := vm.Peek()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnexpectedStackState, err)
	}
	switch val.Tag {
	case TagDump:
		// first call
		dump = val.Data.(*GoroutineDump)
		dump.StartIter()

	case TagGoroutine:
		// subsequent calls: need to clean up the previous goroutine; this is
		// how we track that we're in the middle of a loop
		_, err = vm.Pop()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnexpectedStackState, err)
		}
		val, err = vm.Peek()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnexpectedStackState, err)
		}
		if val.Tag != TagDump {
			return vm.newInvalidTypeErr(TagDump, val.Tag)
		}
		dump = val.Data.(*GoroutineDump)

	default:
		return fmt.Errorf("%w: expected either dump or previous goroutine on top of stack when iterating", ErrUnexpectedStackState)
	}

	if dump.Len() == 0 {
		vm.ip = int(addr) - 1
		vm.regGoroutine = nil
		_, _ = vm.Pop() // we're done: pop the dump off the stack
		return nil
	}
	g := dump.Next()
	if g == nil {
		vm.ip = int(addr) - 1
		vm.regGoroutine = nil
		_, _ = vm.Pop() // we're done: pop the dump off the stack
		return nil
	}

	vm.regGoroutine = g
	vm.Push(Value{Tag: TagGoroutine, Data: g})
	return nil
}

func opTempDump(vm *VM, _ OpCode, _ uint) error {
	vm.regDumpDst = NewGoroutineDump()
	return nil
}

func opPushDump(vm *VM, _ OpCode, _ uint) error {
	vm.pushDump(vm.regDumpDst)
	return nil
}

func (vm *VM) pushDump(dump *GoroutineDump) {
	dump.StartIter() // reset before we push it back onto the stack
	vm.Push(Value{
		Tag:  TagDump,
		Data: dump,
	})
}

func opPushBool(vm *VM, _ OpCode, operand uint) error {
	vm.Push(Value{Tag: TagBool, Data: operand == 1})
	return nil
}

// opAssignment writes the object at the top of the stack to the
// environment, but leaves it on the stack
func opAssignment(vm *VM, _ OpCode, index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	switch target := con.(type) {
	case string:
		val, err := vm.Peek()
		if err != nil {
			return err
		}
		vm.env[target] = val
	case MultiAssignment:
		return vm.handleMultiAssignment(target)
	default:
		return fmt.Errorf(
			"%w assignment: expected identifier or multi-assign but got %v",
			ErrInvalidType, con)
	}

	return nil
}

func (vm *VM) assign(index int, val Value) error {
	if index >= 0 {
		name, err := vm.fetchString(uint(index))
		if err != nil {
			return fmt.Errorf("%w assignment: expected variable name", err)
		}
		if name == "_" {
			return nil
		}
		vm.env[name] = val
	}
	return nil
}

func (vm *VM) handleMultiAssignment(m MultiAssignment) error {

	top, err := vm.Peek()
	if err != nil {
		return err
	}
	switch top.Tag {
	case TagDiff:
		if len(m) != 3 {
			return ErrExpectedDiffAssign
		}
		val, _ := vm.Peek()
		diff, ok := val.Data.(*Diff)
		if !ok {
			t := reflect.TypeOf(val.Data)
			return fmt.Errorf("%w: diff value was a %v", ErrInvalidType, t)
		}
		err := vm.assign(m[0], Value{TagDump, diff.Left})
		if err != nil {
			return err
		}
		err = vm.assign(m[1], Value{TagDump, diff.Right})
		if err != nil {
			return err
		}
		err = vm.assign(m[2], Value{TagDump, diff.Common})
		if err != nil {
			return err
		}
	case TagDump:
		for i, idx := range m {
			val, err := vm.peekN(i)
			if err != nil {
				return err
			}
			err = vm.assign(idx, val)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func opConditionalJump(vm *VM, instruction OpCode, addr uint) error {
	val, err := vm.popBool()
	if err != nil {
		return fmt.Errorf("%w conditional jump", err)
	}
	if val == (instruction == OpCodeJumpIfTrue) {
		vm.ip = int(addr) - 1
	}
	return nil
}

func opJumpTo(vm *VM, _ OpCode, addr uint) error {
	vm.ip = int(addr) - 1
	return nil
}

func opCommandGetWorkingDir(vm *VM, _ OpCode, _ uint) error {
	fmt.Fprint(vm.wOut, vm.cwd+"\n") //nolint:errcheck
	return nil
}

func opCommandChangeDir(vm *VM, _ OpCode, index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	path, ok := con.(string)
	if !ok {
		return fmt.Errorf("cd requires a string argument")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	err = os.Chdir(path)
	if err != nil {
		return fmt.Errorf("could not change working directory: %w", err)
	}
	vm.cwd = path
	return ErrCommandOk
}

func opCommandEmpty(vm *VM, _ OpCode, _ uint) error {
	vm.env = map[string]Value{}
	return ErrCommandOk
}

func opCommandQuit(_ *VM, _ OpCode, _ uint) error {
	return ErrCommandQuit
}

func opCommandHelp(vm *VM, _ OpCode, index uint) error {
	// TODO: what about when we have no topic?
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	topic, ok := con.(string)
	if !ok {
		return fmt.Errorf("help topics must be strings")
	}

	// TODO: lookup topic
	fmt.Fprintf(vm.wOut, "help for topic: %s", topic) //nolint:errcheck
	return ErrCommandOk
}

func opCommandGetPragma(vm *VM, _ OpCode, _ uint) error {
	setting, err := vm.popString()
	if err != nil {
		return err
	}

	switch setting {
	case "empty.confirm":
		fmt.Fprintf(vm.wOut, "%v\n", vm.pragma.EmptyConfirm)
	case "exit.confirm":
		fmt.Fprintf(vm.wOut, "%v\n", vm.pragma.ExitConfirm)
	case "show.color":
		fmt.Fprintf(vm.wOut, "%v\n", vm.pragma.ShowColor)
	case "show.count":
		fmt.Fprintf(vm.wOut, "%v\n", vm.pragma.ShowCount)
	case "ls.format":
		fmt.Fprintf(vm.wOut, "%s\n", vm.pragma.ListFormat)
	case "show.dedup":
		fmt.Fprintf(vm.wOut, "%s\n", vm.pragma.ShowDedup)
	case "vars.display":
		fmt.Fprintf(vm.wOut, "%s\n", vm.pragma.VarsDisplay)
	default:
		return fmt.Errorf("%w pragma.%s", ErrNoSuchPragma, setting)
	}
	if err != nil {
		return err
	}

	return ErrCommandOk

}

func opCommandSetPragma(vm *VM, _ OpCode, _ uint) error {
	setting, err := vm.popString()
	if err != nil {
		return err
	}

	switch setting {
	case "empty.confirm":
		err = popAndSet(vm, &vm.pragma.EmptyConfirm)
	case "exit.confirm":
		err = popAndSet(vm, &vm.pragma.ExitConfirm)
	case "show.color":
		err = popAndSet(vm, &vm.pragma.ShowColor)
	case "show.count":
		err = popAndSet(vm, &vm.pragma.ShowCount)
	case "ls.format":
		err = popAndSet(vm, &vm.pragma.ListFormat)
	case "show.dedup":
		err = popAndSet(vm, &vm.pragma.ShowDedup)
	case "vars.display":
		err = popAndSet(vm, &vm.pragma.VarsDisplay)
	default:
		return fmt.Errorf("%w pragma.%s", ErrNoSuchPragma, setting)
	}
	if err != nil {
		return err
	}

	return ErrCommandOk
}

func popAndSet[T any](vm *VM, setting *T) error {
	raw, err := vm.Pop()
	if err != nil {
		return err
	}
	val, ok := raw.Data.(T)
	if !ok {
		return ErrInvalidType
	}
	*setting = val
	return nil
}

func opCommandVars(vm *VM, _ OpCode, _ uint) error {
	vars := slices.Collect(maps.Keys(vm.env))
	sort.Strings(vars)

	mode := vm.pragma.VarsDisplay
	switch mode {
	case PragmaDisplayCount:
		for _, name := range vars {
			v := vm.env[name]
			if v.Tag == TagDump {
				if dump, ok := v.Data.(*GoroutineDump); ok {
					fmt.Fprintf(vm.wOut, "%s: %d\n", name, dump.Len()) //nolint:errcheck
				}
			}
		}
	case PragmaDisplaySummary:
		for _, name := range vars {
			v := vm.env[name]
			if v.Tag == TagDump {
				if dump, ok := v.Data.(*GoroutineDump); ok {
					dump.Summary(vm.wOut, name)
				}
			}
		}
	case PragmaDisplayNone:
		out := strings.Join(vars, "\t")
		fmt.Fprint(vm.wOut, out) //nolint:errcheck
	default:
		return fmt.Errorf(
			"%w %q: expected \"count\", \"summary\", or \"none\"",
			ErrInvalidOpArg, mode)
	}

	return ErrCommandOk
}

func (vm *VM) popString() (string, error) {
	val, err := vm.Pop()
	if err != nil {
		return "", err
	}
	if val.Tag != TagString {
		return "", ErrInvalidType
	}
	arg, ok := val.Data.(string)
	if !ok {
		return "", fmt.Errorf(
			"%w: expected string", ErrInvalidType)
	}
	return arg, nil
}

func (vm *VM) popBool() (bool, error) {
	val, err := vm.Pop()
	if err != nil {
		return false, err
	}
	if val.Tag != TagBool {
		return false, ErrInvalidType
	}
	arg, ok := val.Data.(bool)
	if !ok {
		return false, fmt.Errorf(
			"%w: expected bool", ErrInvalidType)
	}
	return arg, nil
}

func (vm *VM) popNumber() (int, error) {
	val, err := vm.Pop()
	if err != nil {
		return -1, err
	}
	if val.Tag != TagNumber {
		return -1, ErrInvalidType
	}
	arg, ok := val.Data.(int)
	if !ok {
		return -1, fmt.Errorf(
			"%w: expected number", ErrInvalidType)
	}
	return arg, nil
}

func opFuncLoad(vm *VM, _ OpCode, _ uint) error {
	path, err := vm.popString()
	if err != nil {
		return err
	}
	dump, err := load(path)
	if err != nil {
		return err
	}
	vm.pushDump(dump)
	return nil
}

func opFuncSave(vm *VM, _ OpCode, _ uint) error {
	path, err := vm.popString()
	if err != nil {
		return err
	}
	dump, err := vm.popDump()
	if err != nil {
		return err
	}

	return dump.Save(path)
}

func opFuncShowDump(vm *VM, _ OpCode, _ uint) error {
	limit, err := vm.popNumber()
	if err != nil {
		return err
	}
	if limit < 1 {
		limit = vm.pragma.ShowCount
	}
	offset, err := vm.popNumber()
	if err != nil {
		return err
	}
	dump, err := vm.peekDump()
	if err != nil {
		return err
	}

	dump.Show(vm.wOut, limit, offset)
	return nil
}
