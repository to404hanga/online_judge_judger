package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const defaultConfigPath = "./config/config.yaml"

func main() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Panicf("load location failed: %v", err)
	}
	time.Local = loc

	cfile := pflag.String("config", defaultConfigPath, "config file path")
	pflag.Parse()

	viper.SetConfigFile(*cfile)
	err = viper.ReadInConfig()
	if err != nil {
		log.Panicf("read config file failed: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:    ":2112",
		Handler: mux,
	}

	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics server listen failed: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// 构建消费者服务
	consumer := BuildDependency()

	err = consumer.Start(ctx)
	if err != nil {
		log.Panicf("start consumer failed: %v", err)
	}
}
