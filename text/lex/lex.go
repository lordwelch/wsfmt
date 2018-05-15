// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lex

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Item represents a token or text string returned from the scanner.
type Item struct {
	Typ ItemType // The type of this item.
	Pos Pos      // The starting position, in bytes, of this item in the input string.
	Val string   // The value of this item.
}

func (i Item) String() string {
	switch {
	case i.Typ == ItemComment:
		return fmt.Sprint(i.Val)
	case i.Typ == ItemEOF:
		return "EOF"
	case i.Typ == ItemError:
		return i.Val
	case i.Typ == ItemModifiers:
	case i.Typ > ItemKeyword:
		return fmt.Sprintf("<%s>\t", i.Val)
	}
	return fmt.Sprintf("%q\t%s", i.Val, Rkey[i.Typ])
}

// ItemType identifies the type of lex items.
type ItemType int

const (
	ItemError        ItemType = iota // error occurred; value is text of error
	ItemBool                         // boolean constant
	ItemChar                         // printable ASCII character; grab bag for comma etc.
	ItemCharConstant                 // character constant
	ItemComplex                      // complex constant (1+2i); imaginary is just a number
	ItemColonEquals                  // colon-equals (':=') introducing a declaration
	ItemEOF
	ItemField      // alphanumeric identifier starting with '.'
	ItemIdentifier // alphanumeric identifier not starting with '.'
	ItemLeftDelim  // left action delimiter
	ItemLeftParen  // '(' inside action
	ItemNumber     // simple number, including imaginary
	ItemPipe       // pipe symbol
	ItemRawString  // raw quoted string (includes quotes)
	ItemRightDelim // right action delimiter
	ItemRightParen // ')' inside action
	ItemSpace      // run of spaces separating arguments
	ItemNewline    // newline
	ItemString     // quoted string (includes quotes)
	ItemText       // plain text
	ItemVariable   // variable starting with '$', such as '$' or  '$1' or '$hello'
	ItemLeftBrace
	ItemRightBrace
	ItemComment
	ItemOperator
	// Keywords appear after all the rest.
	ItemKeyword  // used only to delimit the keywords
	ItemDot      // the cursor, spelled '.'
	ItemDefine   // define keyword
	ItemElse     // else keyword
	ItemEnd      // end keyword
	ItemIf       // if keyword
	ItemNil      // the untyped nil constant, easiest to treat as a keyword
	ItemRange    // range keyword
	ItemTemplate // template keyword
	ItemWith     // with keyword
	ItemFor      // for keyword
	ItemSwitch   // switch keyword
	ItemCase     // case keyword
	ItemWhile    // while keyword
	ItemReturn   // return keyword
	ItemBreak    // break keyword
	ItemContinue // continue keyword
	ItemVar      // var keyword
	ItemEnum     // enum keyword
	ItemStruct   // struct keyword
	ItemFunction // function keyword
	ItemEvent    // event keyword
	ItemClass    // class keyword
	ItemArray    // array keyword
	ItemModifiers
)

var key = map[string]ItemType{
	".":      ItemDot,
	"define": ItemDefine,
	"else":   ItemElse,
	// "end":    ItemEnd,
	"if": ItemIf,
	// "range":        ItemRange,
	// "nil": ItemNil,
	// "template": ItemTemplate,
	// "with":     ItemWith,
	"for":    ItemFor, // ws keywords
	"switch": ItemSwitch,
	"case":   ItemCase,
	"while":  ItemWhile,
	"return": ItemReturn,
	// "break":  ItemBreak,
	// "continue":     ItemContinue,
	"var":      ItemVar,
	"enum":     ItemEnum,
	"struct":   ItemStruct,
	"function": ItemFunction,
	// "event":        ItemEvent,
	// "class":        ItemClass,
	"array":        ItemArray,
	"abstract":     ItemModifiers, // ws modifiers
	"entry":        ItemModifiers,
	"out":          ItemModifiers,
	"saved":        ItemModifiers,
	"storyscene":   ItemModifiers,
	"quest":        ItemModifiers,
	"exec":         ItemModifiers,
	"timer":        ItemModifiers,
	"final":        ItemModifiers,
	"import":       ItemModifiers,
	"const":        ItemModifiers,
	"editable":     ItemModifiers,
	"default":      ItemModifiers,
	"statemachine": ItemModifiers,
	"private":      ItemModifiers,
	"protected":    ItemModifiers,
	"public":       ItemModifiers,
}

