package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"timmy.narnian.us/git/timmy/wsfmt/text/lex"
)

type stateFn func(*Formatter) stateFn

type Formatter struct {
	l             *lex.Lexer
	previousToken lex.Item
	token         lex.Item
	nextToken     lex.Item
	state         stateFn
	maxNewlines   int
	newlineCount  int
	parenDepth    int
	Output        strings.Builder
	scopeLevel    []int
	// Each index is one scope deep delimited by braces or case statement.
	// the number is how many 'soft' scopes deep it is resets to 1 on newline e.g. an if statement without braces
}

var (
	blank  = lex.Item{Pos: -1}
	b      []byte
	fmtErr error
)

func main() {

	file, _ := os.Open(os.Args[1])
	f := Format(file)
	f.run()
	fmt.Println(f.Output.String())
	if fmtErr != nil {
		os.Stdout.Sync()
		fmt.Fprintln(os.Stderr, "\n", fmtErr)
		os.Exit(1)
	}
}

func Format(text io.Reader) (f *Formatter) {
	var err error
	FILE := transform.NewReader(text, unicode.BOMOverride(unicode.UTF8.NewDecoder().Transformer))
	b, err = ioutil.ReadAll(FILE)
	if err != nil {
		panic(err)
	}
	f = &Formatter{}
	f.l = lex.Lex("name", string(b))
	f.maxNewlines = 3
	f.nextToken = blank
	return f
}

func (f *Formatter) next() lex.Item {
	f.previousToken = f.token
	var temp lex.Item

	if f.nextToken == blank {
		temp = f.l.NextItem()
	} else {
		temp = f.nextToken
		f.nextToken = blank
	}
	for ; temp.Typ == lex.ItemSpace || temp.Typ == lex.ItemNewline; temp = f.l.NextItem() {
	}

	f.token = temp
	return f.token
}

func (f *Formatter) peek() lex.Item {
	if f.nextToken == blank {
		temp := f.l.NextItem()
		count := 0
		for ; temp.Typ == lex.ItemSpace || temp.Typ == lex.ItemNewline; temp = f.l.NextItem() {
			if temp.Typ == lex.ItemNewline {
				count += strings.Count(temp.Val, "\n")
			}
		}
		if count < 3 {
			f.newlineCount = count
		} else {
			f.newlineCount = 3
		}
		f.nextToken = temp
	}
	return f.nextToken
}

func (f *Formatter) run() {
	for f.state = format; f.state != nil; {
		f.state = f.state(f)
	}
}

func errorf(format string, args ...interface{}) stateFn {
	fmtErr = fmt.Errorf(format, args...)
	return nil
}

func format(f *Formatter) stateFn {
	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		return nil
	case t == lex.ItemError:
		return errorf("error: %s", f.token.Val)
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemFunction:
		return formatFunction
	case t == lex.ItemIf, t == lex.ItemWhile, t == lex.ItemFor, t == lex.ItemSwitch:
		return formatConditional
	case t == lex.ItemElse:
		if f.previousToken.Typ == lex.ItemRightBrace {
			f.Output.WriteString(" else")
		} else {
			f.Output.WriteString("else")
		}
		if f.peek().Typ != lex.ItemLeftBrace && f.peek().Typ != lex.ItemIf {
			f.scopeLevel[len(f.scopeLevel)-1]++
			printNewline(f)
			printTab(f)
		}
	case t == lex.ItemReturn:
		f.Output.WriteString(f.token.Val + " ")
	case t == lex.ItemModifiers, t == lex.ItemIdentifier, t == lex.ItemNumber, t == lex.ItemBool, t == lex.ItemString:
		if !printIdentifier(f) {
			return errorf("invalid identifier: trailing dot '.'")
		}
	case isChar(t):
		return printChar(f)
	case t == lex.ItemStruct:
		return formatStruct
	case t == lex.ItemVar:
		return formatVar
	case t == lex.ItemOperator:
		printOperator(f)
	case t == lex.ItemArray:
		return formatArray
	case t == lex.ItemCase:
		return formatCase
	case t == lex.ItemEnum:
		return formatEnum
	default:
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	return format
}

func (f *Formatter) printComment() {
	f.Output.WriteString(f.token.Val)
	printNewline(f)
}

