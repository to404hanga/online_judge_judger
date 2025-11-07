package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/redis/go-redis/v9"
	"github.com/to404hanga/online_judge_common/proto/gen/judgetask"
	"github.com/to404hanga/online_judge_judger/constants"
	"github.com/to404hanga/pkg404/gotools/retry"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

const (
	groupName = "judger_group"
)

type JudgeService struct {
	log          loggerv2.Logger
	rdb          redis.Cmdable
	consumerName string
}

func NewJudgeService(log loggerv2.Logger, rdb redis.Cmdable) *JudgeService {
	hostname, err := os.Hostname()
	if err != nil {
		log.Error("failed to get hostname", logger.Error(err))
		panic(err)
	}
	return &JudgeService{
		log:          log,
		rdb:          rdb,
		consumerName: fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano()),
	}
}

func (s *JudgeService) Start(ctx context.Context) error {
	s.log.InfoContext(ctx, "Starting judger service",
		logger.String("group", groupName))

	err := s.rdb.XGroupCreateMkStream(ctx, constants.JudgeTaskKey, groupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			streamMsg, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    groupName,
				Consumer: s.consumerName,
				Streams:  []string{constants.JudgeTaskKey, ">"}, // > 表示只接收新消息
				Count:    1,                                     // 每次读取 1 条消息
				Block:    time.Second,                           // 阻塞 1 秒
			}).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}
				s.log.ErrorContext(ctx, "failed to read stream message", logger.Error(err))
				time.Sleep(100 * time.Millisecond) // 出错稍作等待
				continue
			}

			for _, stream := range streamMsg {
				for _, msg := range stream.Messages {
					s.log.InfoContext(ctx, "Received message", logger.String("id", msg.ID))
					if err = s.processMessage(ctx, &msg); err != nil {
						s.log.ErrorContext(ctx, "failed to process message", logger.Error(err))
					}
				}
			}
		}
	}
}

func (s *JudgeService) processMessage(ctx context.Context, msg *redis.XMessage) error {
	taskData, ok := msg.Values["task"].([]byte)
	if !ok {
		return fmt.Errorf("task field is not []byte")
	}

	var task judgetask.JudgeTask
	err := proto.Unmarshal(taskData, &task)
	if err != nil {
		return fmt.Errorf("failed to unmarshal task: %w", err)
	}

	if err = s.handleJudgeTask(ctx, &task); err != nil {
		return fmt.Errorf("failed to handle judge task: %w", err)
	}

	if err = retry.Do(ctx, func() error {
		return s.rdb.XAck(ctx, constants.JudgeTaskKey, groupName, msg.ID).Err()
	}); err != nil {
		return fmt.Errorf("failed to ack message: %w", err)
	}

	s.log.InfoContext(ctx, "Acked message", logger.String("id", msg.ID))
	return nil
}

func (s *JudgeService) handleJudgeTask(ctx context.Context, task *judgetask.JudgeTask) error {
	return nil
}
