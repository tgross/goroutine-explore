// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type VM struct {
	code   *Code
	stack  []Value
	frames []*frame
	frame  *frame

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
	regexCache   map[string]*regexp.Regexp

	// didShow suppresses outputting a dump summary
	didShow bool
}

func NewVM(cfg *Config) *VM {
	wOut, wErr := NewWritersFrom(cfg)
	return &VM{
		stack:      make([]Value, 0, defaultStackLimit),
		env:        make(map[string]Value),
		pragma:     NewPragma(),
		cwd:        cfg.WorkDir,
		gas:        defaultGas,
		wOut:       wOut,
		wErr:       wErr,
		regexCache: make(map[string]*regexp.Regexp),
	}
}

func (vm *VM) Reset(code *Code) {
	vm.code = code
	vm.frame = &frame{
		ip:         -1,
		returnAddr: 0,
		chunk:      code.chunks[0],
	}
	vm.frames = []*frame{vm.frame}
	vm.stack = make([]Value, 0, vm.pragma.StackSize)
	vm.gas = defaultGas
	vm.regexCache = make(map[string]*regexp.Regexp)
	vm.didShow = false
}

type frame struct {
	ip         int    // instruction pointer in chunk
	returnAddr int    // instruction pointer in previous chunk
	chunk      *Chunk // code for this frame
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

		op, err := vm.nextOp()
		if err != nil {
			break // only error this returns is EOF
		}

		instruction, operand := op.decode()
		if int(instruction) > len(jumpTable) {
			return fmt.Errorf("%w %s", ErrNoSuchOpCode, instruction)
		}

		err = jumpTable[instruction](vm, instruction, operand)
		if err != nil {
			if vm.pragma.DebugDisassemble != PragmaDebugDisassembleNone {
				vm.debug()
			}
			pos := vm.frame.chunk.locForInstruction(vm.frame.ip)
			return ErrorWithPosition{pos, err}
		}

	}

	if vm.pragma.DebugDisassemble == PragmaDebugDisassembleOnReturn {
		vm.debug()
	}

	if vm.didShow {
		return nil
	}
	val, err := vm.peek()
	if err == nil {
		switch val.Tag {
		case TagDump:
			dump, ok := val.Data.(*GoroutineDump)
			if !ok {
				return vm.newWrongTagErr(val)
			}
			dump.Summary(vm.wOut, "")
		case TagDiff:
			diff, ok := val.Data.(*Diff)
			if !ok {
				return vm.newWrongTagErr(val)
			}
			diff.Left.Summary(vm.wOut, "left")
			diff.Right.Summary(vm.wOut, "right")
			diff.Common.Summary(vm.wOut, "shared")
		case TagString:
			s, ok := val.Data.(string)
			if !ok {
				return vm.newWrongTagErr(val)
			}
			fmt.Fprintln(vm.wOut, s)
		case TagBool:
			b, ok := val.Data.(bool)
			if !ok {
				return vm.newWrongTagErr(val)
			}
			fmt.Fprintf(vm.wOut, "%v\n", b)
		case TagNumber:
			num, ok := val.Data.(int)
			if !ok {
				return vm.newWrongTagErr(val)
			}
			fmt.Fprintf(vm.wOut, "%d\n", num)
		}
	}

	return nil
}

func (vm *VM) nextOp() (Op, error) {
	vm.frame.ip++
	if vm.frame.ip >= len(vm.frame.chunk.ops) {
		return 0, ErrEOF
	}
	op := vm.frame.chunk.ops[vm.frame.ip]
	return op, nil
}

func (vm *VM) peek() (Value, error) {
	return vm.peekN(1)
}

func (vm *VM) peekN(i int) (Value, error) {
	if len(vm.stack) == 0 {
		return NoValue, ErrEmptyStack
	}
	return vm.stack[len(vm.stack)-i], nil
}

func (vm *VM) push(val Value) {
	if len(vm.stack) >= vm.pragma.StackSize {
		panic(ErrOutOfStackBounds)
	}
	vm.stack = append(vm.stack, val)
}

