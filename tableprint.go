package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lawl/ayy/fancy"
)

type table struct {
	widths     []int
	formatters []fancy.Print
}

// create a table printer. specify width of each column.
// -1 to not pad to any size. only the last element is allowed to be -1
func newTable(columnWidth ...int) table {
	tbl := table{}

	for i := 0; i < len(columnWidth)-1; i++ {
		if columnWidth[i] < 0 {
			panic("only last table element is allowed to be any size (negative size passed columns index " + strconv.Itoa(i) + ")")
		}
	}

	tbl.widths = columnWidth
	tbl.formatters = make([]fancy.Print, len(columnWidth))

	return tbl
}
func (t table) withFormatters(fp ...fancy.Print) table {
	if len(fp) != len(t.widths) {
		panic("supplied formatter count doesn't match column count")
	}
	t.formatters = fp
	return t
}

func (t table) printRow(column ...string) {
	if len(column) != len(t.widths) {
		panic("supplied columns don't match expected size")
	}

	for i, c := range column {
		maxWidth := t.widths[i]
		strWidth := len([]rune(c))
		str := ""
		f := t.formatters[i]

		if maxWidth != -1 {
			if strWidth > maxWidth {
				str = f.Format(c[:maxWidth-3]) + "..."
				strWidth = maxWidth
			} else {
				str = f.Format(c)
			}

			if strWidth < maxWidth {
				pad := strings.Repeat(" ", maxWidth-strWidth)
				str += pad
			}
		} else {
			str = f.Format(c)
		}

		fmt.Print(str)
	}

	fmt.Print("\n")
}

func (t table) printHead(column ...string) {
	if len(column) != len(t.widths) {
		panic("supplied columns don't match expected size")
	}

	f := fancy.Print{}
	f.Bold()
	for i, c := range column {
		maxWidth := t.widths[i]
		strWidth := len([]rune(c))
		str := ""

		if maxWidth != -1 {
			if strWidth > maxWidth {
				str = f.Format(c[:maxWidth-3]) + "..."
				strWidth = maxWidth
			} else {
				str = f.Format(c)
			}

			if strWidth < maxWidth {
				pad := strings.Repeat(" ", maxWidth-strWidth)
				str += pad
			}
		} else {
			str = f.Format(c)
		}

		fmt.Print(str)
	}

	fmt.Print("\n")

	for i := range column {
		maxWidth := t.widths[i]
		str := column[i]
		strWidth := len([]rune(str))

		if maxWidth < 0 {
			maxWidth = strWidth
		}

		pad := strings.Repeat("=", strWidth)
		pad += strings.Repeat(" ", maxWidth-strWidth)
		fmt.Print(pad)
	}
	fmt.Print("\n")
}