var Rkey = map[ItemType]string{
	ItemError:        "error",
	ItemBool:         "bool",
	ItemChar:         "char",
	ItemCharConstant: "charConstant",
	ItemComplex:      "complex",
	ItemColonEquals:  "colonEquals",
	ItemEOF:          "EOF",
	ItemField:        "field",
	ItemIdentifier:   "identifier",
	ItemLeftDelim:    "leftDelim",
	ItemLeftParen:    "leftParen",
	ItemNumber:       "number",
	ItemPipe:         "pipe",
	ItemRawString:    "rawString",
	ItemRightDelim:   "rightDelim",
	ItemRightParen:   "rightParen",
	ItemSpace:        "space",
	ItemNewline:      "newline",
	ItemString:       "string",
	ItemText:         "text",
	ItemVariable:     "variable",
	ItemOperator:     "operator",
	ItemModifiers:    "modifier",
	ItemLeftBrace:    "leftBrace",
	ItemRightBrace:   "rightBrace",
}

const eof = -1

// Pos represents a byte position in the original input text from which
// this template was parsed.
type Pos int

func (p Pos) Position() Pos {
	return p
}

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*Lexer) stateFn

// Lexer holds the state of the scanner.
type Lexer struct {
	name       string    // the name of the input; used only for error reports
	input      string    // the string being scanned
	leftDelim  string    // start of action
	rightDelim string    // end of action
	state      stateFn   // the next lexing function to enter
	pos        Pos       // current position in the input
	start      Pos       // start position of this item
	width      Pos       // width of last rune read from input
	lastPos    Pos       // position of most recent item returned by nextItem
	items      chan Item // channel of scanned items
	parenDepth int       // nesting depth of ( ) exprs
	braceDepth int       // nesting depth of { }
}

// next returns the next rune in the input.
func (l *Lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
	l.pos += l.width
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *Lexer) peek() rune {
	if int(l.pos) >= len(l.input) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.pos:])
	return r
}

// backup steps back one rune. Can only be called once per call of next.
func (l *Lexer) backup() {
	l.pos -= l.width
}

// emit passes an item back to the client.
func (l *Lexer) emit(t ItemType) {
	l.items <- Item{t, l.start, l.input[l.start:l.pos]}
	l.start = l.pos
}

// ignore skips over the pending input before this point.
func (l *Lexer) ignore() {
	l.start = l.pos
}

// accept consumes the next rune if it's from the valid set.
func (l *Lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.backup()
	return false
}

// acceptRun consumes a run of runes from the valid set.
func (l *Lexer) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
	}
	l.backup()
}

// lineNumber reports which line we're on, based on the position of
// the previous item returned by nextItem. Doing it this way
// means we don't have to worry about peek double counting.
func (l *Lexer) lineNumber() int {
	return 1 + strings.Count(l.input[:l.lastPos], "\n")
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *Lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- Item{ItemError, l.start, fmt.Sprintf(format, args...)}
	return nil
}

// nextItem returns the next item from the input.
// Called by the parser, not in the lexing goroutine.
func (l *Lexer) NextItem() Item {
	item := <-l.items
	l.lastPos = item.Pos
	return item
}

// drain drains the output so the lexing goroutine will exit.
// Called by the parser, not in the lexing goroutine.
func (l *Lexer) drain() {
	for range l.items {
	}
}

// lex creates a new scanner for the input string.
func Lex(name, input string) *Lexer {
	l := &Lexer{
		name:  name,
		input: input,
		items: make(chan Item),
	}
	go l.run()
	return l
}

// run runs the state machine for the lexer.
func (l *Lexer) run() {
	for l.state = lexInsideAction; l.state != nil; {
		l.state = l.state(l)
	}
	close(l.items)
}

// state functions

const (
	leftComment  = "/*"
	rightComment = "*/"
)

