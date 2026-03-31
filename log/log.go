// Package log 提供基于 zap 的日志封装，支持文件轮转
package log

import (
	"fmt"
	"path/filepath"
	"sync"

	"fkteams/common"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	sugar *zap.SugaredLogger
	once  sync.Once
)

func logger() *zap.SugaredLogger {
	once.Do(func() {
		level := readLevel()
		hook := &lumberjack.Logger{
			Filename:   filepath.Join(common.AppDir(), "log", "fkteams.log"),
			MaxSize:    10,
			MaxBackups: 30,
			MaxAge:     7,
			Compress:   true,
		}
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			CallerKey:      "lineNum",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"),
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		core := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(hook),
			level,
		)
		sugar = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
	})
	return sugar
}

func readLevel() zapcore.Level {
	return zap.DebugLevel
}

// Debug 输出 Debug 级别日志
func Debug(args ...any) { logger().Debug(args...) }

// Info 输出 Info 级别日志
func Info(args ...any) { logger().Info(args...) }

// Warn 输出 Warn 级别日志
func Warn(args ...any) { logger().Warn(args...) }

// Error 输出 Error 级别日志
func Error(args ...any) { logger().Error(args...) }

// Fatal 输出 Fatal 级别日志并退出
func Fatal(args ...any) { logger().Fatal(args...) }

// Debugf 格式化输出 Debug 级别日志
func Debugf(template string, args ...any) { logger().Debugf(template, args...) }

// Infof 格式化输出 Info 级别日志
func Infof(template string, args ...any) { logger().Infof(template, args...) }

// Warnf 格式化输出 Warn 级别日志
func Warnf(template string, args ...any) { logger().Warnf(template, args...) }

// Errorf 格式化输出 Error 级别日志
func Errorf(template string, args ...any) { logger().Errorf(template, args...) }

// Fatalf 格式化输出 Fatal 级别日志并退出
func Fatalf(template string, args ...any) { logger().Fatalf(template, args...) }

// Printf 格式化输出日志（Info 级别）
func Printf(template string, args ...any) { logger().Infof(template, args...) }

// Println 输出日志（Info 级别）
func Println(args ...any) { logger().Info(args...) }

// Print 输出日志（Info 级别）
func Print(args ...any) { logger().Info(args...) }

// Sprintf 格式化字符串（不写入日志）
func Sprintf(template string, args ...any) string { return fmt.Sprintf(template, args...) }
