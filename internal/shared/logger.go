package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// GlobalLogger 全局日志实例
	GlobalLogger *zap.Logger
)

// LogConfig 日志配置（与 config.LogConfig 保持一致，避免循环依赖）
type LogConfig struct {
	Level    string // 日志级别：debug / info / warn / error
	FilePath string // 日志文件路径，默认 ./logs/app.log
	Console  bool   // 是否同时输出到控制台
}

// InitLogger 初始化全局日志（使用默认配置）
// 设置环境变量 APP_ENV=dev 可切换为开发模式（彩色、可读格式）
func InitLogger() (*zap.Logger, error) {
	cfg := LogConfig{
		Level:    "info",
		FilePath: "./logs/app.log",
		Console:  true,
	}
	if os.Getenv("APP_ENV") == "dev" {
		cfg.Level = "debug"
	}
	return InitLoggerWithConfig(cfg)
}

// InitLoggerWithConfig 使用指定配置初始化全局日志
// 同时输出到控制台（彩色）和文件（JSON 格式，按天滚动）
func InitLoggerWithConfig(cfg LogConfig) (*zap.Logger, error) {
	// 解析日志级别
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	// 确保日志目录存在
	logPath := cfg.FilePath
	if logPath == "" {
		logPath = "./logs/app.log"
	}
	// 按日期在文件名中插入日期后缀，例如 app.2026-03-29.log
	dir := filepath.Dir(logPath)
	base := filepath.Base(logPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	today := time.Now().Format("2006-01-02")
	datedPath := filepath.Join(dir, fmt.Sprintf("%s.%s%s", name, today, ext))

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 文件 WriteSyncer（追加写入）
	logFile, err := os.OpenFile(datedPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 文件编码器：JSON 格式，UTC+8 时间
	fileEncoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"),
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(fileEncoderCfg),
		zapcore.AddSync(logFile),
		level,
	)

	var cores []zapcore.Core
	cores = append(cores, fileCore)

	// 控制台编码器：彩色可读格式
	if cfg.Console {
		consoleEncoderCfg := zapcore.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.TimeEncoderOfLayout("15:04:05.000"),
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderCfg),
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	core := zapcore.NewTee(cores...)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	GlobalLogger = logger
	logger.Info("日志系统初始化完成",
		zap.String("log_level", level.String()),
		zap.String("log_file", datedPath),
		zap.Bool("console", cfg.Console),
	)
	return logger, nil
}

// GetLogger 获取全局日志实例
func GetLogger() *zap.Logger {
	if GlobalLogger == nil {
		// 如果未初始化，创建一个默认的日志实例（仅控制台）
		logger, _ := InitLogger()
		return logger
	}
	return GlobalLogger
}