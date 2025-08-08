package evaluator

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const initialStackCap = 256

type VM struct {
	chunk *Chunk
	ip    int // instruction pointer in chunk
	stack []Value

	// TODO: we should have a copy of the environment that we write to on each
	// pass through run, which only gets flattened into the env when complete
	env map[string]Value

	regGoroutine *Goroutine
	regDumpDst   *GoroutineDump
}

func NewVM() *VM {
	return &VM{
		stack: make([]Value, 0, initialStackCap),
		env:   make(map[string]Value),
	}
}

func (vm *VM) reset(chunk *Chunk) {
	vm.ip = 0
	vm.chunk = chunk
	vm.stack = make([]Value, 0, initialStackCap)
}

func (vm *VM) readByte() (Op, error) {
	if vm.ip >= len(vm.chunk.ops) {
		return 0, ErrEOF
	}
	op := vm.chunk.ops[vm.ip]
	vm.ip++
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

		case OpCodeGreater, OpCodeEqual:
			err = vm.comparison(instruction)

		case OpCodeContains:
			err = vm.contains(instruction)

		case OpCodeAnd, OpCodeOr:
			err = vm.binaryLogic(instruction)

		case OpCodeLoadIdentifier:
			err = vm.loadEnv(operand)

		case OpCodeTempDump:
			vm.regDumpDst = NewGoroutineDump()

		case OpCodePushDump:
			vm.Push(Value{
				Tag:  TagDump,
				Data: vm.regDumpDst,
			})

		case OpCodeStartIter:
			err = vm.handleStartIter()

		case OpCodeNextGoroutine:
			err = vm.handleNextGoroutine(operand)

		case OpCodeJumpTo:
			err = vm.handleJumpTo(operand)

		case OpCodeJumpIfTrue:
			err = vm.handleConditionalJump(operand, true)

		case OpCodeJumpIfFalse:
			err = vm.handleConditionalJump(operand, false)

		case OpCodeStoreEnv:
			err = vm.storeEnv(operand)

		case OpCodeAssignment:
			err = vm.handleAssignment(operand)

		case OpCodeLoadGoroutineDump:
			err = vm.loadEnv(operand)
			// TODO: would be nice to do a type assertion here

		case OpCodeAddGoroutine:
			vm.regDumpDst.Add(vm.regGoroutine)

		case OpCodeLoadEnv:
			err = vm.loadEnv(operand)

		case OpCodeLoadFieldAccessor:
			err = vm.handleFieldAccessor(operand)
		case OpCodeFunction:
			err = vm.loadAndExecCommand()
		default:
			return NoValue, fmt.Errorf("%w %s", ErrNoSuchOpCode, instruction)
		}
		if err != nil {
			return NoValue, err
		}

	}

	return vm.Pop()
}

var (
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
)

// TODO: shouldn't we be short-circuiting evaluation?
func (vm *VM) binaryLogic(instruction OpCode) error {

	left, err := vm.popBool()
	if err != nil {
		return err
	}
	right, err := vm.popBool()
	if err != nil {
		return err
	}

	switch instruction {
	case OpCodeAnd:
		if right && left {
			vm.Push(Value{Tag: TagBool, Data: true})
			return nil
		}
	case OpCodeOr:
		if right || left {
			vm.Push(Value{Tag: TagBool, Data: true})
			return nil
		}
	}

	vm.Push(Value{Tag: TagBool, Data: false})
	return nil
}

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
		switch instruction {
		case OpCodeGreater:
			val = left.Data.(int) > right.Data.(int)
		case OpCodeEqual:
			val = left.Data.(int) == right.Data.(int)
		}
	case string:
		switch instruction {
		case OpCodeGreater:
			val = left.Data.(string) > right.Data.(string)
		case OpCodeEqual:
			val = left.Data.(string) == right.Data.(string)
		}
	}
	vm.Push(Value{Tag: TagBool, Data: val})
	return nil
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
		return fmt.Errorf("%w expected string for contains", ErrInvalidType)
	}
	vm.Push(Value{Tag: TagBool, Data: val})
	return nil
}

func (vm *VM) popBool() (bool, error) {
	val, err := vm.Pop()
	if err != nil {
		return false, err
	}
	if b, ok := val.Data.(bool); !ok {
		return false, fmt.Errorf("%w pop bool", ErrInvalidType)
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

func (vm *VM) loadEnv(index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	name := con.(string)
	val := vm.env[name] // TODO: what if env is empty?
	vm.Push(val)
	return nil
}

func (vm *VM) storeEnv(index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	name, ok := con.(string)
	if !ok {
		return fmt.Errorf(
			"%w store: expected identifier got %v",
			ErrInvalidType, con)
	}
	val, err := vm.Pop()
	if err != nil {
		return err
	}

	vm.env[name] = val
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

func (vm *VM) handleFieldAccessor(index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}

	g := vm.regGoroutine
	if g == nil {
		return fmt.Errorf("%w: no goroutine", ErrUnexpectedRegisterState)
	}

	str := con.(string)
	switch str {
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

func (vm *VM) handleStartIter() error {
	val, err := vm.Peek()
	if err != nil {
		return err
	}
	if val.Tag != TagDump {
		return vm.newInvalidTypeErr(TagDump, val.Tag)
	}
	dump := val.Data.(*GoroutineDump)
	dump.StartIter()

	// get the address we'll jump to when we're done
	addr, err := vm.readAddr()
	if err != nil {
		return err
	}
	vm.Push(Value{Tag: TagNumber, Data: addr})

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
		// subsequent calls: need to clean up the previous goroutine
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
		vm.ip = int(addr)
		vm.regGoroutine = nil
		return nil
	}
	g := dump.Next()
	if g == nil {
		vm.ip = int(addr)
		vm.regGoroutine = nil
		return nil
	}

	// TODO: do we want a goroutine here or would be easier to stick this on the
	// stack?
	vm.regGoroutine = g
	vm.Push(Value{Tag: TagGoroutine, Data: g})
	return nil
}

// handleAssignment writes the object at the top of the stack to the
// environment, but leaves it on the stack
func (vm *VM) handleAssignment(index uint) error {
	con, err := vm.fetchConstant(index)
	if err != nil {
		return err
	}
	name, ok := con.(string)
	if !ok {
		return fmt.Errorf(
			"%w assignment: expected identifier got %v",
			ErrInvalidType, con)
	}
	val, err := vm.Peek()
	if err != nil {
		return err
	}

	vm.env[name] = val
	return nil
}

func (vm *VM) readAddr() (int, error) {
	addr, err := vm.readByte()
	if err != nil {
		return 0, err
	}
	if addr > math.MaxInt {
		return 0, fmt.Errorf(
			"%w: address=%d", ErrOutOfStackBounds, addr)
	}
	return int(addr), nil
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
		vm.ip = int(index)
	}
	return nil
}

func (vm *VM) handleJumpTo(addr uint) error {
	vm.ip = int(addr)
	return nil
}

var commandCodes = []string{
	"cd", "empty", "exit", "help", "ls", "pwd", "pragma", "quit", "vars",
}

var commandFns = map[string]Command{}

func (vm *VM) loadAndExecCommand() error {
	index, err := vm.readByte()
	if err != nil {
		return fmt.Errorf("%w: missing index byte", ErrExpectedCommand)
	}
	if int(index) > len(commandCodes) {
		return fmt.Errorf("%w: no such command with index %d", ErrExpectedCommand, index)
	}

	name := commandCodes[index]
	cmd := commandFns[name]
	return cmd(vm)
}

// Commands have side-effects on the VM state
type Command func(*VM) error
