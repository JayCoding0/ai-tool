package shared

import (
	"go.uber.org/zap"
)

var (
	// GlobalLogger 全局日志实例
	GlobalLogger *zap.Logger
)

// InitLogger 初始化全局日志
func InitLogger() (*zap.Logger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}
	GlobalLogger = logger
	return logger, nil
}

// GetLogger 获取全局日志实例
func GetLogger() *zap.Logger {
	if GlobalLogger == nil {
		// 如果未初始化，创建一个默认的日志实例
		logger, _ := zap.NewProduction()
		return logger
	}
	return GlobalLogger
}