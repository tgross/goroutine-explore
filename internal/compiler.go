// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type CompilerError struct {
	tok   Token
	inner error
}

func compileErr(tok Token, msg string, args ...any) CompilerError {
	inner := fmt.Errorf(msg, args...)
	return CompilerError{tok, inner}
}

func (e CompilerError) Unwrap() error {
	return e.inner
}

func (e CompilerError) Error() string {
	return e.inner.Error()
}

var ErrTooManyArgs = errors.New("too many arguments")
var ErrMissingArgs = errors.New("expected more arguments")
var ErrExpectedPath = errors.New("expected a path argument")
var ErrExpectedMaybeType = errors.New("expected optional")
var ErrInvalidArg = errors.New("invalid argument")

type Compiler struct {
	// internal state which is reset for each Compile call
	tokenizer *Tokenizer
	code      *Code  // all chunks + constants
	chunk     *Chunk // currently written chunk in code

	// grammar consisting of parselet functions and precedence tables
	prefixParseFns      map[TokenType]parseFn
	prefixPrecedenceTab map[TokenType]int
	infixParseFns       map[TokenType]parseFn
	infixPrecedenceTab  map[TokenType]int
}

type BindingPower = int

const (
	BindingNone BindingPower = iota
	BindingComma
	BindingAssignment
	BindingPipe
	BindingFunc
	BindingOr
	BindingAnd
	BindingEq
	BindingCompare
	BindingUnary
	BindingCall
	BindingPrimary
	BindingAtom
)

type parseFn func(Token) error

func NewCompiler() *Compiler {
	p := &Compiler{
		prefixParseFns:      make(map[TokenType]parseFn),
		prefixPrecedenceTab: make(map[TokenType]int),
		infixParseFns:       make(map[TokenType]parseFn),
		infixPrecedenceTab:  make(map[TokenType]int),
		code:                NewCode(),
	}

	setupPrefix := func(tok TokenType, prec int, parselet parseFn) {
		p.prefixParseFns[tok] = parselet
		p.prefixPrecedenceTab[tok] = prec
	}

	setupInfix := func(tok TokenType, prec int, parselet parseFn) {
		p.infixParseFns[tok] = parselet
		p.infixPrecedenceTab[tok] = prec
	}

	setupPrefix(TokenLeftParen, BindingNone, p.parseParenExpr)
	setupPrefix(TokenIdentifier, BindingAtom, p.parseDumpAccessor)
	setupPrefix(TokenFieldAccessor, BindingAtom, p.parseFieldAccessor)
	setupPrefix(TokenString, BindingAtom, p.parseString)
	setupPrefix(TokenNumber, BindingAtom, p.parseNumber)
	setupPrefix(TokenFunction, BindingFunc, p.parseFunction)
	setupPrefix(TokenMethod, BindingFunc, p.parseMethod)
	setupPrefix(TokenCommand, BindingFunc, p.parseCommand)
	setupPrefix(TokenPragma, BindingFunc, p.parsePragma)

	setupInfix(TokenRightParen, BindingNone, nil)
	setupInfix(TokenKeywordAnd, BindingAnd, p.parseBinaryLogicExpr)
	setupInfix(TokenKeywordOr, BindingOr, p.parseBinaryLogicExpr)
	setupInfix(TokenEqual, BindingEq, p.parseCompareExpr)
	setupInfix(TokenNotEqual, BindingEq, p.parseCompareExpr)
	setupInfix(TokenRegexMatch, BindingEq, p.parseCompareExpr)
	setupInfix(TokenRegexNotMatch, BindingEq, p.parseCompareExpr)
	setupInfix(TokenGreaterEqualThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenGreaterThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenLessEqualThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenLessThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenKeywordContains, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenKeywordIn, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenKeywordMatches, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenPipe, BindingPipe, p.parsePipeExpr)
	setupInfix(TokenMethod, BindingFunc, p.parseMethod)

	return p
}