func (vm *VM) pop() (Value, error) {
	if len(vm.stack) == 0 {
		return NoValue, ErrEmptyStack
	}
	val := vm.stack[len(vm.stack)-1]
	vm.stack = vm.stack[:len(vm.stack)-1]
	return val, nil
}

func (vm *VM) popDump() (*GoroutineDump, error) {
	return popExpect[*GoroutineDump](vm, TagDump)
}

func (vm *VM) popNumber() (int, error) {
	return popExpect[int](vm, TagNumber)
}

func (vm *VM) popString() (string, error) {
	return popExpect[string](vm, TagString)
}

func (vm *VM) popBool() (bool, error) {
	return popExpect[bool](vm, TagBool)
}

func popExpect[T any](vm *VM, tag Tag) (T, error) {
	var zero T
	val, err := vm.pop()
	if err != nil {
		return zero, err
	}
	if val.Tag != tag {
		return zero, fmt.Errorf("%w: expected %s", ErrInvalidType, tag)
	}
	data, ok := val.Data.(T)
	if !ok {
		// arguably this should panic because this state isn't really
		// recoverable but let's wait until we've stabilized this first
		return zero, vm.newWrongTagErr(val)
	}
	return data, nil
}

func (vm *VM) debug() {
	fmt.Println(vm.code.disassemble(len(vm.frames)-1, vm.frame.ip))

	fmt.Printf("env\n")
	for k, v := range vm.env {
		fmt.Printf("  %s => %v\n", k, v)
	}

	fmt.Printf("stack\n")
	for i, line := range slices.Backward(vm.stack) {
		fmt.Printf("  [%02d] %v\n", i, line)
	}

	fmt.Printf("registers\n")
	fmt.Printf("  goroutine: %s\n", vm.regGoroutine.Debug())
	fmt.Printf("  dstDump: %d\n", vm.regDumpDst.Len())
}

var (
	ErrOutOfGas                  = errors.New("processed more than maximum number of instructions")
	ErrEmptyStack                = errors.New("tried to pop off empty stack")
	ErrUnexpectedStackState      = errors.New("unexpected stack state")
	ErrNotIterating              = errors.New("not iterating a goroutine dump")
	ErrOutOfStackBounds          = errors.New("jump outside of stack bounds")
	ErrInvalidType               = errors.New("invalid type for operation")
	ErrWrongTag                  = errors.New("value had wrong tag for type (please report as a bug)")
	ErrInvalidOpArg              = errors.New("invalid argument for operation")
	ErrArgumentUnset             = errors.New("expected argument is unset")
	ErrNoSuchOpCode              = errors.New("no such op code")
	ErrExpectedConstantValueByte = errors.New("expected value after constant load byte")
	ErrExpectedJumpAddress       = errors.New("expected address for jump")
	ErrExpectedCommand           = errors.New("expected valid command after command byte")
	ErrNoSuchEnv                 = errors.New("no identifer with name")
	ErrNoSuchPragma              = errors.New("no pragma with name")
	ErrExpectedDiffAssign        = errors.New("assigning a diff must have 3 identifiers or \"_\"")

	ErrCommandQuit    = errors.New("user quit")
	ErrCommandOk      = errors.New("command ok")
	ErrCommandConfirm = errors.New("are you sure?")
)

// newWrongTagErr is for catching bugs where the compiler somehow associated a
// Value with the wrong type for its tag. These are always bugs, so we print the
// debug output to make it obvious.
func (vm *VM) newWrongTagErr(val Value) error {
	vm.debug()
	gotType := reflect.TypeOf(reflect.ValueOf(val.Data).Interface())
	return fmt.Errorf("%w: expected %s got %+v (%v)",
		ErrWrongTag, val.Tag, val.Data, gotType)
}

func opNoop(_ *VM, _ OpCode, _ uint) error { return nil }

func opDup(vm *VM, _ OpCode, _ uint) error {
	val, err := vm.peek()
	if err != nil {
		return err
	}
	dup := val
	vm.push(dup)
	return nil
}

