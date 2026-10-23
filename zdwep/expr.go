package zdwep

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/memmaker/voxelfps/wep"
)

// DECORATE arguments are expressions, not literals: a recoil amount is written
// `pitch - (0.8 * CallACS("FD_recoil")/65536.0)` and a jump condition is
// `user_fadetimer > 0`. A handful of regexes for the shapes one mod happens to
// use would be shorter to write and wrong on the next mod, so this is a small
// evaluator instead.
//
// Two kinds of value come out of it. Anything built only from numbers and the
// mod's own settings folds to a constant here and never reaches the engine. A
// value that depends on the running actor - a countdown it is keeping, how far
// it has faded - cannot be folded without changing what the weapon does, so it
// is compiled into a stack program and shipped.

// Range is the span an expression's value can take. Min and Max are equal
// unless the expression asked for a random one.
type Range struct {
	Min float64
	Max float64
}

func constant(v float64) Range { return Range{Min: v, Max: v} }

// IsConstant reports whether the range is a single number.
func (r Range) IsConstant() bool { return r.Min == r.Max }

// Value is the range's single number, or the middle of it.
func (r Range) Value() float64 { return (r.Min + r.Max) / 2 }

// True reports whether the range reads as true.
func (r Range) True() bool { return r.Value() != 0 }

// Value is what an expression evaluated to: a span, plus the program to work it
// out at run time when it cannot be settled here.
type Value struct {
	Range Range
	// Code is nil when the value is settled, which is the common case.
	Code wep.Expr
}

// Folded reports whether the value is known now.
func (v Value) Folded() bool { return len(v.Code) == 0 }

// Number is the settled value. Only meaningful when Folded.
func (v Value) Number() float64 { return v.Range.Value() }

// True reports how a settled value reads as a condition.
func (v Value) True() bool { return v.Range.True() }

func folded(r Range) Value { return Value{Range: r} }

// code builds an unsettled value out of its operands and an operator.
func code(result Range, op string, operands ...Value) Value {
	var program wep.Expr
	for _, operand := range operands {
		program = append(program, operand.program()...)
	}
	return Value{Range: result, Code: append(program, wep.Token{Kind: wep.TokenOp, Name: op})}
}

// program is the value as tokens, which for a settled value is a single push.
func (v Value) program() wep.Expr {
	if v.Folded() {
		return wep.Expr{{Kind: wep.TokenValue, Value: v.Range.Value()}}
	}
	return v.Code
}

// Arg is one argument to a call inside an expression. DECORATE passes names as
// quoted strings, as in CallACS("FD_recoil"), so an argument is either text or
// a value.
type Arg struct {
	Str   string
	Value Value
	IsStr bool
}

// Resolution is what an environment makes of a name: a number known now, or a
// value the engine reads off the running actor.
type Resolution struct {
	Const Range
	// Var, when set, names a value read while the game runs.
	Var string
}

// Env supplies the values an expression can name. A false result means the name
// is not something this migration knows, which is an error rather than a zero.
type Env func(name string, args []Arg) (Resolution, bool)

// EvalExpr works out what an expression comes to.
func EvalExpr(text string, env Env) (Value, error) {
	p := &exprParser{tokens: tokenizeExpr(text), env: env}
	value, err := p.parseBinary(0)
	if err != nil {
		return Value{}, err
	}
	if p.pos != len(p.tokens) {
		return Value{}, fmt.Errorf("expression %q: unexpected %q", text, p.tokens[p.pos])
	}
	return value, nil
}

