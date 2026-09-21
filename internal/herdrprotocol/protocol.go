// Package herdrprotocol centralizes reviewed Herdr socket protocol compatibility.
package herdrprotocol

// IsReviewed reports whether every Herdr shape consumed by Web TUI was reviewed
// for this protocol. Unknown protocols fail closed until fixtures prove compatibility.
func IsReviewed(protocol uint32) bool {
	switch protocol {
	case 16, 17, 19, 20, 21, 22:
		return true
	default:
		return false
	}
}