func formatFunction(f *Formatter) stateFn {
	if f.token.Typ == lex.ItemFunction {
		f.Output.WriteString(f.token.Val + " ")
	}

	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		return errorf("unexpected EOF wanted identifier\n")
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemIdentifier:
		if !printIdentifier(f) {
			return errorf("invalid identifier: trailing dot '.'")
		}
		if f.next().Typ == lex.ItemLeftParen {
			printChar(f)
			return format
		}
		return errorf("expected left Parenthesis got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
}

func formatStruct(f *Formatter) stateFn {
	if f.token.Typ == lex.ItemStruct {
		if !printIdentifier(f) {
			return errorf("invalid identifier: trailing dot '.'")
		}
	}
	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		return errorf("unexpected EOF wanted identifier\n")
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemIdentifier:
		if !printIdentifier(f) {
			return errorf("invalid identifier: trailing dot '.'")
		}
		return format
	default:
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	return formatStruct
}

func formatVar(f *Formatter) stateFn {
	if !printIdentifier(f) {
		return errorf("invalid identifier: trailing dot '.'")
	}
	for notdone := true; notdone; {
		if f.next().Typ == lex.ItemIdentifier {
			if !printIdentifier(f) {
				return errorf("invalid identifier: trailing dot '.'")
			}
			if f.next().Typ == lex.ItemChar {
				switch f.token.Val {
				case ",":
					printChar(f)
				case ":":
					printChar(f)
					notdone = false
				default:
					return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)

				}
			} else {
				return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)

			}
		} else {
			return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)

		}
	}
	switch f.next().Typ {
	case lex.ItemIdentifier:
		if !printIdentifier(f) {
			return errorf("invalid identifier: trailing dot '.'")
		}
	case lex.ItemArray:
		return formatArray // errorf("Bad array syntax")
	default:
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)

	}
	return format
}

func printIdentifier(f *Formatter) bool {
	switch i := f.peek(); {
	case i.Val == "{", i.Val == "}", i.Val == "(", i.Val == ")", i.Val == "[", i.Val == "]", i.Val == "|", i.Val == ",", i.Val == ":", i.Val == ";":
		f.Output.WriteString(f.token.Val)
	case i.Typ == lex.ItemDot:
		f.Output.WriteString(f.token.Val)
		f.next()
		return printDot(f)
	default:
		f.Output.WriteString(f.token.Val + " ")
	}
	return true
}

func printDot(f *Formatter) bool {
	f.Output.WriteString(".")
	if f.peek().Typ == lex.ItemIdentifier {
		f.next()
		return printIdentifier(f)
	}
	return false
}

func printOperator(f *Formatter) {
	str := "%s"
	switch f.token.Val {
	case "|", "!":
	case "+", "-":
		switch f.previousToken.Typ {
		case lex.ItemLeftParen, lex.ItemOperator, lex.ItemReturn, lex.ItemCase:
		default:
			str = "%s "
		}
	default:
		str = "%s "
	}
	switch f.previousToken.Val {
	case ")", "]":
		str = " " + str
	}
	f.Output.WriteString(fmt.Sprintf(str, f.token.Val))
}

