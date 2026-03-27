package database

import (
	"context"
	"fmt"
	"time"

	"aiProject/internal/config"
	"aiProject/internal/shared"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
	"git.code.oa.com/trpc-go/trpc-go/client"
	"go.uber.org/zap"

	// 注册 trpc-database/mysql 插件和 dsn selector
	_ "git.code.oa.com/trpc-go/trpc-database/mysql"
	_ "git.code.oa.com/trpc-go/trpc-selector-dsn"
)

const mysqlServiceName = "trpc.mysql.ai_chat.db"

// DB 全局 trpc-database/mysql 客户端
var DB trmysql.Client

// InitMySQL 初始化 MySQL 客户端（通过 trpc-database/mysql）
// cfg 用于构建 DSN，当 trpc_go.yaml 未配置 target 时作为回退
func InitMySQL(cfg *config.MySQLConfig) (trmysql.Client, error) {
	// 构建 DSN target
	dsn := fmt.Sprintf("dsn://%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=5s",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	proxy := trmysql.NewClientProxy(mysqlServiceName,
		client.WithTarget(dsn),
	)

	// 探活：执行一次简单查询验证连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dummy int
	dest := []interface{}{&dummy}
	if err := proxy.QueryRow(ctx, dest, "SELECT 1"); err != nil {
		return nil, fmt.Errorf("mysql 连接探活失败: %v", err)
	}

	shared.GetLogger().Info("MySQL 数据库连接成功",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database),
	)

	DB = proxy
	return proxy, nil
}

// GetDB 获取全局 mysql.Client 实例
func GetDB() trmysql.Client {
	return DB
}

// CloseMySQL trpc-database/mysql 由框架管理连接池，无需手动关闭
func CloseMySQL() error {
	return nil
}