// lexComment scans a comment. The left comment marker is known to be present.
func lexComment(l *Lexer) stateFn {
	l.pos += Pos(len(leftComment))
	i := strings.Index(l.input[l.pos:], rightComment)
	if i < 0 {
		return l.errorf("unclosed comment")
	}
	l.pos += Pos(i + len(rightComment))
	/*if !strings.HasPrefix(l.input[l.pos:], l.rightDelim) {
		return l.errorf("comment ends before closing delimiter")

	}
	l.pos += Pos(len(l.rightDelim))*/
	l.emit(ItemComment)
	return lexInsideAction
}

// lexSingleLineComment scans until the end of the line
func lexSingleLineComment(l *Lexer) stateFn {
	i := strings.Index(l.input[l.pos:], "\n")
	if i < 0 {
		l.pos = Pos(len(l.input))
	}
	l.pos += Pos(i)

	l.emit(ItemComment)
	return lexInsideAction
}

// lexInsideAction scans the elements inside action delimiters.
func lexInsideAction(l *Lexer) stateFn {
	// Either number, quoted string, or identifier.
	// Spaces separate arguments; runs of spaces turn into ItemSpace.
	// Pipe symbols separate and are emitted.
	switch r := l.next(); {
	case r == eof:
		if l.parenDepth != 0 {
			return l.errorf("unclosed left paren")
		}
		if l.braceDepth != 0 {
			return l.errorf("unclosed left paren")
		}
		l.emit(ItemEOF)
		return nil
	case isEndOfLine(r):
		return lexEOL
	case isSpace(r):
		return lexSpace
	case strings.HasPrefix(l.input[l.pos-l.width:], leftComment):
		return lexComment
	case strings.HasPrefix(l.input[l.pos-l.width:], "//"):
		return lexSingleLineComment
	case r == '"':
		return lexQuote
	case r == '`':
		return lexRawQuote
	case r == '$':
		return lexVariable
	case r == '\'':
		return lexChar
	case r == '.':
		// special look-ahead for ".field" so we don't break l.backup().
		r = l.peek()
		if r < '0' || '9' < r {
			l.emit(ItemDot)
			return lexInsideAction
		}
		fallthrough // '.' can start a number.
	case '0' <= r && r <= '9':
		l.backup()
		return lexNumber
	case isOperator(r):
		return lexOperator
	case isAlphaNumeric(r):
		l.backup()
		return lexIdentifier
	case r == '(':
		l.emit(ItemLeftParen)
		l.parenDepth++
	case r == ')':
		l.emit(ItemRightParen)
		l.parenDepth--
		if l.parenDepth < 0 {
			return l.errorf("unexpected right paren %#U", r)
		}
	case r == '{':
		l.emit(ItemLeftBrace)
		l.braceDepth++
	case r == '}':
		l.emit(ItemRightBrace)
		l.braceDepth--
		if l.braceDepth < 0 {
			return l.errorf("unexpected right brace %#U", r)
		}
	case r <= unicode.MaxASCII && unicode.IsPrint(r):
		l.emit(ItemChar)
		return lexInsideAction
	default:
		return l.errorf("unrecognized character in action: %#U", r)
	}
	return lexInsideAction
}

func lexOperator(l *Lexer) stateFn {
	l.acceptRun("%&*/!+=-|")
	l.emit(ItemOperator)
	return lexInsideAction
}

// lexEOL scans a run of end of line characters.
// One character has already been seen.
func lexEOL(l *Lexer) stateFn {
	for isEndOfLine(l.peek()) {
		l.next()
	}
	l.emit(ItemNewline)
	return lexInsideAction
}

// lexSpace scans a run of space characters.
// One space has already been seen.
func lexSpace(l *Lexer) stateFn {
	for isSpace(l.peek()) {
		l.next()
	}
	l.emit(ItemSpace)
	return lexInsideAction
}

// lexIdentifier scans an alphanumeric.
func lexIdentifier(l *Lexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// absorb.
		default:
			l.backup()
			word := l.input[l.start:l.pos]
			if !l.atTerminator() {
				return l.errorf("bad character %#U", r)
			}
			switch {
			case key[word] > ItemKeyword:
				l.emit(key[word])
			case word[0] == '.':
				l.emit(ItemField)
			case word == "true", word == "false":
				l.emit(ItemBool)
			default:
				l.emit(ItemIdentifier)
			}
			break Loop
		}
	}
	return lexInsideAction
}

