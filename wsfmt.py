import chardet
import string


class Parser:
	wordFunc = {
		"if": "wordif"
	}
	charFunc = {
		"/": "comment"
	}
	wordBoundary = [
			"{", "}", "<", ">", "(", ")", "[", "]",
			";", ":", '"', "'", "=", "!", "-", "+", "/", "*", ",", "%"
	].extend(string.whitespace)

	def __init__(self, FileName: str):
		self.indent = 0
		self.fmt = ""
		self.currentWord = ""
		self.wordIndex = 0
		self.file = self.openfile(FileName)
		self.possibleLineComment = False
		self.multiLineComment = False

	def openfile(FileName: str) -> str:
		f = open(FileName, mode='b')
		bytestr = f.read(10)
		enc = chardet.detect(bytestr)
		f.close()

		return open(FileName, encoding=enc['encoding'], buffering=1)

	def getNextWord(self):
		cont = True
		word = ""
		ignorewhitespace = True
		while cont:
			char = self.file.read(1)
			if not ignorewhitespace:
				word += char
			else:
				if char not in string.whitespace:
					ignorewhitespace = False
					word += char

			if char in self.wordBoundary and not ignorewhitespace:
				cont = False
		self.currentWord = word.strip()

	def parseWord(self):

	def parse(self):
		cont = True
		while cont:
			self.getNextWord()
			self.parseWord()


wsparser = Parser("test.ws")
wsparser.parse()
