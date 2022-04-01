package dotenv

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	charComment       = '#'
	prefixSingleQuote = '\''
	prefixDoubleQuote = '"'

	exportPrefix = "export"
)

func parseBytes(src []byte, out map[string]string, lookupFn LookupFn) error {
	cutset := src
	for {
		cutset = getStatementStart(cutset)
		if cutset == nil {
			// reached end of file
			break
		}

		key, left, inherited, err := locateKeyName(cutset)
		if err != nil {
			return err
		}
		if strings.Contains(key, " ") {
			return errors.New("key cannot contain a space")
		}

		if inherited {
			if lookupFn == nil {
				lookupFn = noLookupFn
			}

			value, ok := lookupFn(key)
			if ok {
				out[key] = value
			}
			cutset = left
			continue
		}

		value, left, err := extractVarValue(left, out, lookupFn)
		if err != nil {
			return err
		}

		out[key] = value
		cutset = left
	}

	return nil
}

// getStatementPosition returns position of statement begin.
//
// It skips any comment line or non-whitespace character.
func getStatementStart(src []byte) []byte {
	pos := indexOfNonSpaceChar(src)
	if pos == -1 {
		return nil
	}

	src = src[pos:]
	if src[0] != charComment {
		return src
	}

	// skip comment section
	pos = bytes.IndexFunc(src, isCharFunc('\n'))
	if pos == -1 {
		return nil
	}

	return getStatementStart(src[pos:])
}

// locateKeyName locates and parses key name and returns rest of slice
func locateKeyName(src []byte) (key string, cutset []byte, inherited bool, err error) {
	// trim "export" and space at beginning
	src = bytes.TrimLeftFunc(bytes.TrimPrefix(src, []byte(exportPrefix)), isSpace)

	// locate key name end and validate it in single loop
	offset := 0
loop:
	for i, char := range src {
		rchar := rune(char)
		if isSpace(rchar) {
			continue
		}

		switch char {
		case '=', ':', '\n':
			// library also supports yaml-style value declaration
			key = string(src[0:i])
			offset = i + 1
			inherited = char == '\n'
			break loop
		case '_':
		default:
			// variable name should match [A-Za-z0-9_]
			if unicode.IsLetter(rchar) || unicode.IsNumber(rchar) {
				continue
			}

			return "", nil, inherited, fmt.Errorf(
				`unexpected character %q in variable name near %q`,
				string(char), string(src))
		}
	}

	if len(src) == 0 {
		return "", nil, inherited, errors.New("zero length string")
	}

	// trim whitespace
	key = strings.TrimRightFunc(key, unicode.IsSpace)
	cutset = bytes.TrimLeftFunc(src[offset:], isSpace)
	return key, cutset, inherited, nil
}

// extractVarValue extracts variable value and returns rest of slice
func extractVarValue(src []byte, envMap map[string]string, lookupFn LookupFn) (value string, rest []byte, err error) {
	quote, isQuoted := hasQuotePrefix(src)
	if !isQuoted {
		// unquoted value - read until new line
		end := bytes.IndexFunc(src, isNewLine)
		var rest []byte
		if end < 0 {
			value := strings.Split(string(src), "#")[0] // Remove inline comments on unquoted lines
			value = strings.TrimRightFunc(value, unicode.IsSpace)
			return expandVariables(value, envMap, lookupFn), nil, nil
		}

		value := strings.Split(string(src[0:end]), "#")[0]
		value = strings.TrimRightFunc(value, unicode.IsSpace)
		rest = src[end:]
		return expandVariables(value, envMap, lookupFn), rest, nil
	}

	// lookup quoted string terminator
	for i := 1; i < len(src); i++ {
		if char := src[i]; char != quote {
			continue
		}

		// skip escaped quote symbol (\" or \', depends on quote)
		if prevChar := src[i-1]; prevChar == '\\' {
			continue
		}

		// trim quotes
		trimFunc := isCharFunc(rune(quote))
		value = string(bytes.TrimLeftFunc(bytes.TrimRightFunc(src[0:i], trimFunc), trimFunc))
		if quote == prefixDoubleQuote {
			// unescape newlines for double quote (this is compat feature)
			// and expand environment variables
			value = expandVariables(expandEscapes(value), envMap, lookupFn)
		}

		return value, src[i+1:], nil
	}

	// return formatted error if quoted string is not terminated
	valEndIndex := bytes.IndexFunc(src, isCharFunc('\n'))
	if valEndIndex == -1 {
		valEndIndex = len(src)
	}

	return "", nil, fmt.Errorf("unterminated quoted value %s", src[:valEndIndex])
}

func expandEscapes(str string) string {
	out := escapeRegex.ReplaceAllStringFunc(str, func(match string) string {
		c := strings.TrimPrefix(match, `\`)
		switch c {
		case "n":
			return "\n"
		case "r":
			return "\r"
		default:
			return match
		}
	})
	return unescapeCharsRegex.ReplaceAllString(out, "$1")
}

func indexOfNonSpaceChar(src []byte) int {
	return bytes.IndexFunc(src, func(r rune) bool {
		return !unicode.IsSpace(r)
	})
}

// hasQuotePrefix reports whether charset starts with single or double quote and returns quote character
func hasQuotePrefix(src []byte) (quote byte, isQuoted bool) {
	if len(src) == 0 {
		return 0, false
	}

	switch prefix := src[0]; prefix {
	case prefixDoubleQuote, prefixSingleQuote:
		return prefix, true
	default:
		return 0, false
	}
}

func isCharFunc(char rune) func(rune) bool {
	return func(v rune) bool {
		return v == char
	}
}

// isSpace reports whether the rune is a space character but not line break character
//
// this differs from unicode.IsSpace, which also applies line break as space
func isSpace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', '\r', ' ', 0x85, 0xA0:
		return true
	}
	return false
}

// isNewLine reports whether the rune is a new line character
func isNewLine(r rune) bool {
	return r == '\n'
}
