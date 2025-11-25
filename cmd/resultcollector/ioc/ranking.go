package ioc

import (
	"github.com/redis/go-redis/v9"
	ojcservice "github.com/to404hanga/online_judge_controller/service"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"gorm.io/gorm"
)

func InitRankingService(db *gorm.DB, rdb redis.Cmdable, log loggerv2.Logger) ojcservice.RankingService {
	return ojcservice.NewRankingService(db, rdb, log, "")
}
