// Package logger 提供全局共享的 SugaredLogger，所有 internal 包直接引用，
// 避免各自 init 导致写到 stderr。server 进程通过 Set() 注入配置好的 logger。
package logger

import "go.uber.org/zap"

var Logger *zap.SugaredLogger

func Set(l *zap.SugaredLogger) { Logger = l }

func init() {
	l, _ := zap.NewProduction()
	Logger = l.Sugar()
}
