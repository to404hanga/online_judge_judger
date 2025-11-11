package ioc

import (
	"log"

	"github.com/spf13/viper"
	"github.com/to404hanga/online_judge_judger/config"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

func InitLogger() loggerv2.Logger {
	var cfg config.LoggerConfig
	err := viper.UnmarshalKey(cfg.Key(), &cfg)
	if err != nil {
		log.Panicf("unmarshal logger config fail, err: %v", err)
	}

	l, err := loggerv2.NewZapContextLoggerWithConfig(loggerv2.LoggerConfig{
		Output: loggerv2.OutputConfig{
			Type:           cfg.Type,
			FilePath:       cfg.LogFilePath,
			AutoCreateFile: cfg.AutoCreateFile,
		},
		Development: cfg.Development,
	})
	if err != nil {
		log.Panicf("init logger fail, err: %v", err)
	}
	return l
}