func formatConditional(f *Formatter) stateFn {
	switch f.token.Typ {
	case lex.ItemIf, lex.ItemWhile, lex.ItemFor, lex.ItemSwitch:
		if f.previousToken.Typ == lex.ItemElse {
			f.Output.WriteString(" ")
		}
		if f.next().Typ != lex.ItemLeftParen {
			return errorf("expected parenthesis got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
		f.Output.WriteString(f.previousToken.Val + " (")
		f.parenDepth = 1
	}

	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		return errorf("unexpected EOF wanted identifier\n")
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemOperator:
		printOperator(f)
	case t == lex.ItemIdentifier, t == lex.ItemNumber, t == lex.ItemString, t == lex.ItemBool:
		if !printIdentifier(f) {
			return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
	case isChar(t):
		if f.token.Val == ";" {
			f.Output.WriteString("; ")
		} else {
			printChar(f)
		}
		switch f.token.Val {
		case ")":
			f.parenDepth--
			if f.parenDepth == 0 {
				if f.peek().Typ != lex.ItemLeftBrace {
					f.scopeLevel[len(f.scopeLevel)-1]++
					printNewline(f)
					printTab(f)
				}
				return format
			}
		case "(":
			f.parenDepth++
		}
	}
	return formatConditional
}

func formatNewLine(f *Formatter) stateFn {

	switch t := f.peek().Typ; {
	case t == lex.ItemEOF:
		f.Output.WriteString("\n")
		return nil
	case t == lex.ItemError:
		return errorf("error: " + f.token.Val)
	case t == lex.ItemCase:
		printNewline(f)
		f.scopeLevel = f.scopeLevel[:len(f.scopeLevel)-1]
		printTab(f)
	case f.peek().Val == ";" || f.peek().Val == "}":
	default:
		printNewline(f)
		printTab(f)
	}

	return format
}

func formatEnum(f *Formatter) stateFn {
	if !printIdentifier(f) {
		return errorf("invalid identifier: trailing dot '.'")
	}
	if f.next().Typ != lex.ItemIdentifier {
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	if !printIdentifier(f) {
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	if f.next().Typ != lex.ItemLeftBrace {
		return errorf("expected left brace got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}

	f.scopeLevel = append(f.scopeLevel, 1)
	f.Output.WriteString(" {")
	printNewline(f)
	printTab(f)
	return formatEnumIdent
}

func formatEnumIdent(f *Formatter) stateFn {
	if f.next().Typ != lex.ItemIdentifier {
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	if !printIdentifier(f) {
		return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}
	switch f.peek().Val {
	case "=":
		f.next()
		printOperator(f)
		if f.peek().Typ != lex.ItemNumber {
			return errorf("expected Number got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
		f.next()
		if !printIdentifier(f) {
			return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
		if f.peek().Typ == lex.ItemRightBrace {
			return format
		}
	case "}":
		return format
	}
	return formatEnumChar
}

func formatEnumChar(f *Formatter) stateFn {
	if f.next().Val != "," {
		return errorf("expected Comma got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	}

	f.Output.WriteString(",")
	if f.peek().Typ == lex.ItemRightBrace {
		return format
	}
	printNewline(f)
	printTab(f)
	return formatEnumIdent
}

func formatRightBrace(f *Formatter) stateFn {
	f.scopeLevel = f.scopeLevel[:len(f.scopeLevel)-1]
	if f.previousToken.Typ != lex.ItemLeftBrace {
		f.Output.WriteString("\n")
		printTab(f)
	}
	f.Output.WriteString("}")

	switch f.peek().Typ {
	case lex.ItemChar, lex.ItemElse, lex.ItemRightBrace:
		return format
	}
	return formatNewLine
}

func formatCase(f *Formatter) stateFn {
	if !printIdentifier(f) {
		return errorf("invalid identifier: trailing dot '.'")
	}
	switch f.next().Typ {
	case lex.ItemLeftParen:
		f.Output.WriteString(" (")
		if f.next().Typ != lex.ItemIdentifier || !printIdentifier(f) {
			return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
		if f.next().Typ != lex.ItemRightParen {
			return errorf("expected parenthesis got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
		f.Output.WriteString(")")
		if f.next().Typ != lex.ItemIdentifier || !printIdentifier(f) {
			return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
	case lex.ItemIdentifier, lex.ItemNumber, lex.ItemString:
		if !printIdentifier(f) {
			return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
	case lex.ItemOperator:
		if (f.token.Val == "+" || f.token.Val == "-") && f.peek().Typ == lex.ItemNumber {
			printOperator(f)
			f.next()
			if !printIdentifier(f) {
				return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			}
		} else {
			return errorf("Invalid Operator got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		}
	}

	if f.next().Val != ":" {
		return errorf("expected \":\" got %s: %s %s\n", lex.Rkey[f.token.Typ], f.token.Val, f.previousToken.Val)
	}
	f.Output.WriteString(":")
	f.scopeLevel = append(f.scopeLevel, 1)
	return formatNewLine
}

func formatArray(f *Formatter) stateFn {
	if f.next().Val != "<" {
		return errorf("expected \"<\" got %s: %s %s\n", lex.Rkey[f.token.Typ], f.token.Val, f.previousToken.Val)
	}
	f.Output.WriteString("array<")
	switch f.next().Typ {
	case lex.ItemIdentifier:
		for {
			f.Output.WriteString(f.token.Val)
			if f.peek().Typ != lex.ItemDot {
				break
			}
			f.next()
			f.Output.WriteString(".")
			if f.peek().Typ != lex.ItemIdentifier {
				return errorf("expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			}
			f.next()
		}
	case lex.ItemArray:
		return formatArray
	default:
		return errorf("Bad array syntax")
	}
	if f.next().Val != ">" {
		return errorf("expected \">\" got %s: %s %s\n", lex.Rkey[f.token.Typ], f.token.Val, f.previousToken.Val)
	}
	for f.peek(); f.nextToken.Val == ">"; {
		f.next()
		f.Output.WriteString(">")
	}
	if f.peek().Typ == lex.ItemOperator && f.peek().Val != ">" {
		f.Output.WriteString("> ")
	} else {
		f.Output.WriteString(">")
	}
	return format
}

func printTab(f *Formatter) {
	for _, t := range f.scopeLevel {
		for i := 0; i < t; i++ {
			f.Output.WriteString("\t")
		}
	}
}

func isChar(t lex.ItemType) bool {
	switch t {
	case lex.ItemChar, lex.ItemLeftParen, lex.ItemRightParen, lex.ItemLeftBrace, lex.ItemRightBrace, lex.ItemDot:
		return true
	default:
		return false
	}
}

func printChar(f *Formatter) stateFn {
	switch f.token.Val {
	case ":", ",":
		f.Output.WriteString(f.token.Val + " ")
	case ";":
		f.Output.WriteString(";")
		if len(f.scopeLevel) > 0 {
			f.scopeLevel[len(f.scopeLevel)-1] = 1
		}
		return formatNewLine
	case "{":
		f.Output.WriteString(" {")
		f.scopeLevel = append(f.scopeLevel, 1)
		return formatNewLine
	case "}":
		return formatRightBrace
	case ".":
		printDot(f)
	default:
		f.Output.WriteString(f.token.Val)
	}
	return format
}

func printNewline(f *Formatter) {
	f.peek()
	if f.nextToken.Typ != lex.ItemEOF {
		for i := 0; i < f.newlineCount; i++ {
			f.Output.WriteString("\n")
		}
	}
}
