package detector

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	codexTitle      = regexp.MustCompile(`^(Would you like to (run the following command|make the following edits|grant these permissions|send input to (the existing terminal|terminal [^?]+))\?|Do you want to approve network access to ".+"\?|.+ needs your approval\.?)$`)
	codexOptionLine = regexp.MustCompile(`^(›\s*)?([1-9][0-9]*)\.\s+(.+)$`)
	codexShortcut   = regexp.MustCompile(`^(.*) \(([^()]*)\)$`)

	codexRetryConfirmationBody    = regexp.MustCompile(`^Stopthisattemptandretry\?Thiswillstopthecurrentattemptandretryinanewthread\.Anyfilechangesorotheractionsalreadytakenwillremain\.Yourmessagewillbesentagainusing[^,›]+,whichmaybelesscapableoncomplextasks\.$`)
	codexRetryChoices             = regexp.MustCompile(`^(›?)1\.Retrywithafastermodel(›?)2\.Dismissandkeepwaiting(›?)3\.LearnmoreNoactionisrequired\.Codexwillkeepwaiting,andthismenuwillclosewhentheresponseisready\.$`)
	codexRetryConfirmationChoices = regexp.MustCompile(`^(›?)1\.Keepwaiting(›?)2\.Stopandretry$`)
	// Footer spaces carry no command semantics. Ignore them to also recognize
	// a terminal hard-wrap in the middle of "cancel" or "thread".
	codexFooterLine = regexp.MustCompile(`^Press(?:(.+?)toconfirmor(.+?)tocancel|(.+?)tocancel)(?:or(.+?)toopenthread)?$`)
)

type codexOption struct {
	label    string
	shortcut string
	selected bool
}

// Codex recognizes complete approval and retry menus, not arbitrary questions.
// Approvals require a known single-request option; session/persistent grants
// are never substituted for it. See testdata/codex for the rendering contract.
type Codex struct{}

func (Codex) Detect(screenText string) *MatchResult {
	rawLines := strings.Split(screenText, "\n")
	lines := append([]string(nil), rawLines...)
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	if retry := detectCodexRetryMenu(rawLines, lines); retry != nil {
		return retry
	}

	// The footer must end the visible content. This rejects historical menus
	// followed by output, a composer, or a newer partially rendered request.
	footerIdx := -1
	var footer []string
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "Press ") {
			footerIdx = i
			footer = codexFooterLine.FindStringSubmatch(strings.Join(strings.Fields(strings.Join(lines[i:], " ")), ""))
			break
		}
	}
	if footer == nil {
		return nil
	}

	// Start at the last first option. Numbered command output above the menu
	// cannot be mistaken for the menu's own choices.
	menuIdx := -1
	for i := footerIdx - 1; i >= 0; i-- {
		m := codexOptionLine.FindStringSubmatch(lines[i])
		if m != nil && m[2] == "1" {
			menuIdx = i
			break
		}
	}
	if menuIdx < 0 {
		return nil
	}

	startIdx := -1
	titleBoundary := 0
	for i := 0; i < menuIdx; i++ {
		if strings.HasPrefix(lines[i], "Press ") {
			titleBoundary = i + 1 // never borrow a title across an older dialog's footer
		}
	}
	for i := titleBoundary; i < menuIdx; {
		if lines[i] == "" {
			i++
			continue
		}
		// Codex wraps titles across rows, but separates title and body by a
		// blank line. Anchoring the complete paragraph excludes quoted prose.
		end := i + 1
		for end < menuIdx && lines[end] != "" {
			end++
		}
		if codexTitle.MatchString(strings.Join(lines[i:end], " ")) {
			startIdx = i
		}
		i = end + 1
	}
	if startIdx < 0 {
		return nil
	}

	var options []codexOption
	var body []string
	for _, line := range lines[startIdx:menuIdx] {
		if line != "" {
			body = append(body, line)
		}
	}
	for _, line := range lines[menuIdx:footerIdx] {
		if line == "" {
			continue
		}
		m := codexOptionLine.FindStringSubmatch(line)
		if m != nil {
			n, err := strconv.Atoi(m[2])
			if err != nil || n != len(options)+1 {
				return nil
			}
			options = append(options, codexOption{label: m[3], selected: m[1] != ""})
			body = append(body, m[2]+". "+m[3])
		} else {
			// Wrapped labels and shortcuts belong to the preceding option.
			options[len(options)-1].label += " " + line
			body = append(body, line)
		}
	}
	selected, target, selections := -1, -1, 0
	hasNo := false
	for i := range options {
		o := &options[i]
		if m := codexShortcut.FindStringSubmatch(o.label); m != nil {
			o.label, o.shortcut = m[1], m[2]
		}
		if o.selected {
			selected = i
			selections++
		}
		if o.label == "No" || strings.HasPrefix(o.label, "No, ") {
			hasNo = true
		}
		switch o.label {
		case "Yes, proceed", "Yes, just this once", "Yes, grant these permissions for this turn", "Yes, provide the requested info":
			if target >= 0 {
				return nil
			} // ambiguous affirmative actions
			target = i
		}
	}
	if selections != 1 || target < 0 || !hasNo {
		return nil
	}

	// Codex handles cancellation and per-option shortcuts before list Enter.
	// A visible conflicting binding takes precedence over the footer hint.
	enterConflicts := footer[2] == "enter" || footer[3] == "enter" || footer[4] == "enter"
	for i, o := range options {
		if i != target && o.shortcut == "enter" {
			enterConflicts = true
		}
	}
	response := ""
	if selected == target && footer[1] == "enter" && !enterConflicts {
		response = "\r"
	} else {
		// Approval shortcuts submit directly, regardless of the highlighted
		// option. An extra Enter could accidentally submit the next screen.
		key := options[target].shortcut
		if len(key) != 1 || key[0] < 'a' || key[0] > 'z' {
			return nil
		}
		for i, o := range options {
			if i != target && o.shortcut == key {
				return nil
			}
		}
		if key == footer[2] || key == footer[3] || key == footer[4] {
			return nil
		}
		response = key
	}
	return &MatchResult{
		RuleName: "Codex", Response: response, Hash: hashBody(strings.Join(body, "\n")),
		PromptText: strings.Join(rawLines[startIdx:footerIdx], "\n"),
	}
}

