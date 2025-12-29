package evaluator

import (
	"strings"
)

func Evaluate(compiler *Compiler, src string, env map[string]Value) (Value, error) {
	body := strings.NewReader(src)
	tokenizer := NewTokenizer(body)
	chunk, err := compiler.Compile(tokenizer)
	if err != nil {
		return NoValue, err
	}

	vm := NewVM()
	vm.env = env
	vm.reset(chunk)
	val, err := vm.run()
	// vm.debug()

	return val, err
}
