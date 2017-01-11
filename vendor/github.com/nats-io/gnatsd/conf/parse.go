// Copyright 2013-2016 Apcera Inc. All rights reserved.

// Package conf supports a configuration file format used by gnatsd. It is
// a flexible format that combines the best of traditional
// configuration formats and newer styles such as JSON and YAML.
package conf

// The format supported is less restrictive than today's formats.
// Supports mixed Arrays [], nested Maps {}, multiple comment types (# and //)
// Also supports key value assigments using '=' or ':' or whiteSpace()
//   e.g. foo = 2, foo : 2, foo 2
// maps can be assigned with no key separator as well
// semicolons as value terminators in key/value assignments are optional
//
// see parse_test.go for more examples.

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type parser struct {
	mapping map[string]interface{}
	lx      *lexer

	// The current scoped context, can be array or map
	ctx interface{}

	// stack of contexts, either map or array/slice stack
	ctxs []interface{}

	// Keys stack
	keys []string

	// The config file path, empty by default.
	fp string
}

// Parse will return a map of keys to interface{}, although concrete types
// underly them. The values supported are string, bool, int64, float64, DateTime.
// Arrays and nested Maps are also supported.
func Parse(data string) (map[string]interface{}, error) {
	p, err := parse(data, "")
	if err != nil {
		return nil, err
	}
	return p.mapping, nil
}

// ParseFile is a helper to open file, etc. and parse the contents.
func ParseFile(fp string) (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(fp)
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %v", err)
	}

	p, err := parse(string(data), filepath.Dir(fp))
	if err != nil {
		return nil, err
	}
	return p.mapping, nil
}