func opComparison(vm *VM, instruction OpCode, _ uint) error {
	right, err := vm.pop()
	if err != nil {
		return err
	}
	left, err := vm.pop()
	if err != nil {
		return err
	}
	if left.Tag != right.Tag {
		return fmt.Errorf(
			"expected matching types, got %+v and %+v", left.Tag, right.Tag)
	}

	var val bool
	switch left := left.Data.(type) {
	case int:
		if r, ok := right.Data.(int); ok {
			val = compare(left, r, instruction)
		} else {
			return vm.newWrongTagErr(right)
		}
	case string:
		if r, ok := right.Data.(string); ok {
			val = compare(left, r, instruction)
		} else {
			return vm.newWrongTagErr(right)
		}
	}
	vm.push(Value{Tag: TagBool, Data: val})
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
	right, err := vm.pop()
	if err != nil {
		return err
	}
	left, err := vm.pop()
	if err != nil {
		return err
	}
	var containerVal, containedVal Value
	if instruction == OpCodeContains {
		// ex. labels contains "foo"
		containerVal = left
		containedVal = right
	} else {
		// ex. "foo" in labels
		containerVal = right
		containedVal = left
	}

	var val bool
	switch containerVal.Tag {
	case TagString:
		container, ok := containerVal.Data.(string)
		if !ok {
			return vm.newWrongTagErr(containerVal)
		}
		contained, ok := containedVal.Data.(string)
		if !ok {
			return fmt.Errorf(
				"%w: expected to check for string", ErrInvalidType)
		}
		val = strings.Contains(container, contained)
	case TagMap:
		container, ok := containerVal.Data.(map[string]string)
		if !ok {
			return vm.newWrongTagErr(containerVal)
		}
		contained, ok := containedVal.Data.(string)
		if !ok {
			return fmt.Errorf(
				"%w: expected to check for string", ErrInvalidType)
		}
		_, val = container[contained]
	default:
		return fmt.Errorf(
			"%w: expected container to be string or map", ErrInvalidType)
	}

	vm.push(Value{Tag: TagBool, Data: val})
	return nil
}

func opRegexMatches(vm *VM, instruction OpCode, _ uint) error {
	right, err := vm.popString()
	if err != nil {
		return err
	}
	left, err := vm.popString()
	if err != nil {
		return err
	}
	re, ok := vm.regexCache[right]
	if !ok || re == nil {
		re, err = regexp.Compile(right)
		if err != nil {
			return err
		}
		vm.regexCache[right] = re
	}
	val := re.MatchString(left)
	if instruction == OpCodeRegexNotMatches {
		val = !val
	}
	vm.push(Value{Tag: TagBool, Data: val})
	return nil
}

func (vm *VM) peekDump() (*GoroutineDump, error) {
	val, err := vm.peek()
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
	num, ok := con.(int)
	if !ok {
		return fmt.Errorf("%w: expected number", ErrInvalidType)
	}
	vm.push(Value{Tag: TagNumber, Data: num})
	return nil
}

func opLoadString(vm *VM, _ OpCode, index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	str, ok := con.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidType)
	}
	vm.push(Value{Tag: TagString, Data: str})
	return nil
}

func (vm *VM) fetchConstant(index uint) (any, error) {
	if index > uint(len(vm.code.constants)) || len(vm.code.constants) == 0 {
		return nil, ErrExpectedConstantValueByte
	}
	con := vm.code.constants[index]
	return con, nil
}

