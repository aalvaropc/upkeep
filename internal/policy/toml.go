package policy

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type valueKind int

const (
	stringValue valueKind = iota
	intValue
	boolValue
	listValue
)

func (k valueKind) String() string {
	switch k {
	case stringValue:
		return "a string"
	case intValue:
		return "an integer"
	case boolValue:
		return "a boolean"
	default:
		return "an array of strings"
	}
}

type value struct {
	line int
	kind valueKind
	str  string
	num  int64
	flag bool
	list []string
}

type syntaxError struct {
	line int
	msg  string
}

func (e *syntaxError) Error() string { return fmt.Sprintf("line %d: %s", e.line, e.msg) }

func errorf(line int, format string, args ...any) error {
	return &syntaxError{line: line, msg: fmt.Sprintf(format, args...)}
}

func parseTOML(r io.Reader) (map[string]value, error) {
	values := make(map[string]value)
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if strings.HasPrefix(text, "[") {
			return nil, errorf(line, "tables are not supported; write every key at the top level")
		}
		key, rest, ok := strings.Cut(text, "=")
		if !ok {
			return nil, errorf(line, "expected key = value")
		}
		key = strings.TrimSpace(key)
		if !isBareKey(key) {
			return nil, errorf(line, "invalid key %q", key)
		}
		if prev, dup := values[key]; dup {
			return nil, errorf(line, "key %q is already set on line %d", key, prev.line)
		}
		v, rest, err := parseValue(strings.TrimSpace(rest), line)
		if err != nil {
			return nil, err
		}
		if rest = strings.TrimSpace(rest); rest != "" && !strings.HasPrefix(rest, "#") {
			return nil, errorf(line, "unexpected %q after the value", rest)
		}
		v.line = line
		values[key] = v
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func isBareKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func parseValue(s string, line int) (value, string, error) {
	switch {
	case s == "":
		return value{}, "", errorf(line, "missing value")
	case s[0] == '"':
		str, rest, err := parseString(s, line)
		return value{kind: stringValue, str: str}, rest, err
	case s[0] == '[':
		list, rest, err := parseList(s, line)
		return value{kind: listValue, list: list}, rest, err
	case strings.HasPrefix(s, "true"):
		return value{kind: boolValue, flag: true}, s[len("true"):], nil
	case strings.HasPrefix(s, "false"):
		return value{kind: boolValue, flag: false}, s[len("false"):], nil
	case s[0] == '+' || s[0] == '-' || s[0] >= '0' && s[0] <= '9':
		return parseInt(s, line)
	case s[0] == '\'':
		return value{}, "", errorf(line, "literal strings are not supported; use double quotes")
	default:
		return value{}, "", errorf(line, "unsupported value %q", s)
	}
}

func parseString(s string, line int) (string, string, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			return b.String(), s[i+1:], nil
		case '\\':
			i++
			if i == len(s) {
				return "", "", errorf(line, "unterminated string")
			}
			switch s[i] {
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				return "", "", errorf(line, "unsupported escape \\%c", s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", "", errorf(line, "unterminated string")
}

func parseList(s string, line int) ([]string, string, error) {
	list := []string{}
	rest := s[1:]
	for {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return nil, "", errorf(line, "unterminated array; arrays must fit on one line")
		}
		if rest[0] == ']' {
			return list, rest[1:], nil
		}
		if rest[0] != '"' {
			return nil, "", errorf(line, "arrays may only contain strings")
		}
		item, after, err := parseString(rest, line)
		if err != nil {
			return nil, "", err
		}
		list = append(list, item)
		after = strings.TrimSpace(after)
		switch {
		case strings.HasPrefix(after, ","):
			rest = after[1:]
		case strings.HasPrefix(after, "]"):
			rest = after
		default:
			return nil, "", errorf(line, "expected , or ] in the array")
		}
	}
}

func parseInt(s string, line int) (value, string, error) {
	end := 0
	for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '_' || end == 0 && (s[end] == '+' || s[end] == '-')) {
		end++
	}
	raw := s[:end]
	digits := strings.TrimLeft(raw, "+-")
	if digits == "" || strings.HasPrefix(digits, "_") || strings.HasSuffix(digits, "_") || strings.Contains(digits, "__") {
		return value{}, "", errorf(line, "invalid integer %q", raw)
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(raw, "_", ""), 10, 64)
	if err != nil {
		return value{}, "", errorf(line, "invalid integer %q", raw)
	}
	return value{kind: intValue, num: n}, s[end:], nil
}
