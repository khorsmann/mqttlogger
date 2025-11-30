package cli

import "fmt"

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

func Success(msg string) {
	fmt.Println(ColorGreen + "✔ " + msg + ColorReset)
}

func Error(msg string) {
	fmt.Println(ColorRed + "✘ " + msg + ColorReset)
}

func Info(msg string) {
	fmt.Println(ColorCyan + "• " + msg + ColorReset)
}

func Bold(msg string) {
	fmt.Println(ColorBold + msg + ColorReset)
}
