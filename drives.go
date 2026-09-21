package main

import "os"

// rootCandidates lists potential scan roots for the current platform.
func rootCandidates() []string {
	var out []string
	for _, l := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
		"K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"} {
		root := l + ":\\"
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			out = append(out, root)
		}
	}
	return out
}
