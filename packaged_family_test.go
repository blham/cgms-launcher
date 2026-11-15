package main

import "testing"

func TestPackageFamily(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Program Files\WindowsApps\12345Blham.CGMSLauncher_1.9.9.0_x64__abcd1234efgh5\CGMSLauncher.exe`: "12345Blham.CGMSLauncher_abcd1234efgh5",
		`C:/Program Files/WindowsApps/A.B_1.0.0.0_arm64__zz/x/y.exe`:                                       "A.B_zz",
		`C:\Users\a\AppData\Local\Programs\AsiaWarLauncher\AsiaWarLauncher.exe`:                            "",
		`C:\Program Files\WindowsApps\broken\a.exe`:                                                        "",
	} {
		if got := packageFamily(in); got != want {
			t.Errorf("%s: %q != %q", in, got, want)
		}
	}
}
