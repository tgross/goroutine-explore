package evaluator

import (
	"errors"
	"fmt"
	"strconv"
)

type Compiler struct {
	// internal state which is reset for each Compile call
	tokenizer *Tokenizer
	chunk     *Chunk

	// grammar consisting of parselet functions and precedence tables
	prefixParseFns      map[TokenType]parseFn
	prefixPrecedenceTab map[TokenType]int
	infixParseFns       map[TokenType]parseFn
	infixPrecedenceTab  map[TokenType]int
}

type BindingPower = int

const (
	BindingNone BindingPower = iota
	BindingAssignment
	BindingPipe
	BindingFunc
	BindingOr
	BindingAnd
	BindingEq
	BindingCompare
	BindingUnary
	BindingPrimary
	BindingAtom
)

type parseFn func(Token) error

func newCompiler() *Compiler {
	p := &Compiler{
		prefixParseFns:      make(map[TokenType]parseFn),
		prefixPrecedenceTab: make(map[TokenType]int),
		infixParseFns:       make(map[TokenType]parseFn),
		infixPrecedenceTab:  make(map[TokenType]int),
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
	setupInfix(TokenRightParen, BindingNone, nil)
	setupPrefix(TokenIdentifier, BindingAtom, p.parseDumpAccessor)
	setupPrefix(TokenFieldAccessor, BindingAtom, p.parseFieldAccessor)
	setupPrefix(TokenString, BindingAtom, p.parseString)
	setupPrefix(TokenNumber, BindingAtom, p.parseNumber)
	setupPrefix(TokenFunction, BindingFunc, p.parseFunction)
	setupPrefix(TokenKeywordWhere, BindingFunc, p.parseWhere)
	setupPrefix(TokenKeywordDelete, BindingFunc, p.parseDelete)

	setupInfix(TokenKeywordAnd, BindingAnd, p.parseBinaryLogicExpr)
	setupInfix(TokenKeywordOr, BindingOr, p.parseBinaryLogicExpr)

	setupInfix(TokenEqual, BindingEq, p.parseCompareExpr)
	setupInfix(TokenNotEqual, BindingEq, p.parseCompareExpr)
	setupInfix(TokenGreaterEqualThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenGreaterThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenLessEqualThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenLessThan, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenKeywordContains, BindingCompare, p.parseCompareExpr)
	setupInfix(TokenFunctionBinary, BindingFunc, p.parseBinaryFunction)

	setupInfix(TokenAssign, BindingAssignment, p.parseAssignment)
	setupInfix(TokenKeywordWhere, BindingFunc, p.parseWhere)
	setupInfix(TokenKeywordDelete, BindingFunc, p.parseDelete)
	setupInfix(TokenPipe, BindingPipe, p.parsePipeExpr)

	//setupInfix(TokenComma, 9, p.parseComma)

	// TODO: do we need arithmetic operators at all?
	setupInfix(TokenPlus, 2, p.parseBinaryArithmeticExpr)
	setupInfix(TokenMinus, 2, p.parseBinaryArithmeticExpr)
	setupInfix(TokenStar, 3, p.parseBinaryArithmeticExpr)
	setupInfix(TokenSlash, 3, p.parseBinaryArithmeticExpr)

	return p
}

func (p *Compiler) Compile(tokenizer *Tokenizer) (*Chunk, error) {
	p.chunk = NewChunk()
	p.tokenizer = tokenizer

	tok, err := p.maybeConsume(TokenCommand)
	if err != nil {
		return nil, err
	}
	if tok != EmptyToken {
		return p.chunk, p.parseCommand(tok)
	}

	return p.chunk, p.parseExpr(0)
}

func (p *Compiler) parseExpr(precedence int) error {
	tok, err := p.tokenizer.Next()
	if err != nil {
		return ErrEOF
	}

	// every expression will start with a prefix expression, even if it's
	// actually an operand of a later infix expression
	prefix, ok := p.prefixParseFns[tok.Type]
	if !ok {
		// TODO: this error message is terrible
		return fmt.Errorf(
			"expected an identifier or open paren: %+v", tok)
	}

	err = prefix(tok)
	if err != nil {
		return err
	}

	for {
		tok, err = p.tokenizer.Peek()
		if err != nil {
			if errors.Is(err, ErrEOF) {
				break
			}
			return err
		}

		if infixPrec, ok := p.infixPrecedenceTab[tok.Type]; ok {
			if infixPrec < precedence {
				break
			}
		}

		tok, err := p.tokenizer.Next()
		if err != nil {
			return err
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
	next, err := p.tokenizer.Peek()
	if err != nil {
		if err == ErrEOF {
			p.emitLoadConst(OpCodeLoadGoroutineDump, tok.Lexeme)
			return nil
		}
		return err
	}
	if next.Type == TokenAssign {
		// assignment will consume this token from the last constant
		p.createConst(tok.Lexeme)
		return nil
	}
	if next.Type == TokenComma {
		p.createConst(tok.Lexeme)
		_, _ = p.consume(TokenComma)
		// TODO: need to read to next item
	}

	p.emitLoadConst(OpCodeLoadGoroutineDump, tok.Lexeme)
	return nil
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
		return err
	}

	p.emitLoadConst(OpCodeLoadNumber, val)
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
	}

	return nil
}

func (p *Compiler) parseBinaryArithmeticExpr(tok Token) error {
	err := p.parseExpr(p.infixPrecedenceTab[tok.Type])
	if err != nil {
		return err
	}

	switch tok.Type {
	case TokenPlus, TokenMinus, TokenStar, TokenSlash:
		// TODO: implement, if we even want these?
		return nil
	}
	return nil
}

func (p *Compiler) parseParenExpr(tok Token) error {
	err := p.parseExpr(BindingPipe + 1)
	if err != nil {
		return err
	}
	return p.expect(TokenRightParen)
}

func (p *Compiler) expectNoMoreArgs() error {
	_, err := p.tokenizer.Peek()
	if err == nil {
		return ErrTooManyArgs
	}
	if errors.Is(err, ErrEOF) {
		return nil
	}
	return err
}

func (p *Compiler) consume(want TokenType) (Token, error) {
	tok, err := p.tokenizer.Peek()
	if err != nil {
		return EmptyToken, err
	}
	if tok.Type != want {
		// TODO: we didn't implement stringer for Token but if we're going to
		// return it in errors we probably should
		return EmptyToken, fmt.Errorf("expected %v, got %s", want, tok)
	}
	tok, err = p.tokenizer.Next()
	return tok, err
}

func (p *Compiler) maybeConsume(want TokenType) (Token, error) {
	tok, err := p.tokenizer.Peek()
	if err != nil {
		return EmptyToken, nil
	}
	if tok.Type != want {
		return EmptyToken, nil
	}
	tok, err = p.tokenizer.Next()
	return tok, err
}

func (p *Compiler) expect(want TokenType) error {
	tok, err := p.tokenizer.Peek()
	if err != nil {
		return err
	}
	if tok.Type != want {
		// TODO: we didn't implement stringer for Token but if we're going to
		// return it in errors we probably should
		return fmt.Errorf("expected %v, got %s", want, tok)
	}
	_, err = p.tokenizer.Next()
	return err
}

// TODO: there's currently no optimization of pipeline expressions, so we'll end
// up shallow-copying the goroutine dump frequently on long pipelines. Maybe do
// some peephole optimization on the chunk when we're done?
func (p *Compiler) parsePipeExpr(tok Token) error {
	return p.parseExpr(0)
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

func (p *Compiler) parseAssignment(tok Token) error {
	nameIdx := len(p.chunk.constants) - 1
	for {
		err := p.parseExpr(BindingAssignment)
		if err != nil {
			return err
		}
		tok, err := p.tokenizer.Peek()
		if tok.Type == TokenPipe || errors.Is(err, ErrEOF) {
			break
		}
		_, err = p.tokenizer.Next()
		if err != nil {
			return err
		}
	}

	p.emitBytes(OpCodeAssignment, uint(nameIdx))
	return nil
}

func (p *Compiler) parseWhere(tok Token) error {
	return p.parseFilter(tok, true)
}

func (p *Compiler) parseDelete(tok Token) error {
	return p.parseFilter(tok, false)
}

func (p *Compiler) parseFilter(tok Token, keepOnMatch bool) error {

	p.emitByte(OpCodeTempDump)
	addr := p.emitBytes(OpCodeNextGoroutine, OpCodePatchPlaceholder)

	err := p.parseExpr(BindingFunc)
	if err != nil && !errors.Is(err, ErrEOF) {
		return err
	}

	if keepOnMatch {
		// reject, so continue to next goroutine
		p.emitBytes(OpCodeJumpIfFalse, uint(addr))
	} else {
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

func (p *Compiler) parseCommand(tok Token) error {
	name := tok.Lexeme
	switch name {
	case "empty":
		p.emitByte(OpCodeCommandEmpty)
	case "exit", "quit":
		p.emitByte(OpCodeCommandQuit)
	case "help":
		p.emitByte(OpCodeCommandHelp)
	case "ls":
		p.emitByte(OpCodeCommandListDir)
	case "pwd":
		p.emitByte(OpCodeCommandGetWorkingDir)
	case "vars":
		p.emitByte(OpCodeCommandVars)
	case "cd":
		err := p.parsePath()
		if err != nil {
			return fmt.Errorf("invalid arguments for cd: %w", err)
		}
		p.emitByte(OpCodeCommandChangeDir)
	case "pragma":
		err := p.parsePragma()
		if err != nil {
			return err
		}
		p.emitByte(OpCodeCommandPragma)
	default:
		panic("unknown command") // TODO
	}

	return p.expectNoMoreArgs()
}

var ErrTooManyArgs = errors.New("too many arguments")
var ErrExpectedPath = errors.New("expected a path argument")

func (p *Compiler) parsePath() error {
	var ok bool
	path := ""

	for {
		tok, err := p.tokenizer.Peek()
		if tok.Type == TokenPipe || errors.Is(err, ErrEOF) {
			break
		}
		tok, err = p.tokenizer.Next()
		if err != nil {
			return err
		}
		ok = true
		path += tok.Lexeme
	}
	if !ok { // empty!
		return ErrExpectedPath
	}

	_ = p.parseString(Token{Type: TokenString, Lexeme: path})
	return nil
}

func (p *Compiler) parsePragma() error {
	return nil // TODO
}

func (p *Compiler) parseFunction(tok Token) error {
	switch tok.Lexeme {
	case "load":
		err := p.parsePath()
		if err != nil {
			return err
		}
		p.emitByte(OpCodeFuncLoad)
		return nil
	case "save":
		err := p.parsePath()
		if err != nil {
			return err
		}
		p.emitByte(OpCodeFuncSave)
		return nil
	case "as":
		tok, err := p.consume(TokenIdentifier)
		if err != nil {
			return err
		}
		nameIdx := p.createConst(tok.Lexeme)
		p.emitBytes(OpCodeAssignment, uint(nameIdx))
		return nil
	case "show":
		tokCount, err := p.maybeConsume(TokenNumber)
		if err != nil {
			return err
		}
		if tokCount == EmptyToken {
			p.emitLoadConst(OpCodeLoadNumber, 0) // TODO: pragma?
		} else {
			_ = p.parseNumber(tokCount)
		}

		tokOffset, err := p.maybeConsume(TokenNumber)
		if err != nil {
			return err
		}
		if tokOffset == EmptyToken {
			p.emitLoadConst(OpCodeLoadNumber, 0) // TODO: pragma?
		} else {
			_ = p.parseNumber(tokOffset)
		}
		p.emitByte(OpCodeFuncShowDump)
		return nil
	default:
		return fmt.Errorf("no such function %q", tok.Lexeme)
	}
}

func (p *Compiler) parseBinaryFunction(tok Token) error {
	name := tok.Lexeme

	err := p.parseExpr(BindingFunc)
	if err != nil && !errors.Is(err, ErrEOF) {
		return err
	}

	switch name {
	case "union":
		p.emitByte(OpCodeFuncUnion)
	case "intersect":
		p.emitByte(OpCodeFuncIntersect)
	case "diff":
		p.emitByte(OpCodeFuncDiff)

	default:
		// TODO: this is always a programmer error?
		panic("no such binary function")
	}

	return nil
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
	p.chunk.constants = append(p.chunk.constants, x)
	p.emitBytes(op, uint(len(p.chunk.constants)-1))
	return len(p.chunk.ops) - 1
}

// TODO: we don't de-duplicate
func (p *Compiler) createConst(x any) int {
	p.chunk.constants = append(p.chunk.constants, x)
	return len(p.chunk.constants) - 1
}
