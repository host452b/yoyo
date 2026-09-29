package agent_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/screen"
)

func TestAFKDefaultEnterSingleField(t *testing.T) {
	prompt := "Field 1/1 (1 required unanswered)\nChoose the next action\n\n" +
		"  1. Continue  Keep working on the task.\n› 2. Stop  Finish this task.\n" +
		"enter to submit | esc to cancel\n"
	for _, width := range []int{40, 60, 80, 120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			scr := screen.New(width, 24)
			scr.Feed([]byte(strings.ReplaceAll(prompt, "\n", "\r\n")))
			if m := agent.KindCodex.AFKDefaultEnter(scr.Snapshot()); m == nil || m.Response != "\r" {
				t.Fatalf("expected highlighted choice Enter: %+v", m)
			}
		})
	}
	for name, text := range map[string]string{
		"missing field":    strings.ReplaceAll(prompt, "Field 1/1 (1 required unanswered)\n", ""),
		"multiple choices": strings.ReplaceAll(prompt, "  1.", "› 1."),
		"number gap":       strings.ReplaceAll(prompt, "2. Stop", "3. Stop"),
		"draft":            "Field 1/1\nEnter a message\n› my draft\nenter to submit | esc to cancel",
		"quoted":           "The documentation shows this:\n" + prompt,
		"code":             "```\n" + prompt + "```",
	} {
		t.Run(name, func(t *testing.T) {
			if m := agent.KindCodex.AFKDefaultEnter(screen.Snapshot{Text: text}); m != nil {
				t.Fatalf("unexpected Enter: %+v", m)
			}
		})
	}
	for _, kind := range []agent.Kind{agent.KindClaude, agent.KindUnknown, agent.KindCursor} {
		if m := kind.AFKDefaultEnter(screen.Snapshot{Text: prompt}); m != nil {
			t.Fatalf("unsupported %s form accepted: %+v", kind, m)
		}
	}
}

func TestAFKDefaultEnterCodexMenus(t *testing.T) {
	for _, fixture := range []string{"exec.txt", "retry/wait.txt", "retry/prompt.txt", "retry/confirmation.txt"} {
		data, err := os.ReadFile("../detector/testdata/codex/" + fixture)
		if err != nil {
			t.Fatal(err)
		}
		if m := agent.KindCodex.AFKDefaultEnter(screen.Snapshot{Text: string(data)}); m == nil || m.Response != "\r" {
			t.Fatalf("%s did not confirm highlighted choice: %+v", fixture, m)
		}
	}
}
