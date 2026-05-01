// debug_write.go — 桌面调试日志，非 ignore，正常编译
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func debug_write(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	path := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", "starter-debug.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(msg + "\n")
}
