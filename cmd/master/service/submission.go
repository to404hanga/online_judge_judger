package service

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/gogo/protobuf/proto"
	"github.com/redis/go-redis/v9"
	ojmodel "github.com/to404hanga/online_judge_common/model"
	ojconstants "github.com/to404hanga/online_judge_common/proto/constants"
	"github.com/to404hanga/online_judge_common/proto/gen/judgetask"
	pbsubmission "github.com/to404hanga/online_judge_common/proto/gen/submission"
	"github.com/to404hanga/online_judge_judger/constants"
	"github.com/to404hanga/online_judge_judger/consumer"
	"github.com/to404hanga/pkg404/cachex/lru"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
	"gorm.io/gorm"
)

const (
	problemKey = "problem:%d"
)

type SubmissionService struct {
	log      loggerv2.Logger
	consumer consumer.Consumer
	rdb      redis.Cmdable
	db       *gorm.DB
	lru      *lru.Cache
}

var (
	_ consumer.Consumer = (*SubmissionService)(nil)
)

func NewSubmissionService(log loggerv2.Logger, cg sarama.ConsumerGroup, rdb redis.Cmdable, db *gorm.DB, lru *lru.Cache) *SubmissionService {
	s := &SubmissionService{
		log: log,
		rdb: rdb,
		lru: lru,
		db:  db,
	}
	handler := consumer.NewGroupHandler(s.handleSubmission, log)
	c := consumer.NewSaramaConsumer(cg, ojconstants.SubmissionTopic, handler, log)
	s.consumer = c
	return s
}

func (s *SubmissionService) Start(ctx context.Context) error {
	return s.consumer.Start(ctx)
}

func (s *SubmissionService) handleSubmission(ctx context.Context, msg *sarama.ConsumerMessage) error {
	var pbs pbsubmission.Submission
	if err := proto.Unmarshal(msg.Value, &pbs); err != nil {
		return fmt.Errorf("failed to unmarshal submission: %w", err)
	}

	var submission ojmodel.Submission
	err := s.db.WithContext(ctx).Model(&ojmodel.Submission{}).
		Where("id = ?", pbs.SubmissionId).
		Select("problem_id", "code", "language").
		First(&submission).Error
	if err != nil {
		return fmt.Errorf("failed to get submission: %w", err)
	}

	var problem ojmodel.Problem
	lruKey := fmt.Sprintf(problemKey, submission.ProblemID)
	if problemAny, ok := s.lru.Get(lruKey); ok {
		problem = problemAny.(ojmodel.Problem)
	} else {
		err = s.db.WithContext(ctx).Model(&ojmodel.Problem{}).
			Where("id = ?", submission.ProblemID).
			Select("time_limit", "memory_limit").
			First(&problem).Error
		if err != nil {
			return fmt.Errorf("failed to get problem: %w", err)
		}
		s.lru.Add(lruKey, problem)
	}

	task := &judgetask.JudgeTask{
		SubmissionId: pbs.SubmissionId,
		ProblemId:    submission.ProblemID,
		Code:         submission.Code,
		Language:     int32(submission.Language.Int8()),
		TimeLimit:    int32(problem.TimeLimit),
		MemoryLimit:  int32(problem.MemoryLimit),
	}
	taskBytes, err := proto.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal judge task: %w", err)
	}

	err = s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: constants.JudgeTaskKey,
		Values: map[string]any{
			"task": taskBytes,
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to add judge task to stream: %w", err)
	}
	return nil
}
