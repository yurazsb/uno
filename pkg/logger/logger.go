package logger

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Level 日志级别
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// DefaultLogger 默认 Logger
type DefaultLogger struct {
	prefix string
	level  Level
	logger *log.Logger
}

// Default 创建默认 logger，输出到 stdout
func Default(prefix string, level Level) *DefaultLogger {
	return &DefaultLogger{
		prefix: prefix,
		level:  level,
		logger: log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
	}
}

// Output 创建可指定输出的 logger（例如文件）
func Output(prefix string, level Level, output io.Writer) *DefaultLogger {
	return &DefaultLogger{
		prefix: prefix,
		level:  level,
		logger: log.New(output, "", log.LstdFlags|log.Lmicroseconds),
	}
}

func (l *DefaultLogger) Debug(format string, args ...any) {
	if l.shouldLog(DEBUG) {
		l.log("DEBUG", format, args...)
	}
}

func (l *DefaultLogger) Info(format string, args ...any) {
	if l.shouldLog(INFO) {
		l.log("INFO", format, args...)
	}
}

func (l *DefaultLogger) Warn(format string, args ...any) {
	if l.shouldLog(WARN) {
		l.log("WARN", format, args...)
	}
}

func (l *DefaultLogger) Error(format string, args ...any) {
	if l.shouldLog(ERROR) {
		l.log("ERROR", format, args...)
	}
}

func (l *DefaultLogger) shouldLog(level Level) bool {
	return level >= l.level
}

func (l *DefaultLogger) log(level string, format string, args ...interface{}) {
	var msg string
	if len(l.prefix) > 0 {
		msg = fmt.Sprintf("[%s] [%s] %s", level, l.prefix, fmt.Sprintf(format, args...))
	} else {
		msg = fmt.Sprintf("[%s] %s", level, fmt.Sprintf(format, args...))
	}
	l.logger.Println(msg)
}

// SilentLogger 安静的 Logger（不输出任何内容）
type SilentLogger struct{}

func Silent() *SilentLogger {
	return &SilentLogger{}
}

func (s SilentLogger) Debug(format string, args ...any) {}

func (s SilentLogger) Info(format string, args ...any) {}

func (s SilentLogger) Warn(format string, args ...any) {}

func (s SilentLogger) Error(format string, args ...any) {}
