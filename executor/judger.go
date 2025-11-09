package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ojmodel "github.com/to404hanga/online_judge_common/model"
	"github.com/to404hanga/online_judge_judger/executor/service"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

type Judger interface {
	Run(ctx context.Context, task *service.JudgeTask) (*service.ExecuteResult, error)
	Close(ctx context.Context) error
}

type DockerJudger struct {
	log                loggerv2.Logger
	executor           service.Executor
	testcasePathPrefix string
}

func NewDockerJudger(log loggerv2.Logger, containerPoolSize, compileTimeoutSeconds, defaultMemoryLimitMB int, testcasePathPrefix string) Judger {
	return &DockerJudger{
		log:                log,
		executor:           service.NewDockerExecutor(log, containerPoolSize, compileTimeoutSeconds, defaultMemoryLimitMB),
		testcasePathPrefix: testcasePathPrefix,
	}
}

func (j *DockerJudger) Run(ctx context.Context, task *service.JudgeTask) (*service.ExecuteResult, error) {
	finalResult := &service.ExecuteResult{}
	compileResult, err := j.executor.Compile(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to compile: %w", err)
	}
	if !compileResult.Success {
		finalResult.Result = ojmodel.SubmissionResultCompileError
		finalResult.Stderr = compileResult.ErrorMessage
		return finalResult, nil
	}
	testcaseList, err := j.GetTestcaseList(ctx, task.ProblemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get testcase list: %w", err)
	}
	var executeResult *service.ExecuteResult
	for _, testcase := range testcaseList {
		executeResult, err = j.executor.Execute(ctx, task, testcase, compileResult.OutputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to execute: %w", err)
		}
		finalResult.Result = executeResult.Result
		if executeResult.TimeUsed > finalResult.TimeUsed {
			finalResult.TimeUsed = executeResult.TimeUsed
		}
		if executeResult.MemoryUsed > finalResult.MemoryUsed {
			finalResult.MemoryUsed = executeResult.MemoryUsed
		}
		if executeResult.Result != ojmodel.SubmissionResultAccepted {
			break
		}
	}
	return finalResult, nil
}

func (j *DockerJudger) GetTestcaseList(ctx context.Context, problemID uint64) ([]string, error) {
	var files []string
	err := filepath.WalkDir(fmt.Sprintf("%s/%d", j.testcasePathPrefix, problemID), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.EqualFold(filepath.Ext(path), ".in") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk testcase dir: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no testcase found")
	}
	return files, nil
}

func (j *DockerJudger) Close(ctx context.Context) error {
	return j.executor.Close(ctx)
}
