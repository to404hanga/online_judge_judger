package service

import (
	"context"

	ojmodel "github.com/to404hanga/online_judge_common/model"
)

type JudgeTask struct {
	SourceCode  string
	Language    ojmodel.SubmissionLanguage
	TimeLimit   int    // Milliseconds
	MemoryLimit int    // Megabytes
	ProblemID   uint64 // 题目ID
}

type CompileResult struct {
	Success      bool
	ErrorMessage string // Stderr from compiler
	OutputPath   string // Path to the compiled artifact
}

type ExecuteResult struct {
	Result     ojmodel.SubmissionResult
	Stderr     string
	TimeUsed   int64 // Milliseconds
	MemoryUsed int64 // Kilobytes
}

type Executor interface {
	Compile(ctx context.Context, task *JudgeTask) (*CompileResult, error)
	Execute(ctx context.Context, task *JudgeTask, testcasePath, compiledArtifactPath string) (*ExecuteResult, error)
	Close(ctx context.Context) error
}
