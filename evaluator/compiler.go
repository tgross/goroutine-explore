package evaluator

import (
	"errors"
	"fmt"
	"slices"
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
	BindingComma
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
	setupInfix(TokenKeywordWhere, BindingFunc, p.parseWhere)
	setupInfix(TokenKeywordDelete, BindingFunc, p.parseDelete)
	setupInfix(TokenPipe, BindingPipe, p.parsePipeExpr)

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
		return err
	}

	p.emitLoadConst(OpCodeLoadNumber, val)
	return nil
}

func (p *Compiler) parseBool(tok Token) error {
	val, err := strconv.ParseBool(tok.Lexeme)
	if err != nil {
		return err
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
		return p.parsePragma()
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
	topicTok, err := p.tokenizer.Next()
	if err != nil {
		return err
	}
	keyTok, err := p.consume(TokenFieldAccessor)
	if err != nil {
		return err
	}
	valTok, err := p.tokenizer.Next()
	if err != nil {
		return err
	}

	setting := fmt.Sprintf("%s%s", topicTok.Lexeme, keyTok.Lexeme)

	switch setting {
	case "empty.confirm", "exit.confirm", "show.color":
		p.parseBool(valTok)
	case "show.count":
		p.parseNumber(valTok)
	case "ls.format":
		p.parseString(valTok)
	case "show.dedup":
		switch valTok.Lexeme {
		case PragmaDedupIDs, PragmaDedupNone, PragmaDedupNumber:
			p.parseString(valTok)
		default:
			return fmt.Errorf(
				"%w: expected one of ids, number, none", ErrInvalidArg)
		}
	case "vars.display":
		switch valTok.Lexeme {
		case PragmaDisplayCount, PragmaDisplayNone, PragmaDisplaySummary:
			p.parseString(valTok)
		default:
			return fmt.Errorf(
				"%w: expected one of count, summary, none", ErrInvalidArg)
		}
	}

	p.emitLoadConst(OpCodeLoadString, setting)
	p.emitByte(OpCodeCommandPragma)
	return nil
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
		// limit argument
		tokLimit, err := p.maybeConsume(TokenNumber)
		if err != nil {
			return err
		}
		if tokLimit == EmptyToken {
			p.emitLoadConst(OpCodeLoadNumber, 0) // get default from pragma
			p.emitLoadConst(OpCodeLoadNumber, 0) // get default from pragma
			p.emitByte(OpCodeFuncShowDump)
			return nil
		}

		// offset argument
		tokOffset, err := p.maybeConsume(TokenNumber)
		if err != nil {
			return err
		}
		if tokOffset == EmptyToken {
			p.emitLoadConst(OpCodeLoadNumber, 0) // get default from pragma
		} else {
			_ = p.parseNumber(tokOffset)
		}
		_ = p.parseNumber(tokLimit)

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
	idx := p.createConst(x)
	p.emitBytes(op, uint(idx))
	return idx
}

func (p *Compiler) createConst(x any) int {
	idx := slices.Index(p.chunk.constants, x)
	if idx > -1 {
		return idx
	}

	p.chunk.constants = append(p.chunk.constants, x)
	return len(p.chunk.constants) - 1
}
