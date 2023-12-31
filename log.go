package log

import (
	"context"
	"fmt"
	"log"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InfoLogger infoLogger 记录非错误日志
type InfoLogger interface {
	Info(msg string, fields ...Field)
	Infof(template string, args ...interface{})
	Infow(format string, keysAndValues ...interface{})

	Enable() bool
}

// Logger 日志信息，包括错误日志以及非错误日志
type Logger interface {
	InfoLogger

	Debug(msg string, fields ...Field)
	Debugf(template string, args ...interface{})
	Debugw(format string, keysAndValues ...interface{})

	Warn(msg string, fields ...Field)
	Warnf(templace string, args ...interface{})
	Warnw(format string, keysAndValues ...interface{})

	Error(msg string, fields ...Field)
	Errorf(templace string, args ...interface{})
	Errorw(format string, keysAndValues ...interface{})

	Panic(msg string, fields ...Field)
	Panicf(templace string, args ...interface{})
	Panicw(format string, keysAndValues ...interface{})

	Fatal(msg string, fields ...Field)
	Fatalf(templace string, args ...interface{})
	Fatalw(format string, keysAndValues ...interface{})

	// V  返回一个特定等级level的InfoLogger对象，
	// level值越大代表日志级别越低
	// level需要大于0
	V(level int) InfoLogger

	Write(p []byte) (n int, err error)

	// WithValue 为Logger添加keyvalue对
	WithValue(keysAndValues ...interface{}) Logger

	// WithName 向Logger的添加新的名字元素
	WithName(name string) Logger

	// WithContext 拷贝一个context，并且设置了日志值
	WithContext(ctx context.Context) context.Context

	// Flush 调用底层Core的Sync方法 刷新缓存日志
	Flush()
}

type emptyInfoLogeer struct{}

func (l *emptyInfoLogeer) Info(_ string, _ ...Field)        {}
func (l *emptyInfoLogeer) Infof(_ string, _ ...interface{}) {}
func (l *emptyInfoLogeer) Infow(_ string, _ ...interface{}) {}
func (l *emptyInfoLogeer) Enable() bool                     { return false }

var disabledInfoLogeer = &emptyInfoLogeer{}

// infoLogger
type infoLogger struct {
	level zapcore.Level
	log   *zap.Logger
}

func (l *infoLogger) Info(msg string, fields ...Field) {
	if checkEntry := l.log.Check(l.level, msg); checkEntry != nil {
		checkEntry.Write(fields...)
	}
}
func (l *infoLogger) Infof(template string, args ...interface{}) {
	if checkEntry := l.log.Check(l.level, fmt.Sprintf(template, args...)); checkEntry != nil {
		checkEntry.Write()
	}
}

func (l *infoLogger) Infow(msg string, keyAndValues ...interface{}) {
	if checkEntry := l.log.Check(l.level, msg); checkEntry != nil {
		checkEntry.Write(handleFields(l.log, keyAndValues)...)
	}
}

func (l *infoLogger) Enable() bool {
	return true
}

// handlerFields 将pair对转换成zap.Field数组
func handleFields(l *zap.Logger, args []interface{}, additional ...zap.Field) []zap.Field {
	if len(args) == 0 {
		return additional
	}
	fields := make([]zap.Field, len(args)/2+len(additional))
	for i := 0; i < len(args); {
		if _, ok := args[i].(*zap.Field); ok {
			l.DPanic("Strongly-typed Zap Field pass to logz", zap.Any("zap field", args[i]))
			break
		}
		if i == len(args)-1 {
			l.DPanic("add number of arguments passed as key-value pairs for logging.", zap.Any("ignored key", args[i]))
			break
		}
		key, value := args[i], args[i+1]
		keyStr, isString := key.(string)
		if !isString {
			l.DPanic("non-string key argument passed for logging", zap.Any("invalid key", key))
			break
		}

		fields = append(fields, zap.Any(keyStr, value))
		i += 2
	}

	return fields
}

type zapLogger struct {
	zapLogger *zap.Logger
	infoLogger
}

var (
	std = New(NewOptions())
	mu  sync.Mutex
)

func New(opts *Options) *zapLogger {
	if opts == nil {
		opts = NewOptions()
	}
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(opts.Level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	encodeLevel := zapcore.CapitalLevelEncoder
	if opts.Format == consoleFormat && opts.EnableColor {
		encodeLevel = zapcore.CapitalColorLevelEncoder
	}

	encodeConfig := &zapcore.EncoderConfig{
		MessageKey:     "message",
		LevelKey:       "level",
		TimeKey:        "timestamp",
		NameKey:        "logger",
		CallerKey:      "	caller",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    encodeLevel,
		EncodeTime:     timeEncoder,
		EncodeDuration: milliSecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	loggerConfig := &zap.Config{
		Level:             zap.NewAtomicLevelAt(zapLevel),
		Development:       opts.Development,
		DisableStacktrace: opts.DisableStacktrace,
		DisableCaller:     opts.DisableCaller,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         opts.Format,
		EncoderConfig:    *encodeConfig,
		OutputPaths:      opts.OutputPaths,
		ErrorOutputPaths: opts.ErrorOutputPaths,
	}

	var err error
	l, err := loggerConfig.Build(zap.AddStacktrace(zapcore.PanicLevel), zap.AddCallerSkip(1))
	if err != nil {
		panic(err)
	}
	logger := &zapLogger{
		zapLogger: l.Named(opts.Name),
		infoLogger: infoLogger{
			log:   l,
			level: zapcore.InfoLevel,
		},
	}

	zap.RedirectStdLog(l)

	return logger
}

func StdLogger() *zapLogger {
	return std
}

func StdErrLogger() *log.Logger {
	if std == nil {
		return nil
	}

	if l, err := zap.NewStdLogAt(std.zapLogger, zapcore.ErrorLevel); err == nil {
		return l
	}

	return nil
}

func StdInfoLogger() *log.Logger {
	if std == nil {
		return nil
	}

	if l, err := zap.NewStdLogAt(std.zapLogger, zapcore.InfoLevel); err == nil {
		return l
	}

	return nil
}

// V 返回一个特定等级的InfoLogger
func V(level int) InfoLogger {
	return std.V(level)
}

func (l *zapLogger) V(level int) InfoLogger {
	lvl := zapcore.Level(-1 * level)
	if l.zapLogger.Core().Enabled(lvl) {
		return &infoLogger{
			level: lvl,
			log:   l.zapLogger,
		}
	}

	return disabledInfoLogeer
}

func (l *zapLogger) Write(p []byte) (n int, err error) {
	l.zapLogger.Info(string(p))

	return len(p), nil
}

func WriteValue(keyAndValues ...interface{}) Logger {
	return std.WriteValues(keyAndValues...)
}

func (l *zapLogger) WriteValues(keyAndValues ...interface{}) Logger {
	newLogger := l.zapLogger.With(handleFields(l.zapLogger, keyAndValues)...)

	return NewLogger(newLogger)
}
func WithValue(keysAndValues ...interface{}) Logger {
	return std.WithValue(keysAndValues...)
}
func (l *zapLogger) WithValue(keysAndValues ...interface{}) Logger {
	newLogger := l.zapLogger.With(handleFields(l.zapLogger, keysAndValues)...)

	return NewLogger(newLogger)
}

func WithName(s string) Logger {
	return std.WithName(s)
}

func (l *zapLogger) WithName(s string) Logger {
	newLogger := l.zapLogger.Named(s)

	return NewLogger(newLogger)
}

func Flush() {
	std.Flush()
}

func (l *zapLogger) Flush() {
	_ = l.zapLogger.Sync()
}

// NewLogger create a new logz's Logger
func NewLogger(l *zap.Logger) Logger {
	return &zapLogger{
		zapLogger: l,
		infoLogger: infoLogger{
			level: zapcore.InfoLevel,
			log:   l,
		},
	}
}

func Debug(msg string, fields ...Field) {
	std.zapLogger.Debug(msg, fields...)
}

func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.zapLogger.Debug(msg, fields...)
}

func Debugf(template string, args ...interface{}) {
	std.zapLogger.Sugar().Debugf(template, args...)
}

func (l *zapLogger) Debugf(template string, args ...interface{}) {
	l.zapLogger.Sugar().Debugf(template, args...)
}

func Debugw(msg string, keysAndValues ...interface{}) {
	std.zapLogger.Sugar().Debugw(msg, keysAndValues...)
}

func (l *zapLogger) Debugw(msg string, keysAndValues ...interface{}) {
	l.zapLogger.Sugar().Debugw(msg, keysAndValues...)
}

func Info(msg string, fields ...Field) {
	std.zapLogger.Info(msg, fields...)
}

func (l *zapLogger) Info(msg string, fields ...Field) {
	l.zapLogger.Info(msg, fields...)
}
func Infof(format string, v ...interface{}) {
	std.zapLogger.Sugar().Infof(format, v...)
}
func (l *zapLogger) Infof(template string, args ...interface{}) {
	l.zapLogger.Sugar().Infof(template, args...)
}
func Infow(msg string, keysAndValues ...interface{}) {
	std.zapLogger.Sugar().Infow(msg, keysAndValues...)
}

func (l *zapLogger) Infow(msg string, keysAndValues ...interface{}) {
	l.zapLogger.Sugar().Infow(msg, keysAndValues...)
}

func Warn(msg string, fields ...Field) {
	std.zapLogger.Warn(msg, fields...)
}

func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.zapLogger.Warn(msg, fields...)
}
func Warnf(template string, args ...interface{}) {
	std.zapLogger.Sugar().Warnf(template, args...)
}

func (l *zapLogger) Warnf(template string, args ...interface{}) {
	l.zapLogger.Sugar().Warnf(template, args...)
}

func Warnw(msg string, keysAndValues ...interface{}) {
	std.zapLogger.Sugar().Warnw(msg, keysAndValues...)
}

func (l *zapLogger) Warnw(msg string, keysAndValues ...interface{}) {
	l.zapLogger.Sugar().Warnw(msg, keysAndValues...)
}

func Error(msg string, fields ...Field) {
	std.zapLogger.Error(msg, fields...)
}

func (l *zapLogger) Error(msg string, fields ...Field) {
	l.zapLogger.Error(msg, fields...)
}

func Errorf(template string, args ...interface{}) {
	std.zapLogger.Sugar().Errorf(template, args...)
}

func (l *zapLogger) Errorf(template string, args ...interface{}) {
	l.zapLogger.Sugar().Errorf(template, args...)
}

func Errorw(msg string, keysAndValues ...interface{}) {
	std.zapLogger.Sugar().Errorw(msg, keysAndValues...)
}

func (l *zapLogger) Errorw(msg string, keysAndValues ...interface{}) {
	l.zapLogger.Sugar().Errorw(msg, keysAndValues...)
}

func Panic(msg string, fields ...Field) {
	std.zapLogger.Panic(msg, fields...)
}

func (l *zapLogger) Panic(msg string, fields ...Field) {
	l.zapLogger.Panic(msg, fields...)
}
func Panicf(template string, args ...interface{}) {
	std.zapLogger.Sugar().Panicf(template, args...)
}

func (l *zapLogger) Panicf(template string, args ...interface{}) {
	l.zapLogger.Sugar().Panicf(template, args...)
}
func Panicw(msg string, keysAndValues ...interface{}) {
	std.zapLogger.Sugar().Panicw(msg, keysAndValues...)
}

func (l *zapLogger) Panicw(msg string, keysAndValues ...interface{}) {
	l.zapLogger.Sugar().Panicw(msg, keysAndValues...)
}

func Fatal(msg string, fields ...Field) {
	std.zapLogger.Fatal(msg, fields...)
}

func (l *zapLogger) Fatal(msg string, fields ...Field) {
	l.zapLogger.Fatal(msg, fields...)
}

func Fatalf(template string, args ...interface{}) {
	std.zapLogger.Sugar().Fatalf(template, args...)
}

func (l *zapLogger) Fatalf(template string, args ...interface{}) {
	l.zapLogger.Sugar().Fatalf(template, args...)
}

func Fatalw(msg string, keysAndValues ...interface{}) {
	std.zapLogger.Sugar().Fatalw(msg, keysAndValues...)
}

func (l *zapLogger) Fatalw(msg string, keysAndValues ...interface{}) {
	l.zapLogger.Sugar().Fatalw(msg, keysAndValues...)
}