// lexField scans a field: .Alphanumeric.
// The . has been scanned.
func lexField(l *Lexer) stateFn {
	return lexFieldOrVariable(l, ItemField)
}

// lexVariable scans a Variable: $Alphanumeric.
// The $ has been scanned.
func lexVariable(l *Lexer) stateFn {
	if l.atTerminator() { // Nothing interesting follows -> "$".
		l.emit(ItemVariable)
		return lexInsideAction
	}
	return lexFieldOrVariable(l, ItemVariable)
}

// lexVariable scans a field or variable: [.$]Alphanumeric.
// The . or $ has been scanned.
func lexFieldOrVariable(l *Lexer, typ ItemType) stateFn {
	if l.atTerminator() { // Nothing interesting follows -> "." or "$".
		if typ == ItemVariable {
			l.emit(ItemVariable)
		} else {
			l.emit(ItemDot)
		}
		return lexInsideAction
	}
	var r rune
	for {
		r = l.next()
		if !isAlphaNumeric(r) {
			l.backup()
			break
		}
	}
	if !l.atTerminator() {
		return l.errorf("bad character %#U", r)
	}
	l.emit(typ)
	return lexInsideAction
}

// atTerminator reports whether the input is at valid termination character to
// appear after an identifier. Breaks .X.Y into two pieces. Also catches cases
// like "$x+2" not being acceptable without a space, in case we decide one
// day to implement arithmetic.
func (l *Lexer) atTerminator() bool {
	r := l.peek()
	if isSpace(r) || isEndOfLine(r) {
		return true
	}
	switch r {
	case eof, '.', ',', '|', ':', ')', '(', ';', '[', ']', '?', '{':
		return true
	}
	if isOperator(r) {
		return true
	}
	// Does r start the delimiter? This can be ambiguous (with delim=="//", $x/2 will
	// succeed but should fail) but only in extremely rare cases caused by willfully
	// bad choice of delimiter.
	if rd, _ := utf8.DecodeRuneInString(l.rightDelim); rd == r {
		return true
	}
	return false
}

// lexChar scans a character constant. The initial quote is already
// scanned. Syntax checking is done by the parser.
func lexChar(l *Lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated character constant")
		case '\'':
			break Loop
		}
	}
	l.emit(ItemString)
	// l.emit(ItemCharConstant)
	return lexInsideAction
}

// lexNumber scans a number: decimal, octal, hex, float, or imaginary. This
// isn't a perfect number scanner - for instance it accepts "." and "0x0.2"
// and "089" - but when it's wrong the input is invalid and the parser (via
// strconv) will notice.
func lexNumber(l *Lexer) stateFn {
	if !l.scanNumber() {
		return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
	} // complex number logic removed. Messes with math operations without space
	l.emit(ItemNumber)

	return lexInsideAction
}

func (l *Lexer) scanNumber() bool {
	// Optional leading sign.
	l.accept("+-")
	// Is it hex?
	digits := "0123456789"
	if l.accept("0") && l.accept("xX") {
		digits = "0123456789abcdefABCDEF"
	}
	l.acceptRun(digits)
	if l.accept(".") {
		l.acceptRun(digits)
	}
	if l.accept("eE") {
		l.accept("+-")
		l.acceptRun("0123456789")
	}
	// Is it imaginary?
	l.accept("if") // ws alows f after a float
	// Next thing mustn't be alphanumeric.
	if isAlphaNumeric(l.peek()) {
		l.next()
		return false
	}
	return true
}

// lexQuote scans a quoted string.
func lexQuote(l *Lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated quoted string")
		case '"':
			break Loop
		}
	}
	l.emit(ItemString)
	return lexInsideAction
}

// lexRawQuote scans a raw quoted string.
func lexRawQuote(l *Lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case eof:
			return l.errorf("unterminated raw quoted string")
		case '`':
			break Loop
		}
	}
	l.emit(ItemRawString)
	return lexInsideAction
}

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// isEndOfLine reports whether r is an end-of-line character.
func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isOperator(r rune) bool {
	return strings.IndexRune("%&*/!+=-|<>", r) >= 0
}
