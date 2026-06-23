package proxy

import "bytes"

// kittyPrefixCodepoint is the Unicode codepoint of the prefix key ('y'), as
// reported by the Kitty keyboard protocol. prefixByte (0x19) is the legacy
// control-byte encoding of the same Ctrl+Y keypress.
const kittyPrefixCodepoint = 'y' // 121

// normalizeKittyPrefix rewrites a leading Kitty keyboard-protocol encoding of
// Ctrl+Y into the legacy prefixByte (0x19).
//
// Kitty-capable terminals (Ghostty, kitty, WezTerm, foot, …) stop sending the
// legacy control byte for Ctrl+Y once the wrapped child enables progressive
// enhancement; they send "CSI 121 ; <mods> u" instead. yoyo's prefix state
// machine only understands the legacy byte, so without this the prefix key is
// silently swallowed on those terminals. Normalizing up front lets the rest of
// the input handler work unchanged regardless of the terminal's key encoding.
//
// Only a sequence at the very start of data is considered (the prefix is always
// the first byte of a keypress chunk). A Ctrl+Y release event is dropped to
// mirror legacy mode, which never emits releases. Anything that is not exactly a
// Ctrl+Y key sequence — other keys, incomplete sequences, plain text — is
// returned unchanged.
func normalizeKittyPrefix(data []byte) []byte {
	cp, mods, event, rest, ok := parseKittyKey(data)
	if !ok || cp != kittyPrefixCodepoint || !kittyCtrl(mods) {
		return data
	}
	if event == kittyEventRelease {
		// Legacy mode never emits a release for Ctrl+Y, so consume it.
		return rest
	}
	return append([]byte{prefixByte}, rest...)
}

const (
	kittyEventPress   = 1
	kittyEventRepeat  = 2
	kittyEventRelease = 3
)

// kittyCtrl reports whether the Ctrl modifier is set in a Kitty modifier field.
// Kitty encodes modifiers as a bitmask + 1; the Ctrl bit has value 4. Shift and
// other modifiers are ignored, matching legacy Ctrl+letter (which collapses
// Ctrl and Ctrl+Shift to the same control byte).
func kittyCtrl(mods int) bool {
	return (mods-1)&4 != 0
}

// parseKittyKey parses a leading Kitty key sequence of the form
// "CSI <key>[:alt...] [; <mods>[:<event>]] u". It returns the base key
// codepoint, the modifier field (1 when absent), the event type (press when
// absent), the bytes following the sequence, and ok=false if data does not
// start with a complete, well-formed CSI-u sequence.
func parseKittyKey(data []byte) (cp, mods, event int, rest []byte, ok bool) {
	if len(data) < 3 || data[0] != 0x1b || data[1] != '[' {
		return 0, 0, 0, nil, false
	}
	end := bytes.IndexByte(data[2:], 'u')
	if end < 0 {
		return 0, 0, 0, nil, false
	}
	end += 2
	params := data[2:end]
	rest = data[end+1:]

	// Reject anything that isn't pure CSI-u parameter bytes so we never
	// misread some other escape sequence that happens to contain 'u'.
	for _, b := range params {
		if (b < '0' || b > '9') && b != ';' && b != ':' {
			return 0, 0, 0, nil, false
		}
	}

	groups := bytes.Split(params, []byte{';'})

	keyField := bytes.Split(groups[0], []byte{':'})
	cp, ok = atoiField(keyField[0], 0, false)
	if !ok {
		return 0, 0, 0, nil, false
	}

	mods = 1
	event = kittyEventPress
	if len(groups) >= 2 {
		modField := bytes.Split(groups[1], []byte{':'})
		mods, ok = atoiField(modField[0], 1, true)
		if !ok {
			return 0, 0, 0, nil, false
		}
		if len(modField) >= 2 {
			event, ok = atoiField(modField[1], kittyEventPress, true)
			if !ok {
				return 0, 0, 0, nil, false
			}
		}
	}
	return cp, mods, event, rest, true
}

// atoiField parses a decimal byte field. An empty field yields def (Kitty allows
// omitted sub-parameters to mean their default) only when emptyOK is set.
func atoiField(b []byte, def int, emptyOK bool) (int, bool) {
	if len(b) == 0 {
		if emptyOK {
			return def, true
		}
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
