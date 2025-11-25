package ioc

import (
	"github.com/IBM/sarama"
	"github.com/to404hanga/online_judge_judger/cmd/resultcollector/service"
	"github.com/to404hanga/online_judge_judger/ioc"
)

func InitResultCollectorConsumerGroup(client sarama.Client) sarama.ConsumerGroup {
	return ioc.InitConsumerGroup(client, service.ResultCollectorGroupID)
}