func (vm *VM) fetchString(index uint) (string, error) {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return "", err
	}
	val, ok := con.(string)
	if !ok {
		return "", fmt.Errorf("%w: expected string", ErrInvalidType)
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
	vm.push(val)
	return nil
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

	g := NewGoroutineDump()
	for _, lg := range left.goroutines {
		g.Add(lg)
	}
	for _, rg := range right.goroutines {
		g.Add(rg)
	}

	g.Sort()
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
	vm.push(Value{
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
		return fmt.Errorf("%w when accessing field %s", ErrNotIterating, name)
	}

	switch name {
	case ".id":
		vm.push(Value{Tag: TagNumber, Data: g.ID})
	case ".header":
		vm.push(Value{Tag: TagString, Data: g.header})
	case ".trace":
		vm.push(Value{Tag: TagString, Data: g.Trace})
	case ".lines":
		vm.push(Value{Tag: TagNumber, Data: g.LineCount})
	case ".duration":
		vm.push(Value{Tag: TagNumber, Data: g.Duration})
	case ".state":
		vm.push(Value{Tag: TagString, Data: g.State})
	case ".createdBy":
		vm.push(Value{Tag: TagNumber, Data: g.CreatedBy})
	case ".dups":
		vm.push(Value{Tag: TagNumber, Data: len(g.duplicates)})
	case ".labels":
		vm.push(Value{Tag: TagMap, Data: g.Labels})
		return nil // to avoid hitting next case
	}
	if strings.HasPrefix(name, ".labels") {
		label, _ := strings.CutPrefix(name, ".labels.")
		if val, ok := g.Labels[label]; ok {
			vm.push(Value{Tag: TagString, Data: val})
		} else {
			vm.push(Value{Tag: TagString, Data: "<nil>"})
		}
	}

	return nil
}

func opResetDump(vm *VM, _ OpCode, _ uint) error {
	vm.regDumpDst.StartIter()
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

	val, err := vm.peek()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnexpectedStackState, err)
	}
	switch val.Tag {
	case TagDump:
		// first call
		var ok bool
		dump, ok = val.Data.(*GoroutineDump)
		if !ok {
			return vm.newWrongTagErr(val)
		}
		dump.StartIter()

	case TagGoroutine:
		// subsequent calls: need to clean up the previous goroutine; this is
		// how we track that we're in the middle of a loop
		_, err = vm.pop()
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnexpectedStackState, err)
		}
		val, err = vm.peek()
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnexpectedStackState, err)
		}
		if val.Tag != TagDump {
			return fmt.Errorf("%w: expected %s got %s",
				ErrInvalidType, TagDump, val.Tag)
		}
		var ok bool
		dump, ok = val.Data.(*GoroutineDump)
		if !ok {
			return vm.newWrongTagErr(val)
		}

	default:
		return fmt.Errorf("%w: expected either dump or previous goroutine on top of stack when iterating", ErrUnexpectedStackState)
	}

	if dump.Len() == 0 {
		vm.frame.ip = int(addr) - 1
		vm.regGoroutine = nil
		_, _ = vm.pop() // we're done: pop the dump off the stack
		return nil
	}
	g := dump.Next()
	if g == nil {
		vm.frame.ip = int(addr) - 1
		vm.regGoroutine = nil
		_, _ = vm.pop() // we're done: pop the dump off the stack
		return nil
	}

	vm.regGoroutine = g
	vm.push(Value{Tag: TagGoroutine, Data: g})
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
	vm.push(Value{
		Tag:  TagDump,
		Data: dump,
	})
}

func opPushBool(vm *VM, _ OpCode, operand uint) error {
	vm.push(Value{Tag: TagBool, Data: operand == 1})
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
		val, err := vm.peek()
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

	top, err := vm.peek()
	if err != nil {
		return err
	}
	switch top.Tag {
	case TagDiff:
		if len(m) != 3 {
			return ErrExpectedDiffAssign
		}
		val, _ := vm.peek()
		diff, ok := val.Data.(*Diff)
		if !ok {
			t := reflect.TypeOf(val.Data)
			return fmt.Errorf("%w: diff value was a %v", ErrWrongTag, t)
		}
		err := vm.assign(m[0], Value{TagDump, diff.Left})
		if err != nil {
			return err
		}
		err = vm.assign(m[1], Value{TagDump, diff.Common})
		if err != nil {
			return err
		}
		err = vm.assign(m[2], Value{TagDump, diff.Right})
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
		return fmt.Errorf("missing predicate (%w)", err)
	}
	if val == (instruction == OpCodeJumpIfTrue) {
		vm.frame.ip = int(addr) - 1
	}
	return nil
}

func opJumpTo(vm *VM, _ OpCode, addr uint) error {
	vm.frame.ip = int(addr) - 1
	return nil
}

func opCall(vm *VM, _ OpCode, chunkIndex uint) error {
	frame := &frame{
		ip:         -1,
		returnAddr: vm.frame.ip,
		chunk:      vm.code.chunks[chunkIndex],
	}
	vm.frames = append(vm.frames, frame)
	vm.frame = frame
	return nil
}