func isHexDigit(r rune) bool {
	return unicode.IsDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func tokenizeExpr(text string) []string {
	var tokens []string
	runes := []rune(text)
	for i := 0; i < len(runes); {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '"':
			end := i + 1
			for end < len(runes) && runes[end] != '"' {
				end++
			}
			if end < len(runes) {
				end++
			}
			tokens = append(tokens, string(runes[i:end]))
			i = end
		case c == '0' && i+1 < len(runes) && (runes[i+1] == 'x' || runes[i+1] == 'X'):
			// DECORATE writes flag masks in hex - 0x7FFFFFFF for "as much as
			// an int holds" is the usual one. isHex covers what strconv's
			// base 0 parsing of a 0x literal accepts.
			end := i + 2
			for end < len(runes) && isHexDigit(runes[end]) {
				end++
			}
			tokens = append(tokens, string(runes[i:end]))
			i = end
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])):
			end := i
			for end < len(runes) && (unicode.IsDigit(runes[end]) || runes[end] == '.') {
				end++
			}
			tokens = append(tokens, string(runes[i:end]))
			i = end
		case unicode.IsLetter(c) || c == '_':
			end := i
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_' || runes[end] == '.') {
				end++
			}
			tokens = append(tokens, string(runes[i:end]))
			i = end
		default:
			// Two character operators first, so == does not read as two =.
			if i+1 < len(runes) {
				pair := string(runes[i : i+2])
				switch pair {
				case "==", "!=", "<=", ">=", "&&", "||", "::", "<<", ">>":
					tokens = append(tokens, pair)
					i += 2
					continue
				}
			}
			tokens = append(tokens, string(c))
			i++
		}
	}
	return tokens
}

type exprParser struct {
	tokens []string
	pos    int
	env    Env
}

// Binding powers, loosest first. Nothing here needs more than this.
var binaryPrecedence = map[string]int{
	"||": 1, "&&": 2,
	"==": 3, "!=": 3, "<": 3, "<=": 3, ">": 3, ">=": 3,
	"|": 3, "^": 3, "&": 3,
	"<<": 4, ">>": 4,
	"+": 4, "-": 4,
	"*": 5, "/": 5, "%": 5,
}

func (p *exprParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *exprParser) parseBinary(minPrecedence int) (Value, error) {
	left, err := p.parseUnary()
	if err != nil {
		return Value{}, err
	}
	for {
		op := p.peek()
		precedence, isBinary := binaryPrecedence[op]
		if !isBinary || precedence < minPrecedence {
			return left, nil
		}
		p.pos++
		right, err := p.parseBinary(precedence + 1)
		if err != nil {
			return Value{}, err
		}
		left, err = applyBinary(op, left, right)
		if err != nil {
			return Value{}, err
		}
	}
}

func applyBinary(op string, left, right Value) (Value, error) {
	span, err := rangeOf(op, left.Range, right.Range)
	if err != nil {
		return Value{}, err
	}
	if left.Folded() && right.Folded() {
		return folded(span), nil
	}
	return code(span, op, left, right), nil
}

// rangeOf works out the span of a result, which is what a lowering reads when
// it wants bounds rather than a number - a random sideways offset, say.
func rangeOf(op string, left, right Range) (Range, error) {
	a, b := left.Value(), right.Value()
	switch op {
	case "+":
		return Range{left.Min + right.Min, left.Max + right.Max}, nil
	case "-":
		return Range{left.Min - right.Max, left.Max - right.Min}, nil
	case "*":
		if right.IsConstant() {
			return scaled(left, right.Min), nil
		}
		if left.IsConstant() {
			return scaled(right, left.Min), nil
		}
		return constant(a * b), nil
	case "/":
		if right.IsConstant() {
			if right.Min == 0 {
				// A mod dividing by its own zero is the mod's business; ZDoom
				// would do it too. Zero is the answer, not a failed migration.
				return constant(0), nil
			}
			return scaled(left, 1/right.Min), nil
		}
		if b == 0 {
			return constant(0), nil
		}
		return constant(a / b), nil
	case "%":
		if int64(b) == 0 {
			return constant(0), nil
		}
		return constant(float64(int64(a) % int64(b))), nil
	case "==":
		return boolean(a == b), nil
	case "!=":
		return boolean(a != b), nil
	case "<":
		return boolean(a < b), nil
	case "<=":
		return boolean(a <= b), nil
	case ">":
		return boolean(a > b), nil
	case ">=":
		return boolean(a >= b), nil
	case "&&":
		return boolean(a != 0 && b != 0), nil
	case "||":
		return boolean(a != 0 || b != 0), nil
	case "|":
		return constant(float64(int64(a) | int64(b))), nil
	case "&":
		return constant(float64(int64(a) & int64(b))), nil
	case "^":
		return constant(float64(int64(a) ^ int64(b))), nil
	case "<<":
		return constant(float64(int64(a) << uint(int64(b)&63))), nil
	case ">>":
		return constant(float64(int64(a) >> uint(int64(b)&63))), nil
	}
	return Range{}, fmt.Errorf("unknown operator %q", op)
}

