// Package expression provides the shared, data-only Expr runtime for rules and mappings.
package expression

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

func Compile(source string, environment any, predicate bool) (*vm.Program, error) {
	if len(source) > 16384 {
		return nil, fmt.Errorf("expression exceeds 16384 bytes")
	}
	options := []expr.Option{expr.Env(environment), expr.MaxNodes(2048), expr.DisableBuiltin("now"), expr.Optimize(false)}
	if predicate {
		options = append(options, expr.AsBool(), expr.WarnOnAny())
	}
	return expr.Compile(source, options...)
}

func Run(program *vm.Program, environment any) (any, error) {
	runtime := vm.VM{MemoryBudget: 100000}
	return runtime.Run(program, environment)
}

func Test(program *vm.Program, environment any) (bool, error) {
	value, err := Run(program, environment)
	if err != nil {
		return false, err
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("predicate returned %T, expected bool", value)
	}
	return result, nil
}
