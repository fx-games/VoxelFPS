package zdwep

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// DECORATE arguments are expressions, not literals: a recoil amount is written
// `pitch - (0.8 * CallACS("FD_recoil")/65536.0)` and a jump condition is
// `CallACS("FD_gunsmoke") == 0`. A handful of regexes for the shapes one mod
// happens to use would be shorter to write and wrong on the next mod, so this
// is a small evaluator instead: numbers, the usual operators, and the few named
// things DECORATE expressions reach for.
//
// Values carry a range rather than a single number, because a script can ask
// for `frandom(-7.6, -7.2)` and we want to keep the spread instead of picking
// for it at migration time. A constant is a range of zero width.

// Range is an expression's value: Min and Max are equal unless the expression
// asked for a random one.
type Range struct {
	Min float64
	Max float64
}

func constant(v float64) Range { return Range{Min: v, Max: v} }

// IsConstant reports whether the expression evaluated to a single number.
func (r Range) IsConstant() bool { return r.Min == r.Max }

// Value is the range's single number, or the middle of it. Use it where a
// range makes no sense, such as a condition.
func (r Range) Value() float64 { return (r.Min + r.Max) / 2 }

// True reports whether the expression reads as true, the way DECORATE's jump
// conditions do.
func (r Range) True() bool { return r.Value() != 0 }

// Arg is one argument to a call inside an expression. DECORATE passes names as
// quoted strings, as in CallACS("FD_recoil"), so an argument is either text or
// a value.
type Arg struct {
	Str   string
	Value Range
	IsStr bool
}

// Env supplies the values an expression can name. A false result means the name
// is not something this migration knows, which is an error rather than a zero.
type Env func(name string, args []Arg) (Range, bool)

// EvalExpr evaluates a DECORATE expression.
func EvalExpr(text string, env Env) (Range, error) {
	p := &exprParser{tokens: tokenizeExpr(text), env: env}
	value, err := p.parseBinary(0)
	if err != nil {
		return Range{}, err
	}
	if p.pos != len(p.tokens) {
		return Range{}, fmt.Errorf("expression %q: unexpected %q", text, p.tokens[p.pos])
	}
	return value, nil
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
				case "==", "!=", "<=", ">=", "&&", "||":
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
	"+": 4, "-": 4,
	"*": 5, "/": 5, "%": 5,
}

func (p *exprParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *exprParser) parseBinary(minPrecedence int) (Range, error) {
	left, err := p.parseUnary()
	if err != nil {
		return Range{}, err
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
			return Range{}, err
		}
		left, err = applyBinary(op, left, right)
		if err != nil {
			return Range{}, err
		}
	}
}

func applyBinary(op string, left, right Range) (Range, error) {
	// Ranges only survive addition and scaling, which is all a spread ever goes
	// through. Anything else collapses to the middle, which is what a condition
	// wants anyway.
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
		if b == 0 {
			return Range{}, fmt.Errorf("division by zero")
		}
		if right.IsConstant() {
			return scaled(left, 1/right.Min), nil
		}
		return constant(a / b), nil
	case "%":
		if b == 0 {
			return Range{}, fmt.Errorf("division by zero")
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

func (p *exprParser) parseUnary() (Range, error) {
	switch p.peek() {
	case "-":
		p.pos++
		value, err := p.parseUnary()
		if err != nil {
			return Range{}, err
		}
		return Range{-value.Max, -value.Min}, nil
	case "+":
		p.pos++
		return p.parseUnary()
	case "!":
		p.pos++
		value, err := p.parseUnary()
		if err != nil {
			return Range{}, err
		}
		return boolean(!value.True()), nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (Range, error) {
	token := p.peek()
	if token == "" {
		return Range{}, fmt.Errorf("expression ended early")
	}
	if token == "(" {
		p.pos++
		value, err := p.parseBinary(0)
		if err != nil {
			return Range{}, err
		}
		if p.peek() != ")" {
			return Range{}, fmt.Errorf("missing closing bracket")
		}
		p.pos++
		return value, nil
	}
	if number, err := strconv.ParseFloat(token, 64); err == nil {
		p.pos++
		return constant(number), nil
	}
	if strings.HasPrefix(token, `"`) {
		p.pos++
		return Range{}, fmt.Errorf("string %s is not a number", token)
	}
	if !isName(token) {
		return Range{}, fmt.Errorf("unexpected %q", token)
	}
	name := token
	p.pos++

	if p.peek() != "(" {
		value, ok := p.lookup(name, nil)
		if !ok {
			return Range{}, fmt.Errorf("%q has no value in this migration", name)
		}
		return value, nil
	}

	p.pos++
	var args []Arg
	for p.peek() != ")" {
		if p.peek() == "" {
			return Range{}, fmt.Errorf("%s( ran off the end", name)
		}
		if strings.HasPrefix(p.peek(), `"`) {
			args = append(args, Arg{Str: strings.Trim(p.tokens[p.pos], `"`), IsStr: true})
			p.pos++
		} else {
			value, err := p.parseBinary(0)
			if err != nil {
				return Range{}, err
			}
			args = append(args, Arg{Value: value})
		}
		if p.peek() == "," {
			p.pos++
		}
	}
	p.pos++

	value, ok := p.lookup(name, args)
	if !ok {
		return Range{}, fmt.Errorf("%s() is not a call this migration knows", name)
	}
	return value, nil
}

func (p *exprParser) lookup(name string, args []Arg) (Range, bool) {
	if value, ok := builtinExpr(name, args); ok {
		return value, true
	}
	if p.env == nil {
		return Range{}, false
	}
	return p.env(name, args)
}

func builtinExpr(name string, args []Arg) (Range, bool) {
	switch strings.ToLower(name) {
	case "random", "frandom":
		if len(args) == 2 && !args[0].IsStr && !args[1].IsStr {
			low, high := args[0].Value.Value(), args[1].Value.Value()
			if low > high {
				low, high = high, low
			}
			return Range{low, high}, true
		}
	case "true":
		return constant(1), true
	case "false":
		return constant(0), true
	case "pitch", "angle", "roll":
		// Read relative to where the player is already looking, so the value
		// the script adds to it is the whole of what we want.
		return constant(0), true
	}
	return Range{}, false
}

func isName(token string) bool {
	if token == "" {
		return false
	}
	c := rune(token[0])
	return unicode.IsLetter(c) || c == '_'
}
