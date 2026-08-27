// Copyright (c) 2021-2026 The goroutine-explore contributors
// SPDX-License-Identifier: BlueOak-1.0.0

package internal

import (
	"iter"
	"strings"
	"testing"
	"text/scanner"

	"github.com/shoenig/test/must"
)

func TestTokenizer_SmokeTest(t *testing.T) {
	t.Parallel()

	src := `g.where(.duration > 10 and .state == "select" or .label.worker_id == "1")
              | union(g1) | as(g2) | show()`

	body := strings.NewReader(src)
	tokenizer := NewTokenizer()
	tokenizer.Reset(t.Context(), body)
	next, stop := iter.Pull2(tokenizer.Tokens())
	t.Cleanup(stop)

	expectedTypes := []TokenType{
		TokenIdentifier,    // g
		TokenMethod,        // .where
		TokenLeftParen,     // (
		TokenFieldAccessor, // .duration
		TokenGreaterThan,   // >
		TokenNumber,        // 10
		TokenKeywordAnd,    // and
		TokenFieldAccessor, // .state
		TokenEqual,         // ==
		TokenString,        // "select"
		TokenKeywordOr,     // or
		TokenFieldAccessor, // .label
		TokenFieldAccessor, // .worker_id
		TokenEqual,         // =-
		TokenString,        // "1"
		TokenRightParen,    // )
		TokenPipe,          // |
		TokenFunction,      // union
		TokenLeftParen,     // (
		TokenIdentifier,    // g1
		TokenRightParen,    // )
		TokenPipe,          // |
		TokenFunction,      // as
		TokenLeftParen,     // (
		TokenIdentifier,    // g2
		TokenRightParen,    // )
		TokenPipe,          // |
		TokenFunction,      // show
		TokenLeftParen,     // (
		TokenRightParen,    // )
	}

	got := testCollectTokens(t, next)
	gotTypes := []TokenType{}
	for _, token := range got {
		gotTypes = append(gotTypes, token.Type)
	}
	must.Eq(t, expectedTypes, gotTypes)
}

func TestTokenizer_CommandArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		src    string
		expect []Token
	}{
		{
			src: `pragma.show.dedup = number`,
			expect: []Token{
				{TokenPragma, `pragma`, scanner.Position{
					Filename: "", Offset: 0, Line: 1, Column: 1}},
				{TokenMethod, `show`, scanner.Position{
					Filename: "", Offset: 6, Line: 1, Column: 7}},
				{TokenFieldAccessor, `.dedup`, scanner.Position{
					Filename: "", Offset: 11, Line: 1, Column: 12}},
				{TokenAssign, `=`, scanner.Position{
					Filename: "", Offset: 18, Line: 1, Column: 19}},
				{TokenIdentifier, `number`, scanner.Position{
					Filename: "", Offset: 20, Line: 1, Column: 21}},
			},
		},
		{
			src: `cd("/foo/bar")`,
			expect: []Token{
				{TokenCommand, `cd`, scanner.Position{
					Filename: "", Offset: 0, Line: 1, Column: 1}},
				{TokenLeftParen, `(`, scanner.Position{
					Filename: "", Offset: 2, Line: 1, Column: 3}},
				{TokenString, `/foo/bar`, scanner.Position{
					Filename: "", Offset: 3, Line: 1, Column: 4}},
				{TokenRightParen, `)`, scanner.Position{
					Filename: "", Offset: 13, Line: 1, Column: 14}},
			},
		},
		{
			src: `cd(/foo/bar)`,
			expect: []Token{
				{TokenCommand, `cd`, scanner.Position{
					Filename: "", Offset: 0, Line: 1, Column: 1}},
				{TokenLeftParen, `(`, scanner.Position{
					Filename: "", Offset: 2, Line: 1, Column: 3}},
				{TokenSlash, `/`, scanner.Position{
					Filename: "", Offset: 3, Line: 1, Column: 4}},
				{TokenIdentifier, `foo`, scanner.Position{
					Filename: "", Offset: 4, Line: 1, Column: 5}},
				{TokenSlash, `/`, scanner.Position{
					Filename: "", Offset: 7, Line: 1, Column: 8}},
				{TokenIdentifier, `bar`, scanner.Position{
					Filename: "", Offset: 8, Line: 1, Column: 9}},
				{TokenRightParen, `)`, scanner.Position{
					Filename: "", Offset: 11, Line: 1, Column: 12}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer()
			tokenizer.Reset(t.Context(), body)
			next, stop := iter.Pull2(tokenizer.Tokens())
			t.Cleanup(stop)

			got := testCollectTokens(t, next)
			must.Eq(t, tc.expect, got)
		})
	}
}

// testCollectTokens is a helper that consumes the entire tokenizer and returns
// a slice of tokens
func testCollectTokens(
	t *testing.T, next func() (Token, error, bool),
) []Token {
	t.Helper()
	got := []Token{}
DONE:
	for {
		token, err, ok := next()
		if !ok {
			break
		}
		switch {
		case err != nil:
			must.EqError(t, err, ErrEOF.Error())
			break DONE
		default:
			got = append(got, token)
		}
	}
	return got
}
