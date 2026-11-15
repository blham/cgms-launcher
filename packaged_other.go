//go:build !windows

package main

func packaged() bool { return false }

func packagedDataDir() string { return "" }
