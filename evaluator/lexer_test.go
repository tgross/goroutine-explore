package evaluator

import (
	"iter"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

func TestTokenizer_SmokeTest(t *testing.T) {

	src := `g where (.duration > 10 and .state == "select")
              | union g1 | as g2 | show`

	body := strings.NewReader(src)
	tokenizer := NewTokenizer(body)
	next, stop := iter.Pull2(tokenizer.Tokens())
	t.Cleanup(stop)

	expectedTypes := []TokenType{
		TokenIdentifer,     // g
		TokenKeywordWhere,  // where
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
		TokenIdentifer,     // g1
		TokenPipe,          // |
		TokenFunction,      // as
		TokenIdentifer,     // g2
		TokenPipe,          // |
		TokenFunction,      // show
	}

	gotTypes := []TokenType{}

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
			t.Logf("[%02x] %s\n", token.Type, token)
			gotTypes = append(gotTypes, token.Type)
		}
	}

	must.Eq(t, expectedTypes, gotTypes)
}
