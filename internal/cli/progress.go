package cli

import (
	"fmt"
	"strings"
	"time"
)

func ProgressBar(percent int) {
	width := 30
	filled := (percent * width) / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	fmt.Printf("\r%s [%s] %d%%", ColorBlue+"Backup"+ColorReset, bar, percent)
	if percent == 100 {
		fmt.Print("\n")
	}
	time.Sleep(40 * time.Millisecond)
}
