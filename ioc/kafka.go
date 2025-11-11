package ioc

import (
	"github.com/IBM/sarama"
	"github.com/spf13/viper"
	"github.com/to404hanga/online_judge_judger/config"
)

func InitKafka() sarama.Client {
	var cfg config.KafkaConfig
	err := viper.UnmarshalKey("kafka", &cfg)
	if err != nil {
		panic(err)
	}
	saramaCfg := sarama.NewConfig()
	client, err := sarama.NewClient(cfg.Brokers, saramaCfg)
	if err != nil {
		panic(err)
	}
	return client
}

func InitSyncProducer(client sarama.Client) sarama.SyncProducer {
	p, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		panic(err)
	}
	return p
}

func InitConsumerGroup(client sarama.Client, groupID string) sarama.ConsumerGroup {
	cg, err := sarama.NewConsumerGroupFromClient(groupID, client)
	if err != nil {
		panic(err)
	}
	return cg
}
