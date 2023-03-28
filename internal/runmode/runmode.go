// Package runmode determines if the vault lambda extension should run in default mode, file mode, or proxy mode.
// default mode: uses both file and proxy mode
// file mode: writes secrets to disk
// proxy mode: forwards requests to a Vault server
package runmode

import "strings"

type Mode string

var (
	ModeDefault Mode = "default"
	ModeFile    Mode = "file"
	ModeProxy   Mode = "proxy"
)

var modes = map[Mode]struct{}{
	ModeFile:    {},
	ModeProxy:   {},
	ModeDefault: {},
}

func ParseMode(rm string) Mode {
	_, ok := modes[Mode(strings.ToLower(rm))]
	if !ok {
		return ModeDefault
	}

	return Mode(rm)
}

func (m Mode) HasModeProxy() bool {
	return m == ModeDefault || m == ModeProxy
}

func (m Mode) HasModeFile() bool {
	return m == ModeDefault || m == ModeFile
}
