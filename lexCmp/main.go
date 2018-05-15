package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"timmy.narnian.us/git/timmy/wsfmt/text/lex"
)

var (
	args struct {
		ORIG     string `arg:"required,positional,help:Directory or file containing original files"`
		MODIFIED string `arg:"required,positional,help:Directory or file containing modified files e.g. reformatted files"`
	}
	original string
	modified string
)

func main() {
	arg.MustParse(&args)
	args.MODIFIED = filepath.Clean(args.MODIFIED)
	args.ORIG = filepath.Clean(args.ORIG)

	filepath.Walk(args.ORIG, func(path string, info os.FileInfo, err error) error {

		original = path
		modified = filepath.Join(args.MODIFIED, strings.TrimPrefix(path, args.ORIG))

		if err != nil {
			fmt.Println(err)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		in, err := os.Stat(modified)
		if err != nil {
			fmt.Println(err)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if in.IsDir() != info.IsDir() {
			var (
				dir  = modified
				file = original
			)
			if info.IsDir() {
				dir, file = file, dir
				fmt.Printf("File directory mismatch: Directory: %q File: %q\n", dir, file)
				return filepath.SkipDir
			}
			fmt.Printf("File directory mismatch: Directory: %q File: %q\n", dir, file)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		origL, err := Lex(original)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		modL, err := Lex(modified)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		if !cmp(origL, modL) {
			os.Exit(3)
		}
		return nil
	})
}

func cmp(l *lex.Lexer, l2 *lex.Lexer) bool {
	for Continue := true; Continue; {
		var (
			lItem  = next(l)
			l2Item = next(l2)
		)
		if lItem.Typ == l2Item.Typ {
			switch lItem.Typ {
			case lex.ItemIdentifier, lex.ItemChar, lex.ItemCharConstant, lex.ItemString, lex.ItemBool, lex.ItemComment, lex.ItemModifiers, lex.ItemNumber, lex.ItemOperator:
				if strings.Replace(lItem.Val, "\r", "", -1) != strings.Replace(l2Item.Val, "\r", "", -1) {
					fmt.Printf("Value mismatch %s: %s is not %s: %s\n", original, lItem, modified, l2Item)
					return false
				}
			}
		} else {
			fmt.Printf("Value mismatch %s: %s is not %s: %s\n", original, lItem, modified, l2Item)
			return false
		}
		if lItem.Typ == lex.ItemEOF {
			Continue = false
		}
		if lItem.Typ == lex.ItemError {
			fmt.Println(lItem)
		}
		if l2Item.Typ == lex.ItemError {
			fmt.Println(l2Item)
		}
	}
	return true
}

func Lex(file string) (*lex.Lexer, error) {
	var (
		err  error
		b    []byte
		text io.Reader
	)
	text, err = os.Open(file)
	if err != nil {
		fmt.Println(err)
		return lex.Lex("fail", string("Fail")), err
	}
	FILE := transform.NewReader(text, unicode.BOMOverride(unicode.UTF8.NewDecoder().Transformer))
	b, err = ioutil.ReadAll(FILE)
	if err != nil {
		return lex.Lex("fail", string("Fail")), err
	}
	return lex.Lex(file, string(b)), nil
}

func next(l *lex.Lexer) lex.Item {
	var temp lex.Item
	for temp = l.NextItem(); temp.Typ == lex.ItemSpace || temp.Typ == lex.ItemNewline; temp = l.NextItem() {
	}
	return temp
}
