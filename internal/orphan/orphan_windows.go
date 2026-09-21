//go:build windows

package orphan

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// loadInstalled gathers installed applications from the Windows registry
// Uninstall keys (HKLM + HKCU, both native and WOW6432Node views).
func (d *Detector) loadInstalled() {
	type root struct {
		key  registry.Key
		path string
	}
	roots := []root{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	}
	for _, r := range roots {
		k, err := registry.OpenKey(r.key, r.path, 0)
		if err != nil {
			continue
		}
		subkeys, _ := k.ReadSubKeyNames(-1)
		_ = k.Close()
		for _, sub := range subkeys {
			sk, err := registry.OpenKey(r.key, r.path+`\`+sub, 0)
			if err != nil {
				continue
			}
			if dn, _, err := sk.GetStringValue("DisplayName"); err == nil && dn != "" {
				d.addInstalledName(dn)
			}
			if il, _, err := sk.GetStringValue("InstallLocation"); err == nil {
				d.addInstalledPath(il)
			}
			if icon, _, err := sk.GetStringValue("DisplayIcon"); err == nil && icon != "" {
				d.addInstalledPath(filepath.Dir(icon))
			}
			_ = sk.Close()
		}
	}

	// Well-known install roots contribute their top-level folders too, so
	// app-data segments match app names even without Uninstall entries.
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			d.indexFolderTree(pf, 2)
		}
	}
}

// indexFolderTree indexes the first depth path segments of an install root.
func (d *Detector) indexFolderTree(dir string, depth int) {
	segs := strings.Split(strings.TrimLeft(filepath.Clean(dir), `\`), `\`)
	if len(segs) > depth {
		segs = segs[:depth]
	}
	for _, s := range segs {
		d.addInstalledPath(s)
	}
}