//go:build !windows

package orphan

// loadInstalled on non-Windows platforms has no registry to query, so the
// detector stays unconfigured and IsOrphan always returns nil (no false
// positives). Platform specific logic can be added per-OS later.
func (d *Detector) loadInstalled() {}