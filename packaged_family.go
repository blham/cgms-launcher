package main

import "strings"

// ...\WindowsApps\이름_판_구조_언어_게시자\... -> 이름_게시자 (꾸러미 자료 폴더 이름)
func packageFamily(exe string) string {
	parts := strings.Split(strings.ReplaceAll(exe, "\\", "/"), "/")
	for i, p := range parts {
		if strings.EqualFold(p, "windowsapps") && i+1 < len(parts) {
			f := strings.Split(parts[i+1], "_")
			if len(f) != 5 || f[0] == "" || f[4] == "" {
				return ""
			}
			return f[0] + "_" + f[4]
		}
	}
	return ""
}
