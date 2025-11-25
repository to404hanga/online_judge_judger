//go:build wireinject

package main

import (
	"github.com/google/wire"
	iocself "github.com/to404hanga/online_judge_judger/cmd/resultcollector/ioc"
	"github.com/to404hanga/online_judge_judger/cmd/resultcollector/service"
	"github.com/to404hanga/online_judge_judger/ioc"
)

func BuildDependency() *service.ResultCollectorService {
	wire.Build(
		ioc.InitLogger,
		ioc.InitDB,
		ioc.InitKafka,
		iocself.InitResultCollectorConsumerGroup,
		ioc.InitRedis,
		iocself.InitRankingService,
		service.NewResultCollectorService,
	)
	return nil
}
