//go:build wireinject

package main

import (
	"github.com/google/wire"
	iocself "github.com/to404hanga/online_judge_judger/cmd/worker/ioc"
	"github.com/to404hanga/online_judge_judger/cmd/worker/service"
	"github.com/to404hanga/online_judge_judger/event"
	"github.com/to404hanga/online_judge_judger/ioc"
)

func BuildDependency() *service.JudgeService {
	wire.Build(
		ioc.InitLogger,
		ioc.InitDB,
		ioc.InitRedis,
		ioc.InitKafka,
		ioc.InitSyncProducer,
		event.NewSaramaProducer,
		iocself.InitJudgerWorkerService,
	)
	return nil
}
