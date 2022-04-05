package fancy

import (
	"fmt"
	"strings"
)

type ProgressBar struct {
	width    int
	maxValue int
}

func NewProgressBar(width, maxValue int) *ProgressBar {
	return &ProgressBar{width: width, maxValue: maxValue}
}

func (p *ProgressBar) Print(value int) {
	percent := float32(value) / float32(p.maxValue)
	charsToPrint := int(float32(p.width) * percent)

	if charsToPrint < p.width {
		//draw arrow at the end fo the bar
		if charsToPrint > 0 {
			charsToPrint--
		}
		bar := strings.Repeat("=", charsToPrint)
		if charsToPrint != p.width {
			bar += ">"
		}

		space := strings.Repeat(" ", p.width-len(bar))
		fmt.Printf("[%s%s] [%.1f%%]", bar, space, percent*100)
	} else {
		bar := strings.Repeat("=", p.width)
		fmt.Printf("[%s] [%.1f%%]", bar, percent*100)
	}
}
