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
	suffixParseFns      map[TokenType]parseFn
	suffixPrecedenceTab map[TokenType]int
}

type parseFn func(Token) error

func newCompiler() *Compiler {
	p := &Compiler{
		prefixParseFns:      make(map[TokenType]parseFn),
		prefixPrecedenceTab: make(map[TokenType]int),
		infixParseFns:       make(map[TokenType]parseFn),
		infixPrecedenceTab:  make(map[TokenType]int),
		suffixParseFns:      make(map[TokenType]parseFn),
		suffixPrecedenceTab: make(map[TokenType]int),
	}

	setupPrefix := func(tok TokenType, prec int, parselet parseFn) {
		p.prefixParseFns[tok] = parselet
		p.prefixPrecedenceTab[tok] = prec
	}

	setupInfix := func(tok TokenType, prec int, parselet parseFn) {
		p.infixParseFns[tok] = parselet
		p.infixPrecedenceTab[tok] = prec
	}

	setupSuffix := func(tok TokenType, prec int, parselet parseFn) {
		p.suffixParseFns[tok] = parselet
		p.suffixPrecedenceTab[tok] = prec
	}

	setupBinary := func(tok TokenType, prec int) {
		setupInfix(tok, prec, p.parseBinaryExpr)
	}

	setupPrefix(TokenLeftParen, 0, p.parseParenExpr)
	setupPrefix(TokenIdentifer, 0, p.parseDumpAccessor)
	setupPrefix(TokenFieldAccessor, 0, p.parseFieldAccessor)
	setupPrefix(TokenString, 0, p.parseString)
	setupPrefix(TokenNumber, 0, p.parseNumber)
	setupPrefix(TokenFunction, 8, p.parseFunction)
	setupPrefix(TokenCommand, 8, p.parseCommand)
	setupPrefix(TokenKeywordWhere, 8, p.parseWhere)
	setupPrefix(TokenKeywordDelete, 8, p.parseDelete)

	setupBinary(TokenKeywordAnd, 3)
	setupBinary(TokenKeywordOr, 3)
	setupBinary(TokenEqual, 4)
	setupBinary(TokenNotEqual, 4)
	setupBinary(TokenGreaterEqualThan, 4)
	setupBinary(TokenGreaterThan, 4)
	setupBinary(TokenLessEqualThan, 4)
	setupBinary(TokenLessThan, 4)
	setupBinary(TokenKeywordContains, 4)

	// TODO: do we need arithmetic operators at all?
	setupBinary(TokenPlus, 2)
	setupBinary(TokenMinus, 2)
	setupBinary(TokenStar, 3)
	setupBinary(TokenSlash, 3)

	setupInfix(TokenAssign, 10, p.parseAssignment)
	setupInfix(TokenKeywordWhere, 8, p.parseWhere)
	setupInfix(TokenKeywordDelete, 8, p.parseDelete)

	setupSuffix(TokenPipe, 2, p.parsePipeExpr)

	return p
}

func (p *Compiler) Compile(tokenizer *Tokenizer) (*Chunk, error) {
	p.chunk = NewChunk()
	p.tokenizer = tokenizer
	err := p.parseExpr(0)
	if err != nil {
		// we may have a partial chunk here, so returning it makes debugging
		// easier even though this isn't ideomatic
		return p.chunk, err
	}
	return p.chunk, nil
}

func (p *Compiler) parseExpr(precedence int) error {
	tok, err := p.tokenizer.Next()
	if err != nil {
		return ErrEOF
	}

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
			if precedence >= infixPrec {
				break
			}
		}
		if suffixPrec, ok := p.suffixPrecedenceTab[tok.Type]; ok {
			if precedence >= suffixPrec {
				break
			}
		}

		tok, err := p.tokenizer.Next()
		if err != nil {
			return err
		}

		suffix, ok := p.suffixParseFns[tok.Type]
		if ok {
			err = suffix(tok)
			if err != nil {
				return err
			}
			break
		}

		infix, ok := p.infixParseFns[tok.Type]
		if !ok {
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

func (p *Compiler) parseBinaryExpr(tok Token) error {
	err := p.parseExpr(p.infixPrecedenceTab[tok.Type])
	if err != nil {
		return err
	}

	// TODO: currently we don't optimize for short-circuiting
	switch tok.Type {
	case TokenKeywordAnd:
		p.emitByte(OpCodeAnd)
	case TokenKeywordOr:
		p.emitByte(OpCodeOr)
	case TokenGreaterThan:
		p.emitByte(OpCodeGreater)
	case TokenEqual:
		p.emitByte(OpCodeEqual)
	case TokenKeywordContains:
		p.emitByte(OpCodeContains)
	}

	return nil
}

func (p *Compiler) parseParenExpr(tok Token) error {
	err := p.parseExpr(0)
	if err != nil {
		return err
	}
	return p.expect(TokenRightParen)
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
// up shallow-copying the goroutine dump frequently on long pipelines
func (p *Compiler) parsePipeExpr(tok Token) error {
	return nil
}

// emitJump emits the jump OpCode passed in as well as a placeholder for the
// address, which will be returned so that we can patch the address later
func (p *Compiler) emitJump(jumpOp OpCode) int {
	p.emitBytes(jumpOp, OpCodePatchPlaceholder)
	return len(p.chunk.ops) - 1
}

// patchJump takes an address and patches the instruction at that address to
// point to the last OpCode in the chunk
func (p *Compiler) patchJump(addr int) {
	end := len(p.chunk.ops) - 1
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
		err := p.parseExpr(1)
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

	err := p.parseExpr(1)
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
	p.patchJump(addr)

	return nil
}

// TODO: unimplemented, but seems like it'll have the same issue as
// parseFunction in terms of where we fan out from token to implementation in
// the VM
func (p *Compiler) parseCommand(tok Token) error {
	name := tok.Lexeme
	switch name {
	case "ls":
		// TODO, etc.
	default:
	}
	return nil
}

var funcCodes = []string{
	"as", "delete", "diff", "intersect", "load", "save", "show", "union",
}

func (p *Compiler) parseFunction(tok Token) error {
	name := tok.Lexeme
	for {
		err := p.parseExpr(1)
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
	// TODO: this is a little goofy because we'll end up needing another map in
	// the VM to go from the function-specific OpCode to a function. Should we
	// just have an OpCode for each function?
	var fnCode OpCode
	for i, fn := range funcCodes {
		if fn == name {
			fnCode = OpCode(i)
			break
		}
	}
	if fnCode == OpCode(0) {
		return fmt.Errorf("unknown function: %v", name)
	}

	p.emitBytes(OpCodeFunction, uint(fnCode))
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
