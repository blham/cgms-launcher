package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 1.7.9 셰이더팩: 서버가 manifest 의 shaders 로 알려 준 팩을 Modrinth 에서 바로 받아 shaderpacks 폴더에 넣음.
// 서버가 파일을 직접 나눠 주지 않음 (Complementary 라이선스: 모드팩은 Modrinth·CurseForge 를 통해서만).
// 주소는 cdn.modrinth.com 만, 받은 파일은 sha256 이 맞아야 넣음. 처음 받을 때만 Iris 에 그 팩을 골라 켜 두고, 그 뒤엔 본인이 고른 대로.

var shaderURL = regexp.MustCompile(`^https://cdn\.modrinth\.com/data/[A-Za-z0-9]{8}/versions/[A-Za-z0-9]{8}/[A-Za-z0-9._+\-]+\.zip$`)
var shaderFile = regexp.MustCompile(`^[A-Za-z0-9._+\-]+\.zip$`)
var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// manifest 의 셰이더팩을 편의성 모드 목록에 같이 넣음 (화면에서 켜고 끄기). 이상한 항목은 버림
func mergeShaders(m *Manifest) {
	have := map[string]bool{}
	for _, o := range m.Optional {
		have[o.ID] = true
	}
	for _, s := range m.Shaders {
		if s.ID == "" || have[s.ID] || !shaderURL.MatchString(s.URL) || !shaderFile.MatchString(s.File) || !sha256Re.MatchString(s.Sha256) {
			continue
		}
		s.Kind = "shader"
		s.Lib = false
		m.Optional = append(m.Optional, s)
		have[s.ID] = true
	}
}

func shaderDir() string { return filepath.Join(gameDir(), "shaderpacks") }

func syncShaders(man *Manifest, active map[string]bool) {
	dir := shaderDir()
	_ = os.MkdirAll(dir, 0o755)
	managedPath := dirs("managed-shaders.json")
	var old []string
	if b, err := os.ReadFile(managedPath); err == nil {
		_ = json.Unmarshal(b, &old)
	}
	want := map[string]bool{}
	var firstPick string
	for _, s := range man.Optional {
		if s.Kind != "shader" || !active[s.ID] || !shaderURL.MatchString(s.URL) || !shaderFile.MatchString(s.File) {
			continue
		}
		t := dlTask{URL: s.URL, Path: filepath.Join(dir, s.File), Sha256: s.Sha256, Size: s.Size}
		if needDownload(t) {
			if err := downloadAll("셰이더팩 받는 중", []dlTask{t}, 1); err != nil {
				logger.Printf("셰이더팩 %s 를 받지 못했어요: %v", s.File, err)
				continue
			}
		}
		want[s.File] = true
		c := getCfg()
		if !c.ShaderSet[s.ID] && firstPick == "" {
			firstPick = s.File
			cfgMu.Lock()
			if cfg.ShaderSet == nil {
				cfg.ShaderSet = map[string]bool{}
			}
			cfg.ShaderSet[s.ID] = true
			cfgMu.Unlock()
			saveConfig()
		}
	}
	// 예전에 런처가 넣은 팩 중 이제 안 쓰는 것만 지움 (직접 넣은 팩은 그대로). Iris 가 그걸 가리키면 새 팩으로 (없으면 비움)
	var cur string
	for f := range want {
		cur = f
	}
	for _, f := range old {
		if want[f] {
			continue
		}
		if os.Remove(filepath.Join(dir, f)) == nil {
			logger.Printf("예전 셰이더팩 지움: %s", f)
		}
		if irisGet("shaderPack") == f {
			irisSet(map[string]string{"shaderPack": cur})
		}
	}
	var now []string
	for f := range want {
		now = append(now, f)
	}
	b, _ := json.Marshal(now)
	_ = os.WriteFile(managedPath, b, 0o644)
	if firstPick != "" {
		irisSet(map[string]string{"shaderPack": firstPick, "enableShaders": "true"})
		logger.Printf("셰이더팩 %s 를 골라 켰어요 (게임 안에서 K 로 끄고 켬)", firstPick)
	}
}

func irisPath() string { return filepath.Join(gameDir(), "config", "iris.properties") }

func irisGet(key string) string {
	b, err := os.ReadFile(irisPath())
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(l, key+"="))
		}
	}
	return ""
}

// iris.properties 의 몇 줄만 바꿈 (다른 설정은 그대로)
func irisSet(kv map[string]string) {
	p := irisPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	var lines []string
	if b, err := os.ReadFile(p); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	}
	done := map[string]bool{}
	for i, l := range lines {
		t := strings.TrimSpace(l)
		for k, v := range kv {
			if strings.HasPrefix(t, k+"=") {
				lines[i] = k + "=" + v
				done[k] = true
			}
		}
	}
	for k, v := range kv {
		if !done[k] {
			lines = append(lines, k+"="+v)
		}
	}
	out := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	_ = os.WriteFile(p, []byte(out), 0o644)
}
