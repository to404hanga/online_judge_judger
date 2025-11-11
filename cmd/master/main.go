package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const defaultConfigPath = "./config/config.yaml"

func main() {
	cfile := pflag.String("config", defaultConfigPath, "config file path")
	pflag.Parse()

	viper.SetConfigFile(*cfile)
	err := viper.ReadInConfig()
	if err != nil {
		log.Panicf("read config file failed: %v", err)
	}

	// 构建消费者服务
	consumer := BuildDependency()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err = consumer.Start(ctx)
	if err != nil {
		log.Panicf("start consumer failed: %v", err)
	}
}
