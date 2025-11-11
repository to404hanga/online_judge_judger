//go:build wireinject

package main

import (
	"github.com/google/wire"
	iocself "github.com/to404hanga/online_judge_judger/cmd/master/ioc"
	"github.com/to404hanga/online_judge_judger/cmd/master/service"
	"github.com/to404hanga/online_judge_judger/ioc"
)

func BuildDependency() *service.SubmissionService {
	wire.Build(
		ioc.InitDB,
		ioc.InitRedis,
		ioc.InitKafka,
		ioc.InitLogger,
		iocself.InitJudgerMasterConsumerGroup,
		iocself.InitLRUCache,

		service.NewSubmissionService,
	)
	return nil
}