func (p *Compiler) Compile(tokenizer *Tokenizer) (*Code, error) {
	p.code = NewCode()
	p.chunk = p.code.chunks[0]
	p.tokenizer = tokenizer

	err := p.parseExpr(0)
	if err != nil {
		return p.code, err
	}

	p.optimizePredicates()
	return p.code, nil
}

func (p *Compiler) parseExpr(precedence int) error {
	tok, err := p.tokenizer.Next()
	if err != nil {
		return compileErr(tok, "%w", err)
	}

	// every expression will start with a prefix expression, even if it's
	// actually an operand of a later infix expression
	prefix, ok := p.prefixParseFns[tok.Type]
	if !ok {
		return compileErr(tok,
			"expected expression to start with an identifier or open paren")
	}

	err = prefix(tok)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return nil
		}
		return err
	}

	for {
		tok, err = p.tokenizer.Peek()
		if err != nil {
			if errors.Is(err, ErrEOF) {
				break
			}
			return compileErr(tok, "%w", err)
		}

		if infixPrec, ok := p.infixPrecedenceTab[tok.Type]; ok {
			if infixPrec < precedence {
				break
			}
		}

		tok, err := p.tokenizer.Next()
		if err != nil {
			return compileErr(tok, "%w", err)
		}

		infix, ok := p.infixParseFns[tok.Type]
		if !ok || infix == nil {
			return nil
		}
		err = infix(tok)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Compiler) parseDumpAccessor(tok Token) error {
	m := newMultiAssignment()
	return p.parseDumpAccessorImpl(m, tok)
}

func (p *Compiler) parseDumpAccessorImpl(m MultiAssignment, tok Token) error {
	tokIdx := p.createConst(tok.Lexeme)
	m = append(m, tokIdx)

	next, err := p.tokenizer.Peek()
	if err != nil {
		if errors.Is(err, ErrEOF) {
			if len(m) > 1 {
				for _, idx := range m {
					p.emitBytes(OpCodeLoadGoroutineDump, uint(idx))
				}
				return nil
			}
			p.emitLoadConst(OpCodeLoadGoroutineDump, tok.Lexeme)
			return nil
		}
		return err
	}

	switch next.Type {
	case TokenAssign:
		_, _ = p.consume(TokenAssign)
		err := p.parseExpr(BindingAssignment)
		if err != nil {
			return err
		}

		if len(m) > 1 {
			p.emitLoadConst(OpCodeAssignment, m)
			return nil
		}
		p.emitLoadConst(OpCodeAssignment, tok.Lexeme)
		return nil
	case TokenComma:
		_, _ = p.consume(TokenComma)
		tok, err := p.consume(TokenIdentifier)
		if err != nil {
			return err
		}
		return p.parseDumpAccessorImpl(m, tok)
	case TokenMethod:
		fun, err := p.consume(TokenMethod)
		if err != nil {
			return err
		}
		p.emitLoadConst(OpCodeLoadGoroutineDump, tok.Lexeme)
		return p.parseMethod(fun)
	default:
		p.emitLoadConst(OpCodeLoadGoroutineDump, tok.Lexeme)
		return nil
	}
}

func (p *Compiler) parseFieldAccessor(tok Token) error {
	p.emitLoadConst(OpCodeLoadFieldAccessor, tok.Lexeme)
	return nil
}

func (p *Compiler) parseString(tok Token) error {
	p.emitLoadConst(OpCodeLoadString, tok.Lexeme)
	return nil
}

func (p *Compiler) parseNumber(tok Token) error {
	val, err := strconv.Atoi(tok.Lexeme)
	if err != nil {
		return compileErr(tok, "%w", err)
	}

	p.emitLoadConst(OpCodeLoadNumber, val)
	return nil
}

func (p *Compiler) parseBool(tok Token) error {
	val, err := strconv.ParseBool(tok.Lexeme)
	if err != nil {
		return compileErr(tok, "%w", err)
	}
	if val {
		p.emitBytes(OpCodePushBool, 1)
	} else {
		p.emitBytes(OpCodePushBool, 0)
	}

	return nil
}

