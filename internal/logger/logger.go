package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent
)

var (
	mu       sync.RWMutex
	level    = LevelInfo
	verbose  = false
	logFile  io.Writer
	logger   = log.New(os.Stderr, "", log.LstdFlags)
)

// Init 初始化日志系统
func Init(debug bool) {
	mu.Lock()
	defer mu.Unlock()
	verbose = debug
	if debug {
		level = LevelDebug
	}
}

// SetFile 设置日志文件输出
func SetFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	mu.Lock()
	logFile = f
	logger.SetOutput(f)
	mu.Unlock()
	return nil
}

// Debug 调试日志
func Debug(format string, args ...interface{}) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelDebug {
		logger.Printf("[DEBUG] %s", fmt.Sprintf(format, args...))
	}
}

// Info 信息日志
func Info(format string, args ...interface{}) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelInfo {
		logger.Printf("[INFO] %s", fmt.Sprintf(format, args...))
	}
}

// Warn 警告日志
func Warn(format string, args ...interface{}) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelWarn {
		logger.Printf("[WARN] %s", fmt.Sprintf(format, args...))
	}
}

// Error 错误日志
func Error(format string, args ...interface{}) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelError {
		logger.Printf("[ERROR] %s", fmt.Sprintf(format, args...))
	}
}
