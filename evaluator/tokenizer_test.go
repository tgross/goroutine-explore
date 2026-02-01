package evaluator

import (
	"iter"
	"strings"
	"testing"
	"text/scanner"

	"github.com/shoenig/test/must"
)

func TestTokenizer_SmokeTest(t *testing.T) {

	src := `g.where(.duration > 10 and .state == "select")
              | union(g1) | as(g2) | show()`

	body := strings.NewReader(src)
	tokenizer := NewTokenizer(body)
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
	testCases := []struct {
		src    string
		expect []Token
	}{
		{
			src: `pragma.show.dedup = number`,
			expect: []Token{
				Token{TokenPragma, `pragma`, scanner.Position{
					Filename: "", Offset: 0, Line: 1, Column: 1}},
				Token{TokenMethod, `show`, scanner.Position{
					Filename: "", Offset: 6, Line: 1, Column: 7}},
				Token{TokenFieldAccessor, `.dedup`, scanner.Position{
					Filename: "", Offset: 11, Line: 1, Column: 12}},
				Token{TokenAssign, `=`, scanner.Position{
					Filename: "", Offset: 18, Line: 1, Column: 19}},
				Token{TokenIdentifier, `number`, scanner.Position{
					Filename: "", Offset: 20, Line: 1, Column: 21}},
			},
		},
		{
			src: `cd("/foo/bar")`,
			expect: []Token{
				Token{TokenCommand, `cd`, scanner.Position{
					Filename: "", Offset: 0, Line: 1, Column: 1}},
				Token{TokenLeftParen, `(`, scanner.Position{
					Filename: "", Offset: 2, Line: 1, Column: 3}},
				Token{TokenString, `/foo/bar`, scanner.Position{
					Filename: "", Offset: 3, Line: 1, Column: 4}},
				Token{TokenRightParen, `)`, scanner.Position{
					Filename: "", Offset: 13, Line: 1, Column: 14}},
			},
		},
		{
			src: `cd(/foo/bar)`,
			expect: []Token{
				Token{TokenCommand, `cd`, scanner.Position{
					Filename: "", Offset: 0, Line: 1, Column: 1}},
				Token{TokenLeftParen, `(`, scanner.Position{
					Filename: "", Offset: 2, Line: 1, Column: 3}},
				Token{TokenSlash, `/`, scanner.Position{
					Filename: "", Offset: 3, Line: 1, Column: 4}},
				Token{TokenIdentifier, `foo`, scanner.Position{
					Filename: "", Offset: 4, Line: 1, Column: 5}},
				Token{TokenSlash, `/`, scanner.Position{
					Filename: "", Offset: 7, Line: 1, Column: 8}},
				Token{TokenIdentifier, `bar`, scanner.Position{
					Filename: "", Offset: 8, Line: 1, Column: 9}},
				Token{TokenRightParen, `)`, scanner.Position{
					Filename: "", Offset: 11, Line: 1, Column: 12}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			body := strings.NewReader(tc.src)
			tokenizer := NewTokenizer(body)
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
