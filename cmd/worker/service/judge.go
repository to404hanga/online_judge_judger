package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/IBM/sarama"
	"github.com/redis/go-redis/v9"
	ojmodel "github.com/to404hanga/online_judge_common/model"
	ojconstants "github.com/to404hanga/online_judge_common/proto/constants"
	pbjudgeresult "github.com/to404hanga/online_judge_common/proto/gen/judgeresult"
	"github.com/to404hanga/online_judge_common/proto/gen/judgetask"
	"github.com/to404hanga/online_judge_judger/constants"
	"github.com/to404hanga/online_judge_judger/event"
	"github.com/to404hanga/online_judge_judger/executor"
	"github.com/to404hanga/online_judge_judger/executor/service"
	"github.com/to404hanga/pkg404/gotools/retry"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const (
	groupName = "judger_group"
)

type JudgeService struct {
	log                loggerv2.Logger
	db                 *gorm.DB
	rdb                redis.Cmdable
	judger             executor.Judger
	producer           event.Producer
	consumerName       string
	xAutoClaimTimeout  time.Duration
	testcasePathPrefix string
}

func NewJudgeService(log loggerv2.Logger, db *gorm.DB, rdb redis.Cmdable, producer event.Producer, containerPoolSize, compileTimeoutSeconds, defaultMemoryLimitMB, xAutoClaimTimeoutMinutes int, testcasePathPrefix string) *JudgeService {
	hostname, err := os.Hostname()
	if err != nil {
		log.Error("failed to get hostname", logger.Error(err))
		panic(err)
	}
	return &JudgeService{
		log:                log,
		db:                 db,
		rdb:                rdb,
		producer:           producer,
		judger:             executor.NewDockerJudger(log, containerPoolSize, compileTimeoutSeconds, defaultMemoryLimitMB, testcasePathPrefix),
		consumerName:       fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano()),
		xAutoClaimTimeout:  time.Duration(xAutoClaimTimeoutMinutes) * time.Minute,
		testcasePathPrefix: testcasePathPrefix,
	}
}

func (s *JudgeService) Start(ctx context.Context) error {
	s.log.InfoContext(ctx, "Starting judger service",
		logger.String("group", groupName))

	// 启动自动抢占协程：xAutoClaimTimeout 未确认阈值
	go s.autoClaimLoop(ctx, s.xAutoClaimTimeout, 100)

	err := s.rdb.XGroupCreateMkStream(ctx, constants.JudgeTaskKey, groupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			s.judger.Close() // 关闭 judger
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
	var taskData []byte
	switch v := msg.Values["task"].(type) {
	case string:
		taskData = []byte(v)
	case []byte:
		taskData = v
	default:
		s.log.ErrorContext(ctx, "task field is not []byte or string", logger.String("type", fmt.Sprintf("%T", v)))
		return fmt.Errorf("task field is not []byte or string, type: %T", v)
	}

	var task judgetask.JudgeTask
	err := proto.Unmarshal(taskData, &task)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unmarshal task", logger.Error(err))
		return fmt.Errorf("failed to unmarshal task: %w", err)
	}
	ctx = loggerv2.ContextWithFields(ctx, logger.String("RequestID", task.RequestId))

	if err = s.handleJudgeTask(ctx, &task); err != nil {
		s.log.ErrorContext(ctx, "failed to handle judge task", logger.Error(err))
		return fmt.Errorf("failed to handle judge task: %w", err)
	}

	if err = retry.Do(ctx, func() error {
		return s.rdb.XAck(ctx, constants.JudgeTaskKey, groupName, msg.ID).Err()
	}); err != nil {
		s.log.ErrorContext(ctx, "failed to ack message", logger.Error(err))
		return fmt.Errorf("failed to ack message: %w", err)
	}

	s.log.InfoContext(ctx, "Acked message", logger.String("id", msg.ID))
	return nil
}

func (s *JudgeService) handleJudgeTask(ctx context.Context, task *judgetask.JudgeTask) error {
	judgeTask := &service.JudgeTask{
		ProblemID:   task.ProblemId,
		Language:    ojmodel.SubmissionLanguage(task.Language),
		TimeLimit:   int(task.TimeLimit),
		MemoryLimit: int(task.MemoryLimit),
		SourceCode:  task.Code,
	}
	err := s.db.WithContext(ctx).Model(&ojmodel.Submission{}).
		Where("id = ?", task.SubmissionId).
		Update("status", ojmodel.SubmissionStatusJudging).
		Error
	if err != nil {
		// 这里认为状态更新失败也没关系，因为judger会在执行完成后更新状态
		s.log.ErrorContext(ctx, "failed to update judging status", logger.Error(err))
	}

	finalResult, err := s.judger.Run(ctx, judgeTask)
	if err != nil {
		return fmt.Errorf("failed to run judge task: %w", err)
	}

	msg := &pbjudgeresult.JudgeResult{
		SubmissionId: task.SubmissionId,
		Result:       uint32(finalResult.Result),
		TimeUsed:     finalResult.TimeUsed,
		MemoryUsed:   finalResult.MemoryUsed,
		Stderr:       nil,
	}
	if finalResult.Stderr != "" {
		msg.Stderr = &finalResult.Stderr
	}
	val, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal judge result: %w", err)
	}

	_, _, err = s.producer.Produce(ctx, &sarama.ProducerMessage{
		Topic: ojconstants.JudgeResultTopic,
		Value: sarama.ByteEncoder(val),
	})
	if err != nil {
		return fmt.Errorf("failed to produce judge result: %w", err)
	}

	return nil
}

// 自动抢占长时间未确认的消息
func (s *JudgeService) autoClaimLoop(ctx context.Context, minIdle time.Duration, batch int64) {
	stream := constants.JudgeTaskKey
	start := "0-0"
	ticker := time.NewTicker(5 * time.Second) // 扫描间隔可调
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msgs, _, err := s.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
				Stream:   stream,
				Group:    groupName,
				Consumer: s.consumerName,
				MinIdle:  minIdle,
				Start:    start,
				Count:    batch,
				// JustID: false  // 如只需要ID可置true，但建议直接拿到payload
			}).Result()
			if err != nil && err != redis.Nil {
				s.log.ErrorContext(ctx, "XAUTOCLAIM failed",
					logger.Error(err),
					logger.String("stream", stream),
					logger.String("group", groupName))
				continue
			}

			for _, m := range msgs {
				// 直接处理被抢占到的消息
				if err := s.processMessage(ctx, &m); err != nil {
					s.log.ErrorContext(ctx, "process claimed message failed",
						logger.Error(err), logger.String("id", m.ID))
					// 可选：写入DLQ或记录重试
					continue
				}
				// 异步重试
				retry.Do(ctx, func() error {
					return s.rdb.XAck(ctx, stream, groupName, m.ID).Err()
				}, retry.WithAsync(true), retry.WithCallback(func(err error) {
					s.log.ErrorContext(ctx, "XACK claimed message failed",
						logger.Error(err), logger.String("id", m.ID))
				}))
			}
		}
	}
}