func (p *Compiler) parseBinaryLogicExpr(tok Token) error {

	var addrShort int

	// after evaluating the left-hand side, if we can't short-circuit we jump to
	// the right-hand side expression. Otherwise we push the bool back onto the
	// stack and jump past the right-hand side expression.
	switch tok.Type {
	case TokenKeywordAnd:
		addrShort = p.emitBytes(OpCodeJumpIfTrue, OpCodePatchPlaceholder)
		p.emitBytes(OpCodePushBool, 0)
	case TokenKeywordOr:
		addrShort = p.emitBytes(OpCodeJumpIfFalse, OpCodePatchPlaceholder)
		p.emitBytes(OpCodePushBool, 1)
	}
	addrLong := p.emitBytes(OpCodeJumpTo, OpCodePatchPlaceholder)

	err := p.parseExpr(p.infixPrecedenceTab[tok.Type] + 1)
	if err != nil {
		return err
	}

	offset := len(p.chunk.ops) - addrLong - 2
	p.patchJump(addrShort, -offset)
	p.patchJump(addrLong, 1)

	return nil
}

func (p *Compiler) parseCompareExpr(tok Token) error {
	err := p.parseExpr(p.infixPrecedenceTab[tok.Type] + 1)
	if err != nil {
		return err
	}

	switch tok.Type {
	case TokenEqual:
		p.emitByte(OpCodeEqual)
	case TokenNotEqual:
		p.emitByte(OpCodeNotEqual)
	case TokenGreaterThan:
		p.emitByte(OpCodeGreater)
	case TokenGreaterEqualThan:
		p.emitByte(OpCodeGreaterEqual)
	case TokenLessThan:
		p.emitByte(OpCodeLess)
	case TokenLessEqualThan:
		p.emitByte(OpCodeLessEqual)
	case TokenKeywordContains:
		p.emitByte(OpCodeContains)
	case TokenKeywordIn:
		p.emitByte(OpCodeIn)
	case TokenKeywordMatches, TokenRegexMatch:
		p.emitByte(OpCodeRegexMatches)
	case TokenRegexNotMatch:
		p.emitByte(OpCodeRegexNotMatches)
	}

	return nil
}

func (p *Compiler) parseParenExpr(tok Token) error {
	err := p.parseExpr(BindingNone + 1)
	if err != nil {
		return err
	}
	return p.expect(TokenRightParen)
}

func (p *Compiler) parsePipeExpr(tok Token) error {
	// note that a pipe must *always* be followed by method with the first
	// argument being made implicit
	fun, err := p.consume(TokenFunction)
	if err != nil {
		return err
	}
	return p.parseMethod(fun)
}

// parseMethod is a parser for functions where the receiver has already been
// parsed so it'll be on the stack when the function is called in the VM; in the
// case of a pipe this will actually be a free function. This parser emits
// identical bytecode for `receiver | fun(arg2)` and `receiver.func(arg2)`, and
// the parseFunction parser emits the same bytecode for `func(receiver, arg2)`
func (p *Compiler) parseMethod(fun Token) error {
	err := p.expect(TokenLeftParen)
	if err != nil {
		return err
	}
	return p.parseFunctionArgs(fun)
}

// parseFunction is a parser for functions where the receiver has not already
// been parsed so it needs to be parsed from the arguments, such as
// `func(receiver, arg2)`. This parser emits the same bytecode as parseMethod
// does for expressions like `receiver | fun(arg2)` and `receiver.func(arg2)`
func (p *Compiler) parseFunction(fun Token) error {
	err := p.expect(TokenLeftParen)
	if err != nil {
		return err
	}

	// receiver expression
	err = p.parseExpr(BindingCall)
	if err != nil {
		return err
	}
	return p.parseFunctionArgs(fun)
}

