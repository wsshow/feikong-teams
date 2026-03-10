package log

import (
	"fmt"
	"path/filepath"
	"sync"

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
			Filename:   filepath.Join("log", "fkteams.log"),
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

func Debug(args ...any)                           { logger().Debug(args...) }
func Info(args ...any)                            { logger().Info(args...) }
func Warn(args ...any)                            { logger().Warn(args...) }
func Error(args ...any)                           { logger().Error(args...) }
func Fatal(args ...any)                           { logger().Fatal(args...) }
func Debugf(template string, args ...any)         { logger().Debugf(template, args...) }
func Infof(template string, args ...any)          { logger().Infof(template, args...) }
func Warnf(template string, args ...any)          { logger().Warnf(template, args...) }
func Errorf(template string, args ...any)         { logger().Errorf(template, args...) }
func Fatalf(template string, args ...any)         { logger().Fatalf(template, args...) }
func Printf(template string, args ...any)         { logger().Infof(template, args...) }
func Println(args ...any)                         { logger().Info(args...) }
func Print(args ...any)                           { logger().Info(args...) }
func Sprintf(template string, args ...any) string { return fmt.Sprintf(template, args...) }
