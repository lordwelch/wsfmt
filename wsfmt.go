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

type FmtSettings struct{}

type Formatter struct {
	l              *lex.Lexer
	pToken         lex.Item
	token          lex.Item
	nToken         lex.Item
	state          stateFn
	maxNewlines    int
	newlineCount   int
	scopeLevel     []int
	tempScopeLevel int
	eol, CASE      bool
	parenDepth     int
}

var (
	blank = lex.Item{Pos: -1}
	b     []byte
	err   error
)

func main() {
	file, _ := os.Open(os.Args[1])
	f := Format(nil, file)
	f.run()
}

func Format(typ *FmtSettings, text io.Reader) (f *Formatter) {
	FILE := transform.NewReader(text, unicode.BOMOverride(unicode.UTF8.NewDecoder().Transformer))
	b, err = ioutil.ReadAll(FILE)
	if err != nil {
		panic(err)
	}
	f = &Formatter{}
	f.l = lex.Lex("name", string(b))
	f.maxNewlines = 3
	f.nToken = blank
	return f
}

func (f *Formatter) next() lex.Item {
	f.pToken = f.token
	var temp lex.Item

	if f.nToken == blank {
		temp = f.l.NextItem()
	} else {
		temp = f.nToken
		f.nToken = blank
	}
	for ; temp.Typ == lex.ItemSpace || temp.Typ == lex.ItemNewline; temp = f.l.NextItem() {
	}

	f.token = temp
	return f.token
}

func (f *Formatter) peek() lex.Item {
	if f.nToken == blank {
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
		f.nToken = temp
	}
	return f.nToken
}

func (f *Formatter) run() {
	for f.state = format; f.state != nil; {
		f.state = f.state(f)
	}
}

func format(f *Formatter) stateFn {
	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		return nil
	case t == lex.ItemError:
		fmt.Print("error:", f.token.Val)
		return nil
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemFunction:
		return formatFunction
	case t == lex.ItemIf, t == lex.ItemWhile, t == lex.ItemFor, t == lex.ItemSwitch:
		return formatConditional
	case t == lex.ItemElse:
		if f.pToken.Typ == lex.ItemRightBrace {
			fmt.Printf(" else")
		} else {
			fmt.Print("else")
		}
		if f.peek().Typ != lex.ItemLeftBrace && f.peek().Typ != lex.ItemIf {
			f.scopeLevel[len(f.scopeLevel)-1]++
			printNewline(f)
			printTab(f)
		}
	case t == lex.ItemReturn:
		fmt.Printf("%s ", f.token.Val)
	case t == lex.ItemModifiers, t == lex.ItemIdentifier, t == lex.ItemNumber, t == lex.ItemBool, t == lex.ItemString:
		printIdentifier(f)
	case isChar(t):
		return printChar(f)
	case t == lex.ItemStruct:
		return formatStruct
	case t == lex.ItemVar:
		return formatVar
	case t == lex.ItemOperator:
		printOperator(f)
	case t == lex.ItemArray:
		if !printArray(f) {
			return nil
		}
	case t == lex.ItemCase:
		return formatCase
	case t == lex.ItemEnum:
		return formatEnum
	default:
		fmt.Fprintf(os.Stderr, "\nexpected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}
	return format
}

func (f *Formatter) printComment() {
	fmt.Printf("%s", f.token.Val)
	printNewline(f)
}

func formatFunction(f *Formatter) stateFn {
	if f.token.Typ == lex.ItemFunction {
		fmt.Printf("%s ", f.token.Val)
	}

	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		fmt.Fprintf(os.Stderr, "unexpected EOF wanted identifier\n")
		return nil
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemIdentifier:
		printIdentifier(f)
		if f.next().Typ == lex.ItemLeftParen {
			printChar(f)
			return format
		}
	}
	fmt.Print("\n")
	fmt.Fprintf(os.Stderr, "\nexpected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
	return nil
	// return formatFunction
}

func formatStruct(f *Formatter) stateFn {
	if f.token.Typ == lex.ItemStruct {
		printIdentifier(f)
	}
	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		fmt.Fprintf(os.Stderr, "unexpected EOF wanted identifier\n")
		return nil
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemIdentifier:
		printIdentifier(f)
		return format
	default:
		fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}
	return formatStruct
}

