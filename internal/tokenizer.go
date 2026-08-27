// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"text/scanner"
	"unicode"
)

var ErrEOF = errors.New("EOF")
var EmptyToken = Token{}

type Token struct {
	Type   TokenType
	Lexeme string // text of the token
	Pos    scanner.Position
}

func (t Token) String() string {
	return fmt.Sprintf("Token{Type: %v, Pos: %d:%d, Lexeme: %q}",
		t.Type, t.Pos.Line, t.Pos.Column, t.Lexeme)
}

//go:generate stringer -type TokenType -linecomment
type TokenType uint8

const (
	TokenInvalid    TokenType = iota // invalid
	TokenPipe                        // pipe
	TokenLeftParen                   // left paren
	TokenRightParen                  // right paren
	TokenPlus                        // +
	TokenMinus                       // -
	TokenStar                        // *
	TokenSlash                       // /
	TokenBang                        // !
	TokenComma                       // ,
	TokenAssign                      // =

	// comparisons
	TokenEqual            // ==
	TokenNotEqual         // !=
	TokenLessThan         // <
	TokenLessEqualThan    // <=
	TokenGreaterThan      // >
	TokenGreaterEqualThan // >=
	TokenKeywordContains  // contains
	TokenKeywordMatches   // match
	TokenKeywordIn        // in
	TokenRegexMatch       // =~
	TokenRegexNotMatch    // !~

	// literals
	TokenIdentifier    // identifier
	TokenString        // string
	TokenNumber        // number
	TokenFieldAccessor // field accessor

	// logic
	TokenKeywordAnd // and
	TokenKeywordOr  // or

	//
	TokenCommand  // command
	TokenFunction // function
	TokenMethod   // method
	TokenPragma   // pragma
)

type Tokenizer struct {
	scanner scanner.Scanner
	peeked  Token
	ctx     context.Context
}

func NewTokenizer() *Tokenizer {
	s := scanner.Scanner{}

	s.Mode = scanner.ScanIdents | scanner.ScanChars |
		scanner.ScanInts | scanner.ScanStrings | scanner.SkipComments

	// identifiers can start with . or $ and can have internal _
	s.IsIdentRune = func(ch rune, i int) bool {
		return (ch == '$' || ch == '.') && i == 0 ||
			unicode.IsLetter(ch) ||
			unicode.IsDigit(ch) && i > 0 ||
			ch == '_' && i != 0
	}

	return &Tokenizer{
		scanner: s,
		peeked:  EmptyToken,
	}
}

func (s *Tokenizer) Reset(ctx context.Context, body io.Reader) {
	s.scanner.Init(body)
	s.peeked = EmptyToken
	s.ctx = ctx
}

// Tokens returns an iterator over the source and yields an EOF error when out
// of tokens
func (s *Tokenizer) Tokens() iter.Seq2[Token, error] {
	return func(yield func(Token, error) bool) {
		for {
			if !yield(s.Next()) {
				return
			}
		}
	}
}

// Peek looks at the next token but doesn't consume it from the source, such
// that a subsequent call to Next will return this same token
func (s *Tokenizer) Peek() (Token, error) {
	if s.peeked != EmptyToken {
		return s.peeked, nil
	}
	t, err := s.next()
	if err == nil {
		s.peeked = t
	}
	return t, err
}

// Next returns the next token
func (s *Tokenizer) Next() (Token, error) {
	if s.peeked != EmptyToken {
		peeked := s.peeked
		s.peeked = EmptyToken
		return peeked, nil
	}

	return s.next()
}