func parse(data, fp string) (p *parser, err error) {
	p = &parser{
		mapping: make(map[string]interface{}),
		lx:      lex(data),
		ctxs:    make([]interface{}, 0, 4),
		keys:    make([]string, 0, 4),
		fp:      fp,
	}
	p.pushContext(p.mapping)

	for {
		it := p.next()
		if it.typ == itemEOF {
			break
		}
		if err := p.processItem(it); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func (p *parser) next() item {
	return p.lx.nextItem()
}

func (p *parser) pushContext(ctx interface{}) {
	p.ctxs = append(p.ctxs, ctx)
	p.ctx = ctx
}

func (p *parser) popContext() interface{} {
	if len(p.ctxs) == 0 {
		panic("BUG in parser, context stack empty")
	}
	li := len(p.ctxs) - 1
	last := p.ctxs[li]
	p.ctxs = p.ctxs[0:li]
	p.ctx = p.ctxs[len(p.ctxs)-1]
	return last
}

func (p *parser) pushKey(key string) {
	p.keys = append(p.keys, key)
}

func (p *parser) popKey() string {
	if len(p.keys) == 0 {
		panic("BUG in parser, keys stack empty")
	}
	li := len(p.keys) - 1
	last := p.keys[li]
	p.keys = p.keys[0:li]
	return last
}

func (p *parser) processItem(it item) error {
	switch it.typ {
	case itemError:
		return fmt.Errorf("Parse error on line %d: '%s'", it.line, it.val)
	case itemKey:
		p.pushKey(it.val)
	case itemMapStart:
		newCtx := make(map[string]interface{})
		p.pushContext(newCtx)
	case itemMapEnd:
		p.setValue(p.popContext())
	case itemString:
		p.setValue(it.val) // FIXME(dlc) sanitize string?
	case itemInteger:
		lastDigit := 0
		for _, r := range it.val {
			if !unicode.IsDigit(r) {
				break
			}
			lastDigit++
		}
		numStr := it.val[:lastDigit]
		num, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			if e, ok := err.(*strconv.NumError); ok &&
				e.Err == strconv.ErrRange {
				return fmt.Errorf("Integer '%s' is out of the range.", it.val)
			}
			return fmt.Errorf("Expected integer, but got '%s'.", it.val)
		}
		// Process a suffix
		suffix := strings.ToLower(strings.TrimSpace(it.val[lastDigit:]))
		switch suffix {
		case "":
			p.setValue(num)
		case "k":
			p.setValue(num * 1000)
		case "kb":
			p.setValue(num * 1024)
		case "m":
			p.setValue(num * 1000 * 1000)
		case "mb":
			p.setValue(num * 1024 * 1024)
		case "g":
			p.setValue(num * 1000 * 1000 * 1000)
		case "gb":
			p.setValue(num * 1024 * 1024 * 1024)
		}
	case itemFloat:
		num, err := strconv.ParseFloat(it.val, 64)
		if err != nil {
			if e, ok := err.(*strconv.NumError); ok &&
				e.Err == strconv.ErrRange {
				return fmt.Errorf("Float '%s' is out of the range.", it.val)
			}
			return fmt.Errorf("Expected float, but got '%s'.", it.val)
		}
		p.setValue(num)
	case itemBool:
		switch strings.ToLower(it.val) {
		case "true", "yes", "on":
			p.setValue(true)
		case "false", "no", "off":
			p.setValue(false)
		default:
			return fmt.Errorf("Expected boolean value, but got '%s'.", it.val)
		}
	case itemDatetime:
		dt, err := time.Parse("2006-01-02T15:04:05Z", it.val)
		if err != nil {
			return fmt.Errorf(
				"Expected Zulu formatted DateTime, but got '%s'.", it.val)
		}
		p.setValue(dt)
	case itemArrayStart:
		var array = make([]interface{}, 0)
		p.pushContext(array)
	case itemArrayEnd:
		array := p.ctx
		p.popContext()
		p.setValue(array)
	case itemVariable:
		if value, ok := p.lookupVariable(it.val); ok {
			p.setValue(value)
		} else {
			return fmt.Errorf("Variable reference for '%s' on line %d can not be found.",
				it.val, it.line)
		}
	case itemInclude:
		m, err := ParseFile(filepath.Join(p.fp, it.val))
		if err != nil {
			return fmt.Errorf("Error parsing include file '%s', %v.", it.val, err)
		}
		for k, v := range m {
			p.pushKey(k)
			p.setValue(v)
		}
	}

	return nil
}

// Used to map an environment value into a temporary map to pass to secondary Parse call.
const pkey = "pk"

// We special case raw strings here that are bcrypt'd. This allows us not to force quoting the strings
const bcryptPrefix = "2a$"

// lookupVariable will lookup a variable reference. It will use block scoping on keys
// it has seen before, with the top level scoping being the environment variables. We
// ignore array contexts and only process the map contexts..
//
// Returns true for ok if it finds something, similar to map.
func (p *parser) lookupVariable(varReference string) (interface{}, bool) {
	// Do special check to see if it is a raw bcrypt string.
	if strings.HasPrefix(varReference, bcryptPrefix) {
		return "$" + varReference, true
	}

	// Loop through contexts currently on the stack.
	for i := len(p.ctxs) - 1; i >= 0; i -= 1 {
		ctx := p.ctxs[i]
		// Process if it is a map context
		if m, ok := ctx.(map[string]interface{}); ok {
			if v, ok := m[varReference]; ok {
				return v, ok
			}
		}
	}

	// If we are here, we have exhausted our context maps and still not found anything.
	// Parse from the environment.
	if vStr, ok := os.LookupEnv(varReference); ok {
		// Everything we get here will be a string value, so we need to process as a parser would.
		if vmap, err := Parse(fmt.Sprintf("%s=%s", pkey, vStr)); err == nil {
			v, ok := vmap[pkey]
			return v, ok
		}
	}
	return nil, false
}

func (p *parser) setValue(val interface{}) {
	// Test to see if we are on an array or a map

	// Array processing
	if ctx, ok := p.ctx.([]interface{}); ok {
		p.ctx = append(ctx, val)
		p.ctxs[len(p.ctxs)-1] = p.ctx
	}

	// Map processing
	if ctx, ok := p.ctx.(map[string]interface{}); ok {
		key := p.popKey()
		// FIXME(dlc), make sure to error if redefining same key?
		ctx[key] = val
	}
}
