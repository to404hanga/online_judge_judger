package ioc

import (
	"log"

	"github.com/spf13/viper"
	"github.com/to404hanga/online_judge_judger/cmd/master/config"
	"github.com/to404hanga/pkg404/cachex/lru"
)

func InitLRUCache() *lru.Cache {
	var cfg config.LRUConfig
	err := viper.UnmarshalKey(cfg.Key(), &cfg)
	if err != nil {
		log.Panicf("unmarshal lru config failed, err: %v", err)
	}

	cache, err := lru.NewSimpleLRU(cfg.Size)
	if err != nil {
		log.Panicf("init lru failed, err: %v", err)
	}

	return cache
}
