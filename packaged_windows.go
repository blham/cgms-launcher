//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// 1.9.1 마이크로소프트 스토어판 (MSIX 로 포장된 런처): 스토어가 설치 · 업데이트 · 제거를 맡으므로
// 런처 스스로 설치하거나 새 판으로 바꾸지 않음. 포장된 앱은 Program Files\WindowsApps 아래에서 켜짐.
func packaged() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return strings.Contains(strings.ToLower(filepath.ToSlash(exe)), "/windowsapps/")
}
