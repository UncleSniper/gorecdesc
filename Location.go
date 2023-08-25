package gorecdesc

import (
	"fmt"
	"strconv"
)

type Location struct {
	File string
	Line uint
	Column uint
}

func(location *Location) isEmpty() bool {
	return len(location.File) == 0 && location.Line == 0
}

func(location *Location) Format() string {
	if location == nil || location.isEmpty() {
		return "<unknown location>"
	}
	var f, l, c string
	if len(location.File) == 0 {
		f = "<unknown file>"
	} else {
		f = location.File
	}
	if location.Line == 0 {
		l = "<unknown line>"
	} else {
		l = strconv.FormatUint(uint64(location.Line), 10)
	}
	if location.Column == 0 {
		c = ""
	} else {
		c = fmt.Sprintf(":%d", location.Column)
	}
	return fmt.Sprintf("%s:%s%s", f, l, c)
}

func(location *Location) NextLine() {
	if location.Line > 0 {
		location.Line++
	}
	if location.Column > 0 {
		location.Column = 1
	}
}

func(location *Location) NextColumn() {
	if location.Column > 0 {
		location.Column++
	}
}

func StartOfFile(file string) Location {
	return Location {
		File: file,
		Line: 1,
		Column: 1,
	}
}

func SomewhereInFile(file string) Location {
	return Location {
		File: file,
	}
}
