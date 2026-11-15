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

// 1.9.9 스토어판은 %APPDATA% 에 쓴 파일이 실제로는 Packages\<꾸러미>\LocalCache\Roaming 아래에 놓임.
// 런처가 켠 자바가 그 눈속임 밖에서 돌면 파일을 못 찾아 바로 꺼지므로 (코드 1, 0초) 진짜 경로를 씀.
func packagedDataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	parts := strings.Split(filepath.ToSlash(exe), "/")
	for i, p := range parts {
		if strings.EqualFold(p, "windowsapps") && i+1 < len(parts) {
			f := strings.Split(parts[i+1], "_") // 이름_판_구조_언어_게시자
			if len(f) < 5 {
				return ""
			}
			la := os.Getenv("LOCALAPPDATA")
			if la == "" {
				return ""
			}
			root := filepath.Join(la, "Packages", f[0]+"_"+f[len(f)-1])
			if st, err := os.Stat(root); err != nil || !st.IsDir() {
				return ""
			}
			return filepath.Join(root, "LocalCache", "Roaming", "AsiaWarLauncher")
		}
	}
	return ""
}
