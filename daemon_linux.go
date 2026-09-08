//go:build linux

// Демонизация для Linux (Keenetic/Entware): классический double-fork через re-exec.
// Родительский процесс запускает себя же с служебной env-переменной,
// дочерний процесс вызывает setsid и отвязывается от управляющего терминала.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const daemonEnv = "LSP_DAEMON_CHILD"

// daemonize перезапускает процесс в фоне. Возвращает true, если текущий
// процесс — уже демонизированный потомок (можно продолжать работу).
func daemonize(pidFile string) (bool, error) {
	if os.Getenv(daemonEnv) == "1" {
		return true, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("get executable: %w", err)
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), daemonEnv+"=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Setsid: новая сессия, отцепление от терминала и управляющего tty.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start child: %w", err)
	}
	// Не ждём потомка — он переживёт родителя (пере-парантизируется к init).
	_ = cmd.Process.Release()
	return false, nil
}

// writePID атомарно пишет pid текущего процесса в pidFile.
func writePID(pidFile string) error {
	f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open pidfile: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(strconv.Itoa(os.Getpid()) + "\n"); err != nil {
		return fmt.Errorf("write pidfile: %w", err)
	}
	return nil
}