// next pulls the next token but without handling previous Peek calls
func (s *Tokenizer) next() (Token, error) {
	select {
	case <-s.ctx.Done():
		return EmptyToken, s.ctx.Err()
	default:
	}

	tok := s.scanner.Scan()
	if tok == scanner.EOF {
		return Token{Pos: s.scanner.Position}, ErrEOF
	}

	token := Token{
		Lexeme: s.scanner.TokenText(),
		Pos:    s.scanner.Position,
	}

	switch tok {
	case scanner.Ident:

		token.Type = TokenIdentifier
		// we can't detect these invalid single-char identifiers in the scanner
		if token.Lexeme == "." || token.Lexeme == "$" {
			return token, fmt.Errorf("invalid identifier")
		}

		switch token.Lexeme {
		case "and":
			token.Type = TokenKeywordAnd
		case "or":
			token.Type = TokenKeywordOr
		case "where", "delete", "as", "save", "graph",
			"show", "diff", "intersect", "union", "json", "dot":
			token.Type = TokenFunction
		case "load":
			// load takes no expression argument, so treat it like a method
			token.Type = TokenMethod
		case ".where", ".delete", ".as", ".load", ".save", ".graph",
			".show", ".diff", ".intersect", ".union", ".json", ".dot":
			token.Type = TokenMethod
			token.Lexeme = strings.TrimPrefix(token.Lexeme, ".")

		case "contains":
			token.Type = TokenKeywordContains
		case "in":
			token.Type = TokenKeywordIn
		case "matches":
			token.Type = TokenKeywordMatches
		case "cd", "empty", "exit", "help", "ls", "pwd", "quit", "vars":
			token.Type = TokenCommand
		case "pragma":
			token.Type = TokenPragma
		case "id", "header",
			"trace", "lines",
			"duration", "state",
			"createdby", "createdBy",
			"labels", "label":
			token.Lexeme = "." + token.Lexeme
			token.Type = TokenFieldAccessor
		}
		if strings.HasPrefix(token.Lexeme, ".") {
			token.Type = TokenFieldAccessor
		}

	case scanner.Int:
		token.Type = TokenNumber
	case scanner.String:
		token.Type = TokenString
		token.Lexeme, _ = strings.CutPrefix(token.Lexeme, `"`)
		token.Lexeme, _ = strings.CutSuffix(token.Lexeme, `"`)
	case scanner.RawString:
		token.Type = TokenString
		token.Lexeme, _ = strings.CutPrefix(token.Lexeme, "`")
		token.Lexeme, _ = strings.CutSuffix(token.Lexeme, "`")

	case '|':
		token.Type = TokenPipe
	case '(':
		token.Type = TokenLeftParen
	case ')':
		token.Type = TokenRightParen
	case ',':
		token.Type = TokenComma

		// we don't support arithmetic operators but needs these for parsing
		// unquoted path strings
	case '+':
		token.Type = TokenPlus
	case '-':
		token.Type = TokenMinus
	case '/':
		token.Type = TokenSlash
	case '*':
		token.Type = TokenStar

	case '=':
		token.Type = TokenAssign
		peek := s.scanner.Peek()
		switch peek {
		case '=':
			s.scanner.Next()
			token.Lexeme = "=="
			token.Type = TokenEqual
		case '~':
			s.scanner.Next()
			token.Lexeme = "!~"
			token.Type = TokenRegexMatch
		}
	case '!':
		token.Type = TokenBang
		peek := s.scanner.Peek()
		switch peek {
		case '=':
			s.scanner.Next()
			token.Lexeme = "!="
			token.Type = TokenNotEqual
		case '~':
			s.scanner.Next()
			token.Lexeme = "!~"
			token.Type = TokenRegexNotMatch
		}
	case '>':
		token.Type = TokenGreaterThan
		peek := s.scanner.Peek()
		if peek == '=' {
			s.scanner.Next()
			token.Lexeme = ">="
			token.Type = TokenGreaterEqualThan
		}
	case '<':
		token.Type = TokenLessThan
		peek := s.scanner.Peek()
		if peek == '=' {
			s.scanner.Next()
			token.Lexeme = "<="
			token.Type = TokenLessEqualThan
		}

	default:
		return Token{}, errors.New("invalid token")

	}

	return token, nil
}