func formatVar(f *Formatter) stateFn {
	printIdentifier(f)
	for notdone := true; notdone; {
		if f.next().Typ == lex.ItemIdentifier {
			printIdentifier(f)
			if f.next().Typ == lex.ItemChar {
				switch f.token.Val {
				case ",":
					printChar(f)
				case ":":
					printChar(f)
					notdone = false
				default:
					fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
					return nil
				}
			} else {
				fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
				return nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			return nil
		}
	}
	switch f.next().Typ {
	case lex.ItemIdentifier:
		printIdentifier(f)
	case lex.ItemArray:
		if !printArray(f) {
			return nil
		}
	default:
		fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}
	return format
}

func printIdentifier(f *Formatter) {
	str := "%s"
	switch f.peek().Val {
	case "{", "}", "(", ")", "[", "]", "|", ",", ":", ";", ".":
	default:
		str += " "
	}
	fmt.Printf(str, f.token.Val)
}

func printOperator(f *Formatter) {
	str := "%s"
	switch f.token.Val {
	case "|", "!":
	case "+", "-":
		switch f.pToken.Typ {
		case lex.ItemLeftParen, lex.ItemOperator, lex.ItemReturn:
		default:
			str += " "
		}
	default:
		str += " "
	}
	switch f.pToken.Val {
	case ")", "]":
		str = " " + str
	}
	fmt.Printf(str, f.token.Val)
}

func formatConditional(f *Formatter) stateFn {
	switch f.token.Typ {
	case lex.ItemIf, lex.ItemWhile, lex.ItemFor, lex.ItemSwitch:
		tok := f.token.Val
		if f.pToken.Typ == lex.ItemElse {
			fmt.Print(" ")
		}
		if f.next().Typ != lex.ItemLeftParen {
			fmt.Fprintf(os.Stderr, "expected parenthesis got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			return nil
		}
		fmt.Printf("%s (", tok)
		f.parenDepth = 1
	}

	switch t := f.next().Typ; {
	case t == lex.ItemEOF:
		fmt.Fprintf(os.Stderr, "unexpected EOF wanted identifier\n")
		return nil
	case t == lex.ItemComment:
		f.printComment()
	case t == lex.ItemOperator:
		printOperator(f)
	case t == lex.ItemIdentifier, t == lex.ItemNumber, t == lex.ItemString, t == lex.ItemBool:
		printIdentifier(f)
	case isChar(t):
		if f.token.Val == ";" {
			fmt.Print("; ")
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
	printNewline(f)

	switch t := f.peek().Typ; {
	case t == lex.ItemEOF:
		return nil
	case t == lex.ItemError:
		fmt.Print("error:", f.token.Val)
		return nil
	case t == lex.ItemCase:
		f.scopeLevel = f.scopeLevel[:len(f.scopeLevel)-1]
		printTab(f)
	case isChar(t):
		switch f.nToken.Val {
		case ":", ",":
			fmt.Printf("%s ", f.token.Val)
		case ";":
			fmt.Print(f.token.Val)
			f.next()
			fmt.Print(f.token.Val)
			if len(f.scopeLevel) > 0 {
				f.scopeLevel[len(f.scopeLevel)-1] = 1
			}
		case "}":
			// f.scopeLevel = f.scopeLevel[:len(f.scopeLevel)-1]
			// printTab(f)
			// fmt.Print("}")
			f.next()
			f.peek()
			return formatRightBrace
		default:
			printTab(f)
		}
		return format
	default:
		printTab(f)
	}

	return format
}

func formatEnum(f *Formatter) stateFn {
	printIdentifier(f)
	if f.next().Typ != lex.ItemIdentifier {
		fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}
	printIdentifier(f)
	if f.next().Typ != lex.ItemLeftBrace {
		fmt.Fprintf(os.Stderr, "expected left brace got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}

	f.scopeLevel = append(f.scopeLevel, 1)
	fmt.Print(" {")
	f.newlineCount = -1
	printNewline(f)
	printTab(f)
	return formatEnumIdent
}

func formatEnumIdent(f *Formatter) stateFn {
	if f.next().Typ != lex.ItemIdentifier {
		fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}
	printIdentifier(f)
	switch f.peek().Val {
	case "=":
		f.next()
		printOperator(f)
		if f.peek().Typ != lex.ItemNumber {
			fmt.Fprintf(os.Stderr, "expected Number got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			return nil
		}
		f.next()
		printIdentifier(f)
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
		fmt.Fprintf(os.Stderr, "expected Comma got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return nil
	}

	fmt.Print(",")
	if f.peek().Typ == lex.ItemRightBrace {
		return format
	}
	printNewline(f)
	printTab(f)
	return formatEnumIdent
}

func formatRightBrace(f *Formatter) stateFn {
	f.scopeLevel = f.scopeLevel[:len(f.scopeLevel)-1]
	f.newlineCount = -1
	printNewline(f)
	printTab(f)
	fmt.Print("}")

	switch f.peek().Typ {
	case lex.ItemChar, lex.ItemElse, lex.ItemRightBrace:
		return format
	}

	return formatNewLine
}

func formatCase(f *Formatter) stateFn {
	printIdentifier(f)
	if !printCase(f) {
		return nil
	}

	if f.next().Val != ":" {
		fmt.Fprintf(os.Stderr, "\nexpected \":\" got %s: %s %s\n", lex.Rkey[f.token.Typ], f.token.Val, f.pToken.Val)
		return nil
	}
	fmt.Print(":")
	f.scopeLevel = append(f.scopeLevel, 1)
	return formatNewLine
}

func printCase(f *Formatter) bool {
	switch f.next().Typ {
	case lex.ItemLeftParen:
		fmt.Print(" (")
		if f.next().Typ != lex.ItemIdentifier {
			fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			return false
		}
		printIdentifier(f)
		if f.next().Typ != lex.ItemRightParen {
			fmt.Fprintf(os.Stderr, "expected parenthesis got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			return false
		}
		fmt.Print(")")
		if f.next().Typ != lex.ItemIdentifier {
			fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
			return false
		}
		printIdentifier(f)
	case lex.ItemIdentifier, lex.ItemNumber, lex.ItemString:
		printIdentifier(f)
	}
	return true
}

func printArray(f *Formatter) bool {
	if f.next().Val != "<" {
		fmt.Fprintf(os.Stderr, "expected \"<\" got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return false
	}
	fmt.Printf("array<")
	switch f.next().Typ {
	case lex.ItemIdentifier:
		fmt.Print(f.token.Val)
	case lex.ItemArray:
		printArray(f)
	default:
		fmt.Fprintf(os.Stderr, "expected Identifier got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return false
	}
	if f.next().Val != ">" {
		fmt.Fprintf(os.Stderr, "expected \">\" got %s: %s\n", lex.Rkey[f.token.Typ], f.token.Val)
		return false
	}
	fmt.Print(">")
	return true
}

func printTab(f *Formatter) {
	// fmt.Print(f.scopeLevel)
	for _, t := range f.scopeLevel {
		for i := 0; i < t; i++ {
			fmt.Print("\t")
		}
	}
}

func isChar(t lex.ItemType) bool {
	switch t {
	case lex.ItemChar, lex.ItemLeftParen, lex.ItemRightParen, lex.ItemLeftBrace, lex.ItemRightBrace:
		return true
	default:
		return false
	}
}

func printChar(f *Formatter) stateFn {
	switch f.token.Val {
	case ":", ",":
		fmt.Printf("%s ", f.token.Val)
	case ";":
		fmt.Print(";")
		if len(f.scopeLevel) > 0 {
			f.scopeLevel[len(f.scopeLevel)-1] = 1
		}
		return formatNewLine
	case "{":
		fmt.Print(" {")
		f.scopeLevel = append(f.scopeLevel, 1)
		f.newlineCount = -1
		return formatNewLine
	case "}":
		return formatRightBrace
	default:
		fmt.Print(f.token.Val)
	}
	return format
}

func printNewline(f *Formatter) {
	if f.newlineCount == -1 {
		f.peek()
		if f.newlineCount < 1 {
			f.newlineCount = 1
		}
	}
	f.peek()
	if f.nToken.Typ != lex.ItemEOF {
		for i := 0; i < f.newlineCount-1; i++ {
			fmt.Print("\n")
		}
		fmt.Print("\n")
	} else {
		fmt.Print("\n")
	}
}