// parseCommand is a parser for commands, which may or may not have arguments
func (p *Compiler) parseCommand(cmd Token) error {
	_, err := p.consume(TokenLeftParen)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			sig, ok := signatures[cmd.Lexeme]
			if !ok {
				panic("got a command token for a non-command")
			}
			if sig.minArgs > 0 {
				return compileErr(cmd, "%w for %q command, got none",
					ErrMissingArgs, cmd.Lexeme)
			}
			p.emitByte(sig.op)
			return nil
		}
		return err
	}

	return p.parseFunctionArgs(cmd)
}

type argType byte

const (
	optional argType = 1 << iota
	expr
	numeric
	predicate
	str
	identifier
	field
	pragma
)

// signatures are the type signature of every function, not including the
// receiver, which is always an expression
var signatures = map[string]struct {
	op      OpCode
	args    []argType
	minArgs int
}{
	"union":     {OpCodeFuncUnion, []argType{expr}, 1},
	"diff":      {OpCodeFuncDiff, []argType{expr}, 1},
	"intersect": {OpCodeFuncIntersect, []argType{expr}, 1},
	"load":      {OpCodeFuncLoad, []argType{str}, 1},
	"save":      {OpCodeFuncSave, []argType{str}, 1},
	"as":        {OpCodeAssignment, []argType{identifier}, 1},
	"show": {OpCodeFuncShowDump, []argType{
		numeric | optional, numeric | optional}, 0},
	"json": {OpCodeFuncToJSON, nil, 0},
	"dot":  {OpCodeFuncToDot, nil, 0},

	// these have more complex handling so we don't have a single OpCode
	"where":  {OpCodeNoop, []argType{predicate}, 1},
	"delete": {OpCodeNoop, []argType{predicate}, 1},
	"graph":  {OpCodeFuncGraph, []argType{predicate}, 1},

	// commands
	"cd":    {OpCodeCommandChangeDir, []argType{str}, 1},
	"ls":    {OpCodeCommandListDir, nil, 0},
	"empty": {OpCodeCommandEmpty, nil, 0},
	"exit":  {OpCodeCommandQuit, nil, 0},
	"quit":  {OpCodeCommandQuit, nil, 0},
	"help":  {OpCodeCommandHelp, []argType{str | optional}, 0},
	"pwd":   {OpCodeCommandGetWorkingDir, nil, 0},
	"vars":  {OpCodeCommandVars, nil, 0},
}

func (p *Compiler) parseFunctionArgs(fun Token) error {
	sig, ok := signatures[fun.Lexeme]
	if !ok {
		return compileErr(fun, "no such function %q", fun.Lexeme)
	}

	for i, arg := range sig.args {
		if arg&optional == optional {
			next, err := p.tokenizer.Peek()
			if err != nil {
				return compileErr(next, "%w", err)
			}
			if next.Type == TokenRightParen {
				// get default from pragma
				p.emitLoadConst(OpCodeLoadNumber, 0)
				continue
			}
		}
		if i > 0 {
			err := p.expect(TokenComma)
			if err != nil {
				return err
			}
		}
		switch {
		case arg&expr == expr:
			err := p.parseExpr(BindingCall)
			if err != nil {
				return err
			}
		case arg&numeric == numeric:
			tok, err := p.consume(TokenNumber)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
			err = p.parseNumber(tok)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
		case arg&str == str:
			tok, err := p.consume(TokenString)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
			err = p.parseString(tok)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
		case arg&identifier == identifier:
			tok, err := p.consume(TokenIdentifier)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
			err = p.parseDumpAccessor(tok)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
		case arg&predicate == predicate:
			err := p.parseFilter(fun)
			if err != nil {
				return fmt.Errorf("%w for %s: %w", ErrInvalidArg, fun.Lexeme, err)
			}
		}
	}
	if sig.op != OpCodeNoop {
		p.emitByte(sig.op)
	}

	if fun.Type == TokenCommand {
		_, err := p.maybeConsume(TokenRightParen)
		if err != nil {
			return err
		}
		return p.expectNoMoreArgs()
	}

	err := p.expect(TokenRightParen)
	if err != nil {
		return err
	}

	return nil
}

