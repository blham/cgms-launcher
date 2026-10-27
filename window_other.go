//go:build !windows

package main

// 맥·리눅스: 크롬 계열 브라우저의 앱 창 (없으면 기본 브라우저)
func openWindow(url string) { openAppWindow(url) }

func cleanupOldWindowProfile() {}