func opReturn(vm *VM, _ OpCode, _ uint) error {
	ip := vm.frame.returnAddr
	vm.frames = vm.frames[:len(vm.frames)-1]
	vm.frame = vm.frames[len(vm.frames)-1]
	vm.frame.ip = ip
	return nil
}

func opCommandGetWorkingDir(vm *VM, _ OpCode, _ uint) error {
	fmt.Fprint(vm.wOut, vm.cwd+"\n")
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

// ConfirmationAction lets us close over a function and smuggle it back out to
// the REPL for execution
type ConfirmationAction struct {
	fn     func()
	prompt string
}

func (c ConfirmationAction) Error() string {
	return c.prompt
}

func (c ConfirmationAction) Run() {
	c.fn()
}

func opCommandEmpty(vm *VM, _ OpCode, _ uint) error {
	if vm.pragma.EmptyConfirm {
		confirm := ConfirmationAction{
			fn:     func() { vm.env = map[string]Value{} },
			prompt: "Are you sure you want to empty workspace? [y/N] ",
		}
		return fmt.Errorf("%w: %w", ErrCommandConfirm, confirm)
	}

	vm.env = map[string]Value{}
	return ErrCommandOk
}

func opCommandQuit(vm *VM, _ OpCode, _ uint) error {
	if vm.pragma.ExitConfirm {
		confirm := ConfirmationAction{
			fn:     func() { os.Exit(0) },
			prompt: "Are you sure you want to quit? [y/N] ",
		}
		return fmt.Errorf("%w: %w", ErrCommandConfirm, confirm)
	}

	return ErrCommandQuit
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
	case "debug.disassemble":
		fmt.Fprintf(vm.wOut, "%s\n", vm.pragma.DebugDisassemble)
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
		vm.wOut.useColor = vm.pragma.ShowColor
		vm.wErr.useColor = vm.pragma.ShowColor
	case "show.count":
		err = popAndSet(vm, &vm.pragma.ShowCount)
	case "ls.format":
		err = popAndSet(vm, &vm.pragma.ListFormat)
	case "show.dedup":
		err = popAndSet(vm, &vm.pragma.ShowDedup)
	case "vars.display":
		err = popAndSet(vm, &vm.pragma.VarsDisplay)
	case "debug.disassemble":
		err = popAndSet(vm, &vm.pragma.DebugDisassemble)
	default:
		return fmt.Errorf("%w pragma.%s", ErrNoSuchPragma, setting)
	}
	if err != nil {
		return err
	}

	return ErrCommandOk
}

func popAndSet[T any](vm *VM, setting *T) error {
	raw, err := vm.pop()
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
					fmt.Fprintf(vm.wOut, "%s: %d\n", name, dump.Len())
				} else {
					return vm.newWrongTagErr(v)
				}
			}
		}
	case PragmaDisplaySummary:
		for _, name := range vars {
			v := vm.env[name]
			if v.Tag == TagDump {
				if dump, ok := v.Data.(*GoroutineDump); ok {
					dump.Summary(vm.wOut, name)
				} else {
					return vm.newWrongTagErr(v)
				}
			}
		}
	case PragmaDisplayNone:
		out := strings.Join(vars, "\t")
		fmt.Fprint(vm.wOut, out)
	default:
		return fmt.Errorf(
			"%w %q: expected \"count\", \"summary\", or \"none\"",
			ErrInvalidOpArg, mode)
	}

	return ErrCommandOk
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

	dump.Show(vm.wOut, PragmaDedup(vm.pragma.ShowDedup), limit, offset)
	vm.didShow = true
	return nil
}

func opFuncToJSON(vm *VM, _ OpCode, _ uint) error {
	path, err := vm.popString()
	if err != nil {
		return err
	}
	dump, err := vm.peekDump()
	if err != nil {
		return err
	}
	buf, err := json.MarshalIndent(dump.goroutines, "", "  ")
	if err != nil {
		return err
	}

	vm.didShow = true
	fmt.Fprintln(vm.wOut, string(buf))
	if path == "" {
		return nil
	}
	return writeToFile(path, buf)
}
