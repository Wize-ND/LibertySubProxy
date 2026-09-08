//go:build !linux

// Заглушка демонизации для не-Linux платформ (Windows/macOS для отладки).
// Флаг -d игнорируется, процесс работает на переднем плане.
package main

// daemonize ничего не делает на не-Linux системах.
func daemonize(string) (bool, error) { return true, nil }

// writePID ничего не делает на не-Linux системах.
func writePID(string) error { return nil }
