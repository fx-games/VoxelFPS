package wep

import (
	"math"
	"math/rand"
)

// Some of what a script branches on is only knowable while it runs: an actor's
// own countdown, how translucent it has faded, how fast it is going. Those
// conditions cannot be folded away at migration time without changing what the
// weapon does, so they are compiled into a small stack program and shipped.
//
// Anything that *can* be folded still is - a mod's settings, arithmetic on
// constants - so a shipped expression is only ever the part that genuinely
// varies.

// TokenKind says what one step of an expression does.
type TokenKind string

const (
	// TokenValue pushes a number.
	TokenValue TokenKind = "value"
	// TokenVar pushes a named value read from the running actor.
	TokenVar TokenKind = "var"
	// TokenOp applies an operator to the top of the stack.
	TokenOp TokenKind = "op"
	// TokenRandom pushes a number between Value and Value2.
	TokenRandom TokenKind = "random"
)

// Token is one step of a compiled expression, in postfix order.
type Token struct {
	Kind TokenKind `json:"k"`
	// Value is the number for TokenValue, and the low end for TokenRandom.
	Value float64 `json:"v,omitempty"`
	// Value2 is the high end for TokenRandom.
	Value2 float64 `json:"v2,omitempty"`
	// Name is the variable for TokenVar and the operator for TokenOp.
	Name string `json:"n,omitempty"`
}

// Expr is a compiled expression: postfix tokens, or nothing when the value was
// constant and folded away.
type Expr []Token

// Vars is what a running expression can read: an actor's user variables and the
// few pieces of its own state that scripts branch on.
type Vars interface {
	// Var reports a named value, and whether it is one this actor has. A name
	// it does not know reads as zero, which is what ZDoom does with an unset
	// user variable.
	Var(name string) (float64, bool)
}

// Eval runs the expression. A nil Vars reads every variable as zero.
func (e Expr) Eval(vars Vars, random *rand.Rand) float64 {
	if len(e) == 0 {
		return 0
	}
	stack := make([]float64, 0, 8)
	push := func(v float64) { stack = append(stack, v) }
	pop := func() float64 {
		if len(stack) == 0 {
			return 0
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}

	for _, token := range e {
		switch token.Kind {
		case TokenValue:
			push(token.Value)
		case TokenRandom:
			push(rollBetween(random, token.Value, token.Value2))
		case TokenVar:
			value := 0.0
			if vars != nil {
				value, _ = vars.Var(token.Name)
			}
			push(value)
		case TokenOp:
			if isUnary(token.Name) {
				push(applyUnary(token.Name, pop()))
				continue
			}
			right := pop()
			push(applyOp(token.Name, pop(), right))
		}
	}
	return pop()
}

// True reports whether the expression reads as true, the way a jump condition
// does.
func (e Expr) True(vars Vars, random *rand.Rand) bool {
	return e.Eval(vars, random) != 0
}

// unaryOps are the operators that take one value off the stack.
var unaryOps = map[string]bool{
	"neg": true, "!": true, "abs": true, "sin": true, "cos": true,
	"tan": true, "sqrt": true, "atan": true, "asin": true, "acos": true,
}

func isUnary(op string) bool { return unaryOps[op] }

func applyUnary(op string, value float64) float64 {
	switch op {
	case "neg":
		return -value
	case "!":
		return boolValue(value == 0)
	case "abs":
		return math.Abs(value)
	case "sin":
		return math.Sin(value * math.Pi / 180)
	case "cos":
		return math.Cos(value * math.Pi / 180)
	case "tan":
		return math.Tan(value * math.Pi / 180)
	case "sqrt":
		if value < 0 {
			return 0
		}
		return math.Sqrt(value)
	case "atan":
		return math.Atan(value) * 180 / math.Pi
	case "asin":
		return math.Asin(clampUnit(value)) * 180 / math.Pi
	case "acos":
		return math.Acos(clampUnit(value)) * 180 / math.Pi
	}
	return value
}

func clampUnit(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

func applyOp(op string, left, right float64) float64 {
	switch op {
	case "+":
		return left + right
	case "-":
		return left - right
	case "*":
		return left * right
	case "/":
		if right == 0 {
			return 0
		}
		return left / right
	case "%":
		if int64(right) == 0 {
			return 0
		}
		return float64(int64(left) % int64(right))
	case "==":
		return boolValue(left == right)
	case "!=":
		return boolValue(left != right)
	case "<":
		return boolValue(left < right)
	case "<=":
		return boolValue(left <= right)
	case ">":
		return boolValue(left > right)
	case ">=":
		return boolValue(left >= right)
	case "&&":
		return boolValue(left != 0 && right != 0)
	case "||":
		return boolValue(left != 0 || right != 0)
	}
	return 0
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func rollBetween(random *rand.Rand, low, high float64) float64 {
	if high <= low {
		return low
	}
	if random == nil {
		return low
	}
	return low + random.Float64()*(high-low)
}
