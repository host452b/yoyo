package main

import (
	"fmt"

	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/screen"
)

func main() {
	prompt := "──────────────────────────────────────────\r\n" +
		" Bash command\r\n" +
		"   pip3 show python-dotenv 2>/dev/null || echo \"dotenv not installed\"\r\n" +
		"   Check if python-dotenv is installed\r\n" +
		"\r\n" +
		" Do you want to proceed?\r\n" +
		" ❯ 1. Yes\r\n" +
		"   2. Yes, and don't ask again for: pip3 show *\r\n" +
		"   3. No\r\n" +
		"\r\n" +
		" Esc to cancel · Tab to amend · ctrl+e to explain\r\n"
	scr := screen.New(120, 40)
	scr.Feed([]byte(prompt))
	text := scr.Text()
	fmt.Println("--- screen text ---")
	fmt.Println(text)
	fmt.Println("--- detector ---")
	if r := (detector.Claude{}).Detect(text); r != nil {
		fmt.Printf("MATCH: rule=%s response=%q hash=%s\n", r.RuleName, r.Response, r.Hash[:16])
	} else {
		fmt.Println("NO MATCH")
	}
}
