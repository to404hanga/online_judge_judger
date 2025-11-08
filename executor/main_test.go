package executor

import (
	"context"
	"testing"

	ojmodel "github.com/to404hanga/online_judge_common/model"
	"github.com/to404hanga/online_judge_judger/executor/service"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

func TestRun(t *testing.T) {
	l := loggerv2.GetGlobalLogger()
	ctx := context.Background()
	task := &service.JudgeTask{
		SourceCode: `package main

import "fmt"

func main() {
	var n int
	if _, err := fmt.Scan(&n); err != nil {
		return
	}
	fmt.Println(n)
}
`,
		Language:    ojmodel.SubmissionLanguageGo,
		ProblemID:   1,
		TimeLimit:   200,
		MemoryLimit: 256,
	}
	executor := service.NewDockerExecutor(l, 1, 10, 512)
	defer func() {
		err := executor.Close(ctx)
		if err != nil {
			l.ErrorContext(ctx, "Failed to close executor", logger.Error(err))
		}
	}()
	l.InfoContext(ctx, "start")
	var finalResult service.ExecuteResult
	result, err := executor.Compile(ctx, task)
	if err != nil {
		l.ErrorContext(ctx, "Failed to compile", logger.Error(err))
		return
	}
	l.InfoContext(ctx, "compiled")
	if !result.Success {
		finalResult.Result = ojmodel.SubmissionResultCompileError
		finalResult.Stderr = result.ErrorMessage
	} else {
		maxTimeUsed := int64(0)
		maxMemoryUsed := int64(0)
		for i := 1; i <= 3; i++ {
			res, err := executor.Execute(ctx, task, i, result.OutputPath)
			if err != nil {
				l.ErrorContext(ctx, "Failed to execute", logger.Error(err))
			}
			finalResult.Result = res.Result
			if res.TimeUsed > maxTimeUsed {
				maxTimeUsed = res.TimeUsed
			}
			if res.MemoryUsed > maxMemoryUsed {
				maxMemoryUsed = res.MemoryUsed
			}
			if res.Result != ojmodel.SubmissionResultAccepted {
				break
			}
		}
		finalResult.TimeUsed = maxTimeUsed
		finalResult.MemoryUsed = maxMemoryUsed
	}
	l.InfoContext(ctx, "Execute result", logger.Any("result", finalResult))
}

// const (
// 	SubmissionResultUnjudged            SubmissionResult = iota // 未判题 0
// 	SubmissionResultAccepted                                    // Accepted 1
// 	SubmissionResultWrongAnswer                                 // Wrong Answer 2
// 	SubmissionResultCompileError                                // Compile Error 3
// 	SubmissionResultRuntimeError                                // Runtime Error 4
// 	SubmissionResultTimeLimitExceeded                           // Time Limit Exceeded 5
// 	SubmissionResultMemoryLimitExceeded                         // Memory Limit Exceeded 6
// 	SubmissionResultOutputLimitExceeded                         // Output Limit Exceeded 7
// )
