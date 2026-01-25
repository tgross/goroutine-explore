package evaluator

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

const initialStackCap = 256
const defaultGas = 1024 * 1024

type VM struct {
	chunk *Chunk
	ip    int // instruction pointer in chunk
	stack []Value

	// gas is how many instructions can be retired before halting (prevents
	// infinite loops)
	gas int

	// TODO: we should have a copy of the environment that we write to on each
	// pass through run, which only gets flattened into the env when complete
	env map[string]Value
	cwd *os.Root

	regGoroutine *Goroutine
	regDumpDst   *GoroutineDump
}

type vmConfig struct {
	cwd string
}

func NewVM(cfg *vmConfig) (*VM, error) {
	root, err := os.OpenRoot(cfg.cwd)
	if err != nil {
		return nil, err
	}

	return &VM{
		stack: make([]Value, 0, initialStackCap),
		env:   make(map[string]Value),
		cwd:   root,
		gas:   defaultGas,
	}, nil
}

func (vm *VM) reset(chunk *Chunk) {
	vm.ip = -1
	vm.chunk = chunk
	vm.stack = make([]Value, 0, initialStackCap)
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

func (vm *VM) run() (Value, error) {
	for {
		vm.gas--
		if vm.gas <= 0 {
			return NoValue, fmt.Errorf("%w (%d)", ErrOutOfGas, defaultGas)
		}

		op, err := vm.readByte()
		if err != nil {
			break // only error this returns is EOF
		}
		instruction, operand := op.decode()
		switch instruction {
		case OpCodeLoadNumber:
			err = vm.loadNumber(operand)

		case OpCodeLoadString:
			err = vm.loadString(operand)

		case OpCodeGreater, OpCodeGreaterEqual,
			OpCodeLess, OpCodeLessEqual,
			OpCodeEqual, OpCodeNotEqual:
			err = vm.comparison(instruction)

		case OpCodeContains:
			err = vm.contains(instruction)

		case OpCodeTempDump:
			vm.regDumpDst = NewGoroutineDump()

		case OpCodePushDump:
			vm.pushDump(vm.regDumpDst)

		case OpCodeNextGoroutine:
			err = vm.handleNextGoroutine(operand)

		case OpCodeJumpTo:
			err = vm.handleJumpTo(operand)

		case OpCodeJumpIfTrue:
			err = vm.handleConditionalJump(operand, true)

		case OpCodeJumpIfFalse:
			err = vm.handleConditionalJump(operand, false)

		case OpCodeAssignment:
			err = vm.handleAssignment(operand)

		case OpCodeLoadGoroutineDump:
			err = vm.loadEnv(operand)
			// TODO: would be nice to do a type assertion here

		case OpCodeAddGoroutine:
			vm.regDumpDst.Add(vm.regGoroutine)

		case OpCodeLoadFieldAccessor:
			err = vm.handleFieldAccessor(operand)

		case OpCodePushBool:
			vm.Push(Value{Tag: TagBool, Data: operand == 1})

		case OpCodeCommandChangeDir:
			err = vm.commandChangeDir(operand)
			return NoValue, err

		case OpCodeCommandEmpty:
			vm.env = map[string]Value{}
			return NoValue, nil

		case OpCodeCommandGetWorkingDir:
			return vm.commandGetWorkDir(), nil

		case OpCodeCommandQuit:
			// TODO: how do we expect caller to detect this and quit?
			return NoValue, err

		case OpCodeCommandHelp:
			return vm.commandHelp(operand)

		case OpCodeCommandListDir:
		case OpCodeCommandVars:
		case OpCodeCommandPragma:
			// TODO: probably just need a function for each one?
			//err = vm.loadAndExecCommand()

		case OpCodeFuncUnion:
			err = vm.handleUnion()

		case OpCodeFuncDiff:
			err = vm.handleDiff()

		case OpCodeFuncIntersect:
			err = vm.handleIntersect()

		case OpCodeFuncShowDump:
		case OpCodeFuncLoad:
		case OpCodeFuncSave:

		default:
			return NoValue, fmt.Errorf("%w %s", ErrNoSuchOpCode, instruction)
		}
		if err != nil {
			return NoValue, err
		}

	}

	// TODO: shouldn't we just pop everything off the stack so we can print it
	// all?
	return vm.Pop()
}

var (
	ErrOutOfGas                  = errors.New("processed more than maximum number of instructions")
	ErrEmptyStack                = errors.New("tried to pop off empty stack")
	ErrUnexpectedStackState      = errors.New("unexpected stack state")
	ErrUnexpectedRegisterState   = errors.New("unexpected register state")
	ErrOutOfStackBounds          = errors.New("jump outside of stack bounds")
	ErrInvalidType               = errors.New("invalid type for operation")
	ErrNoSuchOpCode              = errors.New("no such op code")
	ErrExpectedConstantValueByte = errors.New("expected value after constant load byte")
	ErrExpectedJumpAddress       = errors.New("expected address for jump")
	ErrExpectedCommand           = errors.New("expected valid command after command byte")
	ErrNoSuchEnv                 = errors.New("no identifer with name")
	ErrExpectedDiffAssign        = errors.New("assigning a diff must have 3 identifiers or \"_\"")
)

func (vm *VM) comparison(instruction OpCode) error {
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

func (vm *VM) contains(instruction OpCode) error {
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

func (vm *VM) loadNumber(index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	num := con.(int)
	vm.Push(Value{Tag: TagNumber, Data: num})
	return nil
}

func (vm *VM) loadString(index uint) error {
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

func (vm *VM) loadEnv(index uint) error {
	name, err := vm.fetchString(index)
	if err != nil {
		return fmt.Errorf("%w: expected name of a variable or constant", err)
	}

	val := vm.env[name] // TODO: what if env is empty?
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

func (vm *VM) handleUnion() error {

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

func (vm *VM) handleIntersect() error {

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

func (vm *VM) handleDiff() error {
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

func (vm *VM) handleFieldAccessor(index uint) error {
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
		vm.Push(Value{Tag: TagNumber, Data: g.Lines})
	case "duration", ".duration":
		vm.Push(Value{Tag: TagNumber, Data: g.Duration})
	case "state", ".state":
		vm.Push(Value{Tag: TagString, Data: g.State})
	}

	return nil
}

// starting stack:
// - dump
// ending stack:
// - dump
// - goroutine
// OR (when complete)
// - dump
func (vm *VM) handleNextGoroutine(addr uint) error {
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

func (vm *VM) pushDump(dump *GoroutineDump) {
	dump.StartIter() // reset before we push it back onto the stack
	vm.Push(Value{
		Tag:  TagDump,
		Data: dump,
	})
}

// handleAssignment writes the object at the top of the stack to the
// environment, but leaves it on the stack
func (vm *VM) handleAssignment(index uint) error {
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

func (vm *VM) handleConditionalJump(index uint, expected bool) error {
	val, err := vm.Pop()
	if err != nil {
		return err
	}
	if val.Tag != TagBool {
		return fmt.Errorf(
			"%w conditional jump: %s (%#v) at address=%d",
			ErrInvalidType, val.Tag, val.Data, index)
	}
	if val.Data.(bool) == expected {
		vm.ip = int(index) - 1
	}
	return nil
}

func (vm *VM) handleJumpTo(addr uint) error {
	vm.ip = int(addr) - 1
	return nil
}

func (vm *VM) commandGetWorkDir() Value {
	return Value{
		Tag:  TagString,
		Data: vm.cwd.Name(),
	}
}

func (vm *VM) commandChangeDir(index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	path, ok := con.(string)
	if !ok {
		return fmt.Errorf("cd requires a string argument")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	vm.cwd = root
	return nil
}

func (vm *VM) commandHelp(index uint) (Value, error) {
	// TODO: what about when we have no topic?
	con, err := vm.fetchConstant(index)
	if err != nil {
		return NoValue, err
	}
	topic, ok := con.(string)
	if !ok {
		return NoValue, fmt.Errorf("help topics must be strings")
	}

	return Value{
		Tag: TagString,
		// TODO: lookup help for topic here?
		Data: fmt.Sprintf("help for topic: %s", topic),
	}, nil
}