// patchJump takes an address and patches the instruction at that address to
// point to the last OpCode in the chunk + any offset (to allow patching past
// the current chunk)
func (p *Compiler) patchJump(addr, offset int) {
	end := len(p.chunk.ops) - 1 + offset
	if addr > len(p.chunk.ops) {
		panic(fmt.Sprintf("trying to patch too far back: len(chunk)=%d target=%d",
			len(p.chunk.ops), addr))
	}
	instruction := p.chunk.ops[addr]
	op, _ := instruction.decode()
	switch op {
	case OpCodeJumpIfFalse, OpCodeJumpIfTrue, OpCodeJumpTo, OpCodeNextGoroutine:
	default:
		panic(fmt.Sprintf("trying to patch a non-jump instruction: len(chunk)=%d target=%d instruction=%v", len(p.chunk.ops), addr, op))
	}

	p.chunk.ops[addr] = encode(op, uint(end))
}

func (p *Compiler) parseFilter(tok Token) error {
	switch tok.Lexeme {
	case "graph":
		p.emitByte(OpCodeDup)
	default:
	}
	p.emitByte(OpCodeTempDump)
	addr := p.emitBytes(OpCodeNextGoroutine, OpCodePatchPlaceholder)

	err := p.parsePredicate()
	if err != nil && !errors.Is(err, ErrEOF) {
		return err
	}

	switch tok.Lexeme {
	case "where", "graph":
		// reject, so continue to next goroutine
		p.emitBytes(OpCodeJumpIfFalse, uint(addr))
	case "delete":
		p.emitBytes(OpCodeJumpIfTrue, uint(addr))
	}

	// keep this goroutine in temp dump
	p.emitByte(OpCodeAddGoroutine)
	p.emitBytes(OpCodeJumpTo, uint(addr))

	// when we run out of goroutines we need to jump to the end of the loop
	// where we push the temporary dump onto the stack
	p.emitByte(OpCodePushDump)
	p.patchJump(addr, 0)

	return nil
}

func (p *Compiler) parsePredicate() error {
	p.chunk = NewChunk()
	p.code.chunks = append(p.code.chunks, p.chunk)
	chunkIndex := len(p.code.chunks) - 1

	err := p.parseExpr(BindingFunc)
	if err != nil && !errors.Is(err, ErrEOF) {
		return err
	}
	p.emitByte(OpCodeReturn)
	p.chunk = p.code.chunks[0]
	p.emitBytes(OpCodeCall, uint(chunkIndex))
	return nil
}

func (p *Compiler) parsePragma(_ Token) error {
	topicTok, err := p.tokenizer.Next()
	if err != nil {
		if errors.Is(err, ErrEOF) {
			p.emitLoadConst(OpCodeLoadString, "*.*")
			p.emitBytes(OpCodeCommandGetPragma, 0)
			return nil
		}
		return compileErr(topicTok, "%w", err)
	}
	topic := strings.TrimPrefix(topicTok.Lexeme, ".")
	keyTok, err := p.tokenizer.Next()
	if err != nil {
		if errors.Is(err, ErrEOF) {
			p.emitLoadConst(OpCodeLoadString, topic+".*")
			p.emitBytes(OpCodeCommandGetPragma, 0)
			return nil
		}
		return compileErr(keyTok, "%w", err)
	}
	setting := fmt.Sprintf("%s%s", topic, keyTok.Lexeme)
	tok, err := p.maybeConsume(TokenAssign)
	if err != nil {
		return err
	}
	if tok == EmptyToken {
		p.emitLoadConst(OpCodeLoadString, setting)
		p.emitBytes(OpCodeCommandGetPragma, 0)
		return nil
	}

	valTok, err := p.tokenizer.Next()
	if err != nil {
		return compileErr(valTok, "%w", err)
	}

	switch setting {
	case "empty.confirm", "exit.confirm", "show.color":
		err = p.parseBool(valTok)
	case "show.count", "limits.stack", "limits.steps":
		err = p.parseNumber(valTok)
	case "ls.format":
		err = p.parseString(valTok)
	case "show.dedup":
		switch valTok.Lexeme {
		case PragmaDedupIDs, PragmaDedupNone, PragmaDedupNumber:
			err = p.parseString(valTok)
		default:
			return compileErr(valTok,
				`invalid pragma value: expected one of "ids", "number", or "none"`)
		}
	case "debug.disassemble":
		switch valTok.Lexeme {
		case PragmaDebugDisassembleNone, PragmaDebugDisassembleOnError, PragmaDebugDisassembleOnReturn:
			err = p.parseString(valTok)
		default:
			return compileErr(valTok,
				`invalid pragma value: expected one of "none", "error", or "return"`)
		}
	case "vars.display":
		switch valTok.Lexeme {
		case PragmaDisplayCount, PragmaDisplayNone, PragmaDisplaySummary:
			err = p.parseString(valTok)
		default:
			return compileErr(valTok,
				`invalid pragma value: expected one of "count", "summary", or "none"`)
		}
	}
	if err != nil {
		return compileErr(valTok, "%w", err)
	}

	p.emitLoadConst(OpCodeLoadString, setting)
	p.emitByte(OpCodeCommandSetPragma)
	return nil
}

