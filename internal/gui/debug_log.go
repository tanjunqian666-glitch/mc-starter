// debug_log.go — GUI 调试日志写到桌面，便于排错
package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var debugMu sync.Mutex

func debugLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	debugMu.Lock()
	defer debugMu.Unlock()
	path := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", "starter-debug.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(msg + "\n")
}