func scaled(r Range, factor float64) Range {
	low, high := r.Min*factor, r.Max*factor
	if low > high {
		low, high = high, low
	}
	return Range{low, high}
}

func boolean(b bool) Range {
	if b {
		return constant(1)
	}
	return constant(0)
}

func (p *exprParser) parseUnary() (Value, error) {
	switch p.peek() {
	case "-":
		p.pos++
		value, err := p.parseUnary()
		if err != nil {
			return Value{}, err
		}
		span := Range{-value.Range.Max, -value.Range.Min}
		if value.Folded() {
			return folded(span), nil
		}
		return code(span, "neg", value), nil
	case "+":
		p.pos++
		return p.parseUnary()
	case "!":
		p.pos++
		value, err := p.parseUnary()
		if err != nil {
			return Value{}, err
		}
		span := boolean(!value.Range.True())
		if value.Folded() {
			return folded(span), nil
		}
		return code(span, "!", value), nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (Value, error) {
	token := p.peek()
	if token == "" {
		return Value{}, fmt.Errorf("expression ended early")
	}
	if token == "(" {
		p.pos++
		value, err := p.parseBinary(0)
		if err != nil {
			return Value{}, err
		}
		if p.peek() != ")" {
			return Value{}, fmt.Errorf("missing closing bracket")
		}
		p.pos++
		return value, nil
	}
	if strings.HasPrefix(token, "0x") || strings.HasPrefix(token, "0X") {
		if value, err := strconv.ParseUint(token[2:], 16, 64); err == nil {
			p.pos++
			return folded(constant(float64(value))), nil
		}
	}
	if number, err := strconv.ParseFloat(token, 64); err == nil {
		p.pos++
		return folded(constant(number)), nil
	}
	if strings.HasPrefix(token, `"`) {
		p.pos++
		return Value{}, fmt.Errorf("string %s is not a number", token)
	}
	if !isName(token) {
		return Value{}, fmt.Errorf("unexpected %q", token)
	}
	name := token
	p.pos++
	// A qualified name such as FDPlayer::user_x reads as its last part.
	for p.peek() == "::" {
		p.pos++
		name = p.peek()
		p.pos++
	}
	// An indexed user array, user_life[0], is one variable as far as we are
	// concerned: the engine keeps them by name and the index is part of it.
	if p.peek() == "[" {
		p.pos++
		index := p.peek()
		p.pos++
		if p.peek() == "]" {
			p.pos++
		}
		name = name + "[" + index + "]"
	}

	if p.peek() != "(" {
		return p.resolve(name, nil)
	}

	p.pos++
	var args []Arg
	for p.peek() != ")" {
		if p.peek() == "" {
			return Value{}, fmt.Errorf("%s( ran off the end", name)
		}
		if strings.HasPrefix(p.peek(), `"`) {
			args = append(args, Arg{Str: strings.Trim(p.tokens[p.pos], `"`), IsStr: true})
			p.pos++
		} else {
			value, err := p.parseBinary(0)
			if err != nil {
				return Value{}, err
			}
			args = append(args, Arg{Value: value})
		}
		if p.peek() == "," {
			p.pos++
		}
	}
	p.pos++
	return p.resolve(name, args)
}

// resolve turns a name into a value, through the builtins and then the
// environment.
func (p *exprParser) resolve(name string, args []Arg) (Value, error) {
	if value, ok := builtinExpr(name, args); ok {
		return value, nil
	}
	if p.env != nil {
		if resolution, ok := p.env(name, args); ok {
			if resolution.Var != "" {
				return Value{
					Range: resolution.Const,
					Code:  wep.Expr{{Kind: wep.TokenVar, Name: resolution.Var}},
				}, nil
			}
			return folded(resolution.Const), nil
		}
	}
	if len(args) > 0 {
		return Value{}, fmt.Errorf("%s() is not a call this migration knows", name)
	}
	return Value{}, fmt.Errorf("%q has no value in this migration", name)
}

func builtinExpr(name string, args []Arg) (Value, bool) {
	switch strings.ToLower(name) {
	case "random", "frandom":
		if len(args) == 2 && !args[0].IsStr && !args[1].IsStr {
			low, high := args[0].Value.Number(), args[1].Value.Number()
			if low > high {
				low, high = high, low
			}
			// The roll happens while the game runs, so the span travels with a
			// program that draws from it rather than a number picked here.
			if low == high {
				return folded(constant(low)), true
			}
			return Value{Range: Range{low, high}, Code: wep.Expr{
				{Kind: wep.TokenRandom, Value: low, Value2: high},
			}}, true
		}
	case "randompick", "frandompick":
		// One of the listed values, so the span covers them all.
		if len(args) == 0 {
			return Value{}, false
		}
		low, high := 0.0, 0.0
		for i, arg := range args {
			if arg.IsStr {
				return Value{}, false
			}
			value := arg.Value.Number()
			if i == 0 || value < low {
				low = value
			}
			if i == 0 || value > high {
				high = value
			}
		}
		if low == high {
			return folded(constant(low)), true
		}
		return Value{Range: Range{low, high}, Code: wep.Expr{
			{Kind: wep.TokenRandom, Value: low, Value2: high},
		}}, true
	case "fixed2float":
		// ACS hands floats back in sixteen bit fixed point; this undoes that.
		if len(args) == 1 && !args[0].IsStr {
			value, err := applyBinary("/", args[0].Value, folded(constant(65536)))
			if err != nil {
				return Value{}, false
			}
			return value, true
		}
	case "abs", "sin", "cos", "tan", "sqrt", "atan", "asin", "acos":
		if len(args) == 1 && !args[0].IsStr {
			return mathCall(strings.ToLower(name), args[0].Value), true
		}
	case "true":
		return folded(constant(1)), true
	case "false":
		return folded(constant(0)), true
	case "pitch", "angle", "roll":
		// Read relative to where the actor is already pointing, so what a
		// script adds to it is the whole of what it means.
		return folded(constant(0)), true
	}
	return Value{}, false
}

// mathCall applies one of DECORATE's maths functions. A settled argument gives
// a settled answer; one that varies travels with the weapon, and the span falls
// back to what the function can possibly return.
func mathCall(name string, arg Value) Value {
	apply := func(v float64) float64 {
		switch name {
		case "abs":
			return math.Abs(v)
		case "sin":
			return math.Sin(v * math.Pi / 180)
		case "cos":
			return math.Cos(v * math.Pi / 180)
		case "tan":
			return math.Tan(v * math.Pi / 180)
		case "sqrt":
			if v < 0 {
				return 0
			}
			return math.Sqrt(v)
		case "atan":
			return math.Atan(v) * 180 / math.Pi
		case "asin":
			return math.Asin(clampUnit(v)) * 180 / math.Pi
		case "acos":
			return math.Acos(clampUnit(v)) * 180 / math.Pi
		}
		return v
	}
	if arg.Folded() {
		return folded(constant(apply(arg.Number())))
	}
	span := Range{apply(arg.Range.Min), apply(arg.Range.Max)}
	if span.Min > span.Max {
		span.Min, span.Max = span.Max, span.Min
	}
	switch name {
	case "sin", "cos":
		span = Range{-1, 1} // the argument varies, so the answer covers the circle
	}
	return code(span, name, arg)
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

func isName(token string) bool {
	if token == "" {
		return false
	}
	c := rune(token[0])
	return unicode.IsLetter(c) || c == '_'
}