func (p *Compiler) expectNoMoreArgs() error {
	tok, err := p.tokenizer.Peek()
	if err == nil {
		return compileErr(tok, "%w", ErrTooManyArgs)
	}
	if errors.Is(err, ErrEOF) {
		return nil
	}
	return err
}

func (p *Compiler) consume(want TokenType) (Token, error) {
	tok, err := p.tokenizer.Peek()
	if err != nil {
		return EmptyToken, compileErr(tok, "%w", err)
	}
	if tok.Type != want {
		return EmptyToken, compileErr(tok, "expected %v got %v", want, tok.Type)
	}
	tok, err = p.tokenizer.Next()
	if err != nil {
		return tok, compileErr(tok, "%w", err)
	}
	return tok, nil
}

// maybeConsume returns the wanted token if there are any more tokens at
// all. Returns EmptyToken but no error if there's no more tokens at all,
// otherwise returns an error
func (p *Compiler) maybeConsume(want TokenType) (Token, error) {
	tok, err := p.tokenizer.Peek()
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return EmptyToken, nil
		}
		return EmptyToken, compileErr(tok, "%w", err)
	}
	if tok.Type != want {
		return EmptyToken, compileErr(tok, "%w %v", ErrExpectedMaybeType, want)
	}
	tok, err = p.tokenizer.Next()
	if err != nil {
		return tok, compileErr(tok, "%w", err)
	}
	return tok, nil
}

func (p *Compiler) expect(want TokenType) error {
	tok, err := p.tokenizer.Peek()
	if err != nil {
		return compileErr(tok, "expected %v, got error %v", want, err)
	}
	if tok.Type != want {
		return compileErr(tok, "expected %v, got %v", want, tok.Type)
	}
	_, err = p.tokenizer.Next()
	if err != nil {
		return compileErr(tok, "%w", err)
	}
	return err
}

func (p *Compiler) emitByte(op OpCode) int {
	p.chunk.ops = append(p.chunk.ops, encode(op, 0))
	return len(p.chunk.ops) - 1
}

func (p *Compiler) emitBytes(op OpCode, val uint) int {
	p.chunk.ops = append(p.chunk.ops, encode(op, val))
	return len(p.chunk.ops) - 1
}

func (p *Compiler) emitLoadConst(op OpCode, x any) int {
	idx := p.createConst(x)
	p.emitBytes(op, uint(idx))
	return idx
}

func (p *Compiler) createConst(x any) int {
	idx := slices.Index(p.code.constants, x)
	if idx > -1 {
		return idx
	}

	p.code.constants = append(p.code.constants, x)
	return len(p.code.constants) - 1
}

