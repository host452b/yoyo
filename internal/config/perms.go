// internal/config/perms.go
package config

import (
	"fmt"
	"io"
	"os"
)

// CheckPerms warns (to w) if path is writable by group or other. Callers
// still get the config; this is advisory — but a writable config is a
// privilege-escalation vector because [[rules]] can auto-approve any
// prompt.
//
// Returned bool is true if the file is suspicious (perms too loose).
func CheckPerms(path string, w io.Writer) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false // stat failed, don't warn — Load will surface the error
	}
	mode := info.Mode().Perm()
	looseWrite := mode & 0o022 // group-write OR other-write bit set
	if looseWrite == 0 {
		return false
	}
	fmt.Fprintf(w,
		"yoyo: warning: config file %s has mode %04o — writable by group/other.\n"+
			"yoyo: a writable config allows malicious [[rules]] to auto-approve anything.\n"+
			"yoyo: tighten with: chmod 600 %s\n",
		path, mode, path,
	)
	return true
}
