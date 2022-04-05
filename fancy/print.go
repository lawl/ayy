package fancy

import (
	"os"
	"strconv"
	"strings"
)

//docs: https://gist.github.com/fnky/458719343aabd01cfb17a3a4f7296797

type Print struct {
	formatStack []string
	resetStack  []string

	checkedShouldColorize bool
	shouldColorize        bool
}

func (p *Print) Format(str string) string {
	if !p.checkedShouldColorize {
		fi, _ := os.Stdout.Stat()

		if (fi.Mode() & os.ModeCharDevice) != 0 {
			p.shouldColorize = true
		}
	}
	if !p.shouldColorize {
		return str
	}
	var sb strings.Builder
	for _, f := range p.formatStack {
		sb.WriteString(f)
	}
	sb.WriteString(str)
	for i := len(p.resetStack) - 1; i >= 0; i-- {
		sb.WriteString(p.resetStack[i])
	}

	return sb.String()
}

func (p *Print) Bold() *Print {
	p.formatStack = append(p.formatStack, "\033[1m")
	p.resetStack = append(p.resetStack, "\033[22m")
	return p
}

func (p *Print) Dim() *Print {
	p.formatStack = append(p.formatStack, "\033[2m")
	p.resetStack = append(p.resetStack, "\033[22m")
	return p
}

func (p *Print) Italic() *Print {
	p.formatStack = append(p.formatStack, "\033[3m")
	p.resetStack = append(p.resetStack, "\033[23m")
	return p
}

func (p *Print) Underline() *Print {
	p.formatStack = append(p.formatStack, "\033[4m")
	p.resetStack = append(p.resetStack, "\033[24m")
	return p
}

func (p *Print) Blinking() *Print {
	p.formatStack = append(p.formatStack, "\033[5m")
	p.resetStack = append(p.resetStack, "\033[25m")
	return p
}

func (p *Print) Strikethrough() *Print {
	p.formatStack = append(p.formatStack, "\033[9m")
	p.resetStack = append(p.resetStack, "\033[29m")
	return p
}

const (
	Black = iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
	_
	Default
)

func (p *Print) Color(color int) *Print {
	str := "3" //foreground prefix

	// let's not pull in strconv for single digit int to ascii
	var r rune
	r = '0'
	r += rune(color)
	str += string(r)
	p.formatStack = append(p.formatStack, "\033["+str+"m")
	p.resetStack = append(p.resetStack, "\033[39m")

	return p
}

func (p *Print) Background(color int) *Print {
	str := "4" //background prefix

	// let's not pull in strconv for single digit int to ascii
	var r rune
	r = '0'
	r += rune(color)
	str += string(r)
	p.formatStack = append(p.formatStack, "\033["+str+"m")
	p.resetStack = append(p.resetStack, "\033[39m")
	return p
}

func CursorUp(n int) {
	os.Stdout.WriteString("\033[" + strconv.Itoa(n) + "A")
}

func CursorDown(n int) {
	os.Stdout.WriteString("\033[" + strconv.Itoa(n) + "B")
}

func CursorColumn(n int) {
	os.Stdout.WriteString("\033[" + strconv.Itoa(n) + "G")
}

func CursorSave() {
	os.Stdout.WriteString("\033[s")
}

func CursorRestore() {
	os.Stdout.WriteString("\033[u")
}

func EraseRemainingLine() {
	os.Stdout.WriteString("\033[K")
}

func itoa(singleDigit int) string {
	var r rune
	r = '0'
	r += rune(singleDigit)
	return string(r)
}