// optimizedPredicates finds adjacent where/delete filter expressions and
// compacts their loops together so that we call the predicate chunks for each
// goroutine without having to loop again. This significantly improves
// performance for larger goroutine dumps when the initial filter in a chain is
// "weak" and doesn't filter many values.
func (p *Compiler) optimizePredicates() {

	for origIndex := 0; origIndex < len(p.chunk.ops); origIndex++ {
		_, _, ok := nextFilterExpr(p.chunk, origIndex)
		if !ok {
			continue
		}

		start, newIndex := uint(origIndex), uint(origIndex)
		nextAddr := start + 1 // address we'll patch for OpCodeNextGoroutine
		toPatchOut := uint(0)
		var found bool

	OUTER:
		for {
			origIndex += 7 // advance past previous filter
			callOp, jumpOp, ok := nextFilterExpr(p.chunk, origIndex)
			if !ok {
				break OUTER
			}
			if !found && ok {
				newIndex += 4
			}
			found = true

			jumpCode, _ := jumpOp.decode()
			p.chunk.ops[newIndex] = callOp

			p.chunk.ops[newIndex+1] = encode(jumpCode, nextAddr)
			newIndex += 2
			toPatchOut += 5
		}

		if found {
			p.chunk.ops[newIndex] = encode(OpCodeAddGoroutine, 0)
			p.chunk.ops[newIndex+1] = encode(OpCodeJumpTo, nextAddr)
			p.chunk.ops[newIndex+2] = encode(OpCodePushDump, 0)
			p.chunk.ops[start+1] = encode(OpCodeNextGoroutine, newIndex+2)
			newIndex += 3

			// we've removed instructions which we could replace with nops but
			// that makes finding the next opportunity to look for a window to
			// optimize harder, so patch them out and shift the remaining ops
			// instead
			_ = p.patchOut(p.chunk, newIndex, toPatchOut)
		}
	}
}

var filterOps = []OpCode{
	OpCodeTempDump,
	OpCodeNextGoroutine,
	OpCodeCall,
	OpCodeNoop, // hole for conditional jump
	OpCodeAddGoroutine,
	OpCodeJumpTo,
	OpCodePushDump,
}

func nextFilterExpr(chunk *Chunk, i int) (callOp, jumpOp Op, ok bool) {
	if len(chunk.ops) < i+7 {
		return // not enough ops for another filter expression
	}

	code00, _ := chunk.ops[i].decode()   // temp dump
	code01, _ := chunk.ops[i+1].decode() // next goroutine
	code02, _ := chunk.ops[i+2].decode() // call
	// skip i+3 which is the jump
	code04, _ := chunk.ops[i+4].decode() // add goroutine
	code05, _ := chunk.ops[i+5].decode() // jump back to next
	code06, _ := chunk.ops[i+6].decode() // push
	if !slices.Equal(filterOps,
		[]OpCode{
			code00,
			code01,
			code02,
			OpCodeNoop,
			code04,
			code05,
			code06,
		}) {
		return
	}
	callOp = chunk.ops[i+2]
	jumpOp = chunk.ops[i+3]
	ok = true
	return
}

func (p *Compiler) patchOut(chunk *Chunk, first, by uint) error {
	ops := slices.Clone(chunk.ops[:first])
	tail := slices.Clone(chunk.ops[first+by:])
	ops = append(ops, tail...)

	for i := first; i < uint(len(ops)); i++ {
		op := ops[i]
		code, addr := op.decode()
		switch code {
		case OpCodeJumpIfFalse, OpCodeJumpTo, OpCodeJumpIfTrue, OpCodeNextGoroutine:
			if addr >= first && addr < first+by {
				return fmt.Errorf("patch out found address inside patched window (op=%02d addr=%d)", i, addr)
			}
			if addr >= uint(first) {
				addr -= uint(by)
				ops[i] = encode(code, addr)
			}
		default:
			ops[i] = encode(code, addr)
		}
	}
	chunk.ops = ops
	return nil
}
