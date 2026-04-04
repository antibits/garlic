package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	once         sync.Once
)

// Init initializes the global logger with the given configuration
func Init(debug bool) error {
	var err error
	once.Do(func() {
		var config zap.Config
		if debug {
			config = zap.NewDevelopmentConfig()
		} else {
			config = zap.NewProductionConfig()
		}

		// Customize encoder config for better readability
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		globalLogger, err = config.Build()
		if err != nil {
			return
		}

		// Redirect default zap logger to our logger
		zap.ReplaceGlobals(globalLogger)
	})
	return err
}

// Get returns the global logger
func Get() *zap.Logger {
	if globalLogger == nil {
		// Fallback to default logger if not initialized
		return zap.L()
	}
	return globalLogger
}

// Sync flushes any buffered log entries
func Sync() error {
	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}

// Close closes the logger
func Close() error {
	return Sync()
}

// Debug logs a message at debug level
func Debug(msg string, fields ...zap.Field) {
	globalLogger.Debug(msg, fields...)
}

// Info logs a message at info level
func Info(msg string, fields ...zap.Field) {
	globalLogger.Info(msg, fields...)
}

// Warn logs a message at warn level
func Warn(msg string, fields ...zap.Field) {
	globalLogger.Warn(msg, fields...)
}

// Error logs a message at error level
func Error(msg string, fields ...zap.Field) {
	globalLogger.Error(msg, fields...)
}

// Fatal logs a message at fatal level and exits
func Fatal(msg string, fields ...zap.Field) {
	globalLogger.Fatal(msg, fields...)
}

// DPanic logs a message at DPanic level
func DPanic(msg string, fields ...zap.Field) {
	globalLogger.DPanic(msg, fields...)
}

// Panic logs a message at panic level
func Panic(msg string, fields ...zap.Field) {
	globalLogger.Panic(msg, fields...)
}

// With creates a child logger with the given fields
func With(fields ...zap.Field) *zap.Logger {
	return globalLogger.With(fields...)
}

// New creates a new logger with the given configuration
func New(debug bool) (*zap.Logger, error) {
	var config zap.Config
	if debug {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	// Customize encoder config for better readability
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	return config.Build()
}

// NewConsoleLogger creates a new console-friendly logger
func NewConsoleLogger(debug bool) *zap.Logger {
	config := zap.NewDevelopmentConfig()
	if !debug {
		config = zap.NewProductionConfig()
	}

	// Console-friendly format
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Use console encoder
	config.Encoding = "console"

	logger, err := config.Build()
	if err != nil {
		// Fallback to default
		return zap.NewNop()
	}

	return logger
}

// NewFileLogger creates a new logger that writes to a file
func NewFileLogger(filePath string, debug bool) (*zap.Logger, error) {
	writeSyncer := getLogWriter(filePath)
	encoder := getEncoder()

	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	core := zapcore.NewCore(encoder, writeSyncer, level)
	return zap.New(core, zap.AddCaller()), nil
}

func getLogWriter(filePath string) zapcore.WriteSyncer {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return zapcore.Lock(os.Stdout)
	}
	return zapcore.NewMultiWriteSyncer(zapcore.AddSync(file))
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}