// Retry dialogs use selection lists without the approval footer. Match their
// complete body and exact choices at the end of the screen, including the
// informational footer when present. Whitespace is not semantic in these menus;
// removing it handles both word wrapping and terminal wraps within a word.
func detectCodexRetryMenu(rawLines, lines []string) *MatchResult {
	menuIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if m := codexOptionLine.FindStringSubmatch(lines[i]); m != nil && m[2] == "1" {
			menuIdx = i
			break
		}
	}
	if menuIdx < 0 {
		return nil
	}
	compact := func(rows []string) string {
		return strings.Join(strings.Fields(strings.Join(rows, "")), "")
	}
	choices := compact(lines[menuIdx:])
	for start := menuIdx - 1; start >= 0; start-- {
		if lines[start] == "" {
			continue
		}
		var selections []string
		switch body := compact(lines[start:menuIdx]); {
		case body == "GivingthisrequestalittleextrathoughtIfyou'drathernotwait,retrywithafastermodel.Itmaybelesscapableofhandlingcomplexrequests.":
			selections = codexRetryChoices.FindStringSubmatch(choices)
		case codexRetryConfirmationBody.MatchString(body):
			selections = codexRetryConfirmationChoices.FindStringSubmatch(choices)
		}
		if selections == nil {
			continue
		}
		selected := 0
		for _, marker := range selections[1:] {
			if marker != "" {
				selected++
			}
		}
		if selected != 1 {
			return nil
		}
		// Keep visibility evidence through the last option row, excluding the
		// informational footer. A partial footer redraw must not clear the
		// proxy's duplicate-send suppression while the menu is still visible.
		bodyEnd := menuIdx + 1
		for i := menuIdx + 1; i < len(lines); i++ {
			if codexOptionLine.MatchString(lines[i]) {
				bodyEnd = i + 1
			}
		}
		// ListSelectionView's numeric shortcut selects and accepts option 1
		// directly for both dialogs. Do not append Enter: it could act on the
		// confirmation dialog opened by the first menu.
		body := compact(lines[start:menuIdx]) + strings.ReplaceAll(choices, "›", "")
		return &MatchResult{
			RuleName: "Codex", Response: "1", Hash: hashBody(body),
			PromptText: strings.TrimSpace(strings.Join(rawLines[start:bodyEnd], "\n")),
		}
	}
	return nil
}
