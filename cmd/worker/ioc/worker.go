package ioc

import (
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/to404hanga/online_judge_judger/cmd/worker/config"
	"github.com/to404hanga/online_judge_judger/cmd/worker/service"
	"github.com/to404hanga/online_judge_judger/event"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"gorm.io/gorm"
)

func InitJudgerWorkerService(l loggerv2.Logger, db *gorm.DB, rdb redis.Cmdable, producer event.Producer) *service.JudgeService {
	var cfg config.JudgerWorkerConfig
	err := viper.UnmarshalKey(cfg.Key(), &cfg)
	if err != nil {
		log.Panicf("unmarshal lru config failed, err: %v", err)
	}

	s := service.NewJudgeService(l, db, rdb, producer, cfg.ContainerPoolSize, cfg.CompileTimeoutSeconds, cfg.DefaultMemoryLimitMB, cfg.XAutoClaimTimeoutMinutes, cfg.TestcasePathPrefix)
	return s
}
