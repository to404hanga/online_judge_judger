package ioc

import (
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/to404hanga/online_judge_judger/config"
)

func InitRedis() redis.Cmdable {
	var cfg config.RedisConfig
	if err := viper.UnmarshalKey(cfg.Key(), &cfg); err != nil {
		log.Panicf("unmarshal redis config fail, err: %v", err)
	}

	cmd := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DB:       cfg.DB,
		Password: cfg.Password,
	})
	return cmd
}
