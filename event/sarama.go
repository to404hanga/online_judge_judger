package event

import (
	"context"

	"github.com/IBM/sarama"
)

type SaramaProducer struct {
	producer sarama.SyncProducer
}

func NewSaramaProducer(producer sarama.SyncProducer) Producer {
	return &SaramaProducer{producer: producer}
}

func (s *SaramaProducer) Produce(ctx context.Context, msg *sarama.ProducerMessage) (int32, int64, error) {
	return s.producer.SendMessage(msg)
}