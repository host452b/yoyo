package detector_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/screen"
)

func TestCodex_MCPToolApproval(t *testing.T) {
	data, err := os.ReadFile("testdata/codex/mcp/tool-approval.txt")
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(data)
	base := (detector.Codex{}).Detect(prompt)
	if base == nil || base.Response != "1" || base.PromptText == "" {
		t.Fatalf("expected single-call Allow shortcut, got %+v", base)
	}
	for _, kind := range []agent.Kind{agent.KindCodex, agent.KindUnknown} {
		r := kind.Detect(prompt)
		if r == nil || r.RuleName != "Codex" || r.Response != "1" {
			t.Fatalf("%v did not route tool approval to Codex: %+v", kind, r)
		}
	}
	for _, cols := range []int{40, 60, 80, 120} {
		t.Run(fmt.Sprint(cols), func(t *testing.T) {
			scr := screen.New(cols, 60)
			for _, b := range []byte("\x1b[2J\x1b[H\x1b[1m" + strings.ReplaceAll(prompt, "\n", "\r\n") + "\x1b[0m") {
				scr.Feed([]byte{b})
			}
			if r := (detector.Codex{}).Detect(scr.Text()); r == nil || r.Response != "1" {
				t.Fatalf("expected Allow through wrapped terminal: %+v\n%s", r, scr.Text())
			}
		})
	}
	for _, option := range []string{"1", "2", "3", "4"} {
		moved := strings.ReplaceAll(prompt, "› 3.", "  3.")
		moved = strings.ReplaceAll(moved, "  "+option+".", "› "+option+".")
		r := (detector.Codex{}).Detect(moved)
		if r == nil || r.Response != "1" || r.Hash != base.Hash {
			t.Fatalf("cursor on %s changed approval or identity: %+v", option, r)
		}
	}
	answered := strings.ReplaceAll(prompt, " (1 required unanswered)", "")
	if r := (detector.Codex{}).Detect(answered); r == nil || r.Hash != base.Hash || r.PromptText != base.PromptText {
		t.Fatalf("field progress changed request identity: %+v", r)
	}
	for _, replacement := range [][2]string{
		{"maas-jira", "another-server"}, {"jira_get_issue", "another_tool"},
		{"full_text: true", "full_text: false"}, {"full_text: true", "full_text:  true"},
	} {
		r := (detector.Codex{}).Detect(strings.ReplaceAll(prompt, replacement[0], replacement[1]))
		if r == nil || r.Hash == base.Hash {
			t.Fatalf("changed request was deduplicated: %v, %+v", replacement, r)
		}
	}
}

func TestCodex_MCPToolApprovalOptionalPersistence(t *testing.T) {
	for _, grants := range [][]string{
		nil,
		{"Allow for this session  Run the tool and remember this choice for this session."},
		{"Always allow  Run the tool and remember this choice for future tool calls."},
	} {
		options := append([]string{"Allow  Run the tool and continue."}, grants...)
		options = append(options, "Cancel  Cancel this tool call")
		prompt := "Field 1/1\nAllow the test-server MCP server to run tool \"test_tool\"?\n"
		for i, option := range options {
			marker := " "
			if i == len(options)-1 {
				marker = "›"
			}
			prompt += fmt.Sprintf("%s %d. %s\n", marker, i+1, option)
		}
		prompt += "enter to submit | esc to cancel"
		if r := (detector.Codex{}).Detect(prompt); r == nil || r.Response != "1" {
			t.Fatalf("expected Allow with optional grants %v: %+v", grants, r)
		}
	}
}

func TestCodex_MCPToolApprovalRejectsIncompleteOrUnrelatedMenus(t *testing.T) {
	data, err := os.ReadFile("testdata/codex/mcp/tool-approval.txt")
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(data)
	for name, input := range map[string]string{
		"no progress":      prompt[strings.Index(prompt, "\n")+1:],
		"multiple fields":  strings.ReplaceAll(prompt, "Field 1/1", "Field 1/2"),
		"unrelated title":  strings.ReplaceAll(prompt, "Allow the maas-jira MCP server to run tool \"jira_get_issue\"?", "Choose the default permission policy"),
		"quoted title":     strings.ReplaceAll(prompt, "Allow the maas-jira", "The documentation says: Allow the maas-jira"),
		"no footer":        strings.Split(prompt, "  enter to submit")[0],
		"partial footer":   strings.ReplaceAll(prompt, "esc to cancel", "esc to can"),
		"changed binding":  strings.ReplaceAll(prompt, "enter to submit", "enter to cancel"),
		"no cursor":        strings.ReplaceAll(prompt, "›", " "),
		"two cursors":      strings.ReplaceAll(prompt, "    1.", "  › 1."),
		"number gap":       strings.ReplaceAll(prompt, "4. Cancel", "5. Cancel"),
		"persistent only":  strings.ReplaceAll(prompt, "1. Allow ", "1. Always allow "),
		"changed action":   strings.ReplaceAll(prompt, "Run the tool and continue.", "Run all tools and continue."),
		"no cancel":        strings.ReplaceAll(prompt, "    4. Cancel                  Cancel this tool call\n", ""),
		"stale":            prompt + "Working...\n",
		"new partial form": prompt + "Field 1/1\nAllow the next-server MCP server",
		"quoted menu":      "```text\n" + prompt + "```\n",
	} {
		t.Run(name, func(t *testing.T) {
			if r := (detector.Codex{}).Detect(input); r != nil {
				t.Fatalf("unexpected approval: %+v", r)
			}
		})
	}
}
