// Package vmcontrol defines the JSON-RPC protocol shared between foyer
// (the unprivileged web service) and foyer-vm-controller (the small daemon
// that holds libvirt access). The protocol is intentionally tiny: only the
// fields below cross the trust boundary.
package vmcontrol

import "regexp"

const (
	// SocketPath is where the controller listens. Path lives under /run so
	// systemd manages permissions; foyer connects as a client.
	SocketPath = "/run/foyer-vm/sock"

	// MaxRequestBytes caps a single JSON request size on the wire.
	// Real requests are < 100B; anything larger is rejected to prevent
	// memory exhaustion via long names or junk bodies.
	MaxRequestBytes = 1024
)

// Request is the only shape the controller will accept. Extra fields are
// rejected by DisallowUnknownFields on the decoder.
type Request struct {
	Action string `json:"action"`
	VM     string `json:"vm"`
}

// Response from the controller.
type Response struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

// Allowed actions. The controller will reject anything not in this set.
const (
	ActionList     = "list"     // list all defined domains (admin only at the foyer layer)
	ActionInfo     = "info"     // virsh dominfo — state, memory, vcpus
	ActionStats    = "stats"    // virsh domstats + domblkstat + domifstat
	ActionReboot   = "reboot"   // graceful reboot (ACPI)
	ActionShutdown = "shutdown" // graceful shutdown (ACPI)
)

// AllowedActions is the canonical allowlist. Any change here must be reviewed.
var AllowedActions = map[string]struct{}{
	ActionList:     {},
	ActionInfo:     {},
	ActionStats:    {},
	ActionReboot:   {},
	ActionShutdown: {},
}

// vmNameRe is the only character set we accept in VM names. libvirt itself
// is more permissive but we lock to a safe subset to keep our parsing
// trivial and rule out shell metacharacters, path separators, dashes-as-flags
// at the start, etc.
var vmNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)

// ValidVMName returns true if name is safe to pass as a positional arg to
// virsh. Defense in depth: foyer validates this too before sending.
func ValidVMName(name string) bool {
	return vmNameRe.MatchString(name)
}

// IsActionAllowed reports whether action is in the allowlist.
func IsActionAllowed(action string) bool {
	_, ok := AllowedActions[action]
	return ok
}
