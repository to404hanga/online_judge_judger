package ioc

import (
	"github.com/IBM/sarama"
	"github.com/to404hanga/online_judge_judger/cmd/master/service"
	"github.com/to404hanga/online_judge_judger/ioc"
)

func InitJudgerMasterConsumerGroup(client sarama.Client) sarama.ConsumerGroup {
	return ioc.InitConsumerGroup(client, service.JudgerMasterSubmissionGroupID)
}
