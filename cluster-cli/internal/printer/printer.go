package printer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
)

var (
	cyan   = color.New(color.FgCyan).SprintFunc()
	green  = color.New(color.FgGreen).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
)

func Header(msg string)  { fmt.Println("\n" + bold(msg)) }
func Info(msg string)    { fmt.Println(cyan("==>") + " " + msg) }
func Success(msg string) { fmt.Println(green("✔") + "  " + msg) }
func Warn(msg string)    { fmt.Println(yellow("⚠") + "  " + msg) }
func Error(msg string)   { fmt.Fprintln(os.Stderr, red("✖")+"  "+msg) }

func Confirm(prompt string) bool {
	return ConfirmFrom(os.Stdin, prompt)
}

func ConfirmFrom(r io.Reader, prompt string) bool {
	fmt.Println()
	fmt.Print("  " + prompt + ": ")
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	fmt.Println()

	switch prompt {
	case "Type 'shutdown' to confirm":
		return answer == "shutdown"
	default:
		return answer == "yes"
	}
}
