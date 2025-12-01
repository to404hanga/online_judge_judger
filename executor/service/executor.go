package service

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/edsrzf/mmap-go"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	ojmodel "github.com/to404hanga/online_judge_common/model"
	"github.com/to404hanga/online_judge_judger/executor/config"
	"github.com/to404hanga/pkg404/logger"
	loggerv2 "github.com/to404hanga/pkg404/logger/v2"
)

type DockerExecutor struct {
	client             *client.Client
	log                loggerv2.Logger
	containerPool      map[ojmodel.SubmissionLanguage]chan string
	containerPoolSize  int
	compileTimeout     time.Duration // 不建议低于 10s, 经测试编译 go 代码需要将近 9s
	defaultMemoryLimit int64
}

func NewDockerExecutor(log loggerv2.Logger, containerPoolSize, compileTimeoutSeconds, defaultMemoryLimitMB int) Executor {
	c, err := client.New(client.WithAPIVersionNegotiation())
	if err != nil {
		log.Error("Failed to create docker client", logger.Error(err))
		panic(err)
	}
	_, err = c.Ping(context.Background(), client.PingOptions{})
	if err != nil {
		log.Error("Failed to ping docker daemon", logger.Error(err))
		panic(err)
	}
	return &DockerExecutor{
		client:             c,
		log:                log,
		containerPool:      make(map[ojmodel.SubmissionLanguage]chan string),
		containerPoolSize:  containerPoolSize,
		compileTimeout:     time.Duration(compileTimeoutSeconds) * time.Second,
		defaultMemoryLimit: int64(defaultMemoryLimitMB) * 1024 * 1024,
	}
}

func (e *DockerExecutor) Compile(ctx context.Context, task *JudgeTask) (*CompileResult, error) {
	// 获取对应编程语言的配置信息
	cfg, ok := config.LanguageConfigs[task.Language]
	if !ok {
		return &CompileResult{Success: false, ErrorMessage: fmt.Sprintf("unsupported language: %v", task.Language)}, nil
	}

	// 确保Docker镜像已准备就绪
	if err := e.ensureImage(ctx, cfg.ImageName); err != nil {
		return nil, fmt.Errorf("ensure image failed: %w", err)
	}

	// 获取可用的工作容器（worker）
	workerID, err := e.acquireWorker(ctx, task.Language, cfg.ImageName, 512) // 编译容器限制 512 MB
	if err != nil {
		return nil, fmt.Errorf("acquire worker failed: %w", err)
	}
	healthy := true
	defer func() {
		// 任务完成后释放工作容器，healthy标记容器是否正常工作
		e.releaseWorker(ctx, task.Language, workerID, healthy, cfg.ImageName)
	}()

	// 设置容器内的工作目录
	workDirInContainer := "/app"
	// 将源代码文件复制到容器内
	if err = e.copyFileToContainer(ctx, workerID, workDirInContainer, cfg.SourceFileName, []byte(task.SourceCode)); err != nil {
		healthy = false
		return nil, fmt.Errorf("copy source failed: %w", err)
	}

	// 如果是解释型语言（无编译命令），直接复制源文件作为编译结果
	if len(cfg.BuildCommand) == 0 {
		hostOutDir, err := os.MkdirTemp("", "oj-artifact-*")
		if err != nil {
			healthy = false
			return nil, fmt.Errorf("create artifact dir failed: %w", err)
		}
		srcInContainer := filepath.Join(workDirInContainer, cfg.SourceFileName)
		if err := e.copyFromContainer(ctx, workerID, srcInContainer, hostOutDir); err != nil {
			healthy = false
			return nil, fmt.Errorf("copy interpreted source failed: %w", err)
		}
		return &CompileResult{Success: true, OutputPath: filepath.Join(hostOutDir, cfg.SourceFileName)}, nil
	}

	// 设置编译超时时间
	compileCtx, cancel := context.WithTimeout(ctx, e.compileTimeout)
	defer cancel()

	// 执行编译命令
	stdout, stderr, exitCode, err := e.execWithAttach(compileCtx, workerID, cfg.BuildCommand, workDirInContainer)
	_ = stdout
	if err != nil || exitCode != 0 {
		// 如果编译超时，标记容器不健康
		if err != nil && compileCtx.Err() == context.DeadlineExceeded {
			healthy = false
		}
		return &CompileResult{Success: false, ErrorMessage: stderr}, nil
	}

	// 创建本地临时目录用于存放编译产物
	hostOutDir, err := os.MkdirTemp("", "oj-artifact-*")
	if err != nil {
		healthy = false
		return nil, fmt.Errorf("create artifact dir failed: %w", err)
	}

	// 根据编程语言类型复制不同的编译产物
	switch task.Language {
	case ojmodel.SubmissionLanguageCPP, ojmodel.SubmissionLanguageC, ojmodel.SubmissionLanguageGo:
		// C/C++/Go语言：复制生成的可执行文件main
		binInContainer := filepath.Join(workDirInContainer, "main")
		if err := e.copyFromContainer(ctx, workerID, binInContainer, hostOutDir); err != nil {
			healthy = false
			return nil, fmt.Errorf("copy binary failed: %w", err)
		}
		// 确保复制的可执行文件有执行权限
		mainPath := filepath.Join(hostOutDir, "main")
		if err := os.Chmod(mainPath, 0755); err != nil {
			healthy = false
			return nil, fmt.Errorf("chmod binary failed: %w", err)
		}
		return &CompileResult{Success: true, OutputPath: mainPath}, nil
	case ojmodel.SubmissionLanguageJava:
		// Java语言：复制整个工作目录（包含.class文件）
		if err := e.copyFromContainer(ctx, workerID, workDirInContainer, hostOutDir); err != nil {
			healthy = false
			return nil, fmt.Errorf("copy classes failed: %w", err)
		}
		return &CompileResult{Success: true, OutputPath: hostOutDir}, nil
	default:
		// 其他语言：复制整个工作目录
		return &CompileResult{Success: true, OutputPath: hostOutDir}, nil
	}
}

func (e *DockerExecutor) Execute(ctx context.Context, task *JudgeTask, testcasePath, compiledArtifactPath string) (*ExecuteResult, error) {
	// 获取对应编程语言的配置信息
	cfg, ok := config.LanguageConfigs[task.Language]
	if !ok {
		return &ExecuteResult{Stderr: fmt.Sprintf("unsupported language: %v", task.Language)}, nil
	}

	// 确保Docker镜像已准备就绪
	if err := e.ensureImage(ctx, cfg.ImageName); err != nil {
		return nil, fmt.Errorf("ensure image failed: %w", err)
	}

	// 获取可用的工作容器（worker）
	workerID, err := e.acquireWorker(ctx, task.Language, cfg.ImageName, task.MemoryLimit)
	if err != nil {
		return nil, fmt.Errorf("acquire worker failed: %w", err)
	}
	healthy := true
	defer func() {
		// 任务完成后释放工作容器，healthy标记容器是否正常工作
		e.releaseWorker(ctx, task.Language, workerID, healthy, cfg.ImageName)
		e.log.InfoContext(ctx, "release worker", logger.String("workerID", workerID))
	}()

	// 设置容器内的工作目录
	workDirInContainer := "/app"

	// 检查编译产物的类型（文件或目录）
	fi, err := os.Stat(compiledArtifactPath)
	if err != nil {
		healthy = false
		return nil, fmt.Errorf("stat artifact failed: %w", err)
	}

	// 根据产物类型选择不同的复制策略
	if fi.IsDir() {
		// 如果是目录（如Java的class文件），复制整个目录到容器
		if err = e.copyDirToContainer(ctx, workerID, workDirInContainer, compiledArtifactPath); err != nil {
			healthy = false
			return nil, fmt.Errorf("copy dir artifact failed: %w", err)
		}
	} else {
		// 如果是文件（如可执行文件），复制单个文件到容器
		targetName := filepath.Base(compiledArtifactPath)
		if err = e.copyFileToContainer(ctx, workerID, workDirInContainer, targetName, mustReadFile(compiledArtifactPath)); err != nil {
			healthy = false
			return nil, fmt.Errorf("copy file artifact failed: %w", err)
		}
	}

	// 1. 从executor容器复制测试用例输入文件到执行容器
	inputContent, err := os.ReadFile(testcasePath)
	if err != nil {
		healthy = false
		return nil, fmt.Errorf("read input file failed: %w", err)
	}

	// 将输入文件复制到执行容器
	if err = e.copyFileToContainer(ctx, workerID, "/app", "in.txt", inputContent); err != nil {
		healthy = false
		return nil, fmt.Errorf("copy input file failed: %w", err)
	}

	// 2. 修改运行命令，将标准输入重定向到in.txt
	modifiedRunCommand := make([]string, len(cfg.RunCommand))
	copy(modifiedRunCommand, cfg.RunCommand)

	// 根据不同的语言类型，添加重定向
	switch task.Language {
	case ojmodel.SubmissionLanguageCPP, ojmodel.SubmissionLanguageC, ojmodel.SubmissionLanguageGo:
		// 对于C/C++/Go，直接修改运行命令添加重定向
		modifiedRunCommand = []string{"sh", "-c", fmt.Sprintf("%s < /app/in.txt", cfg.RunCommand[0])}
	case ojmodel.SubmissionLanguageJava:
		// 对于Java，构建完整的重定向命令
		javaCmd := strings.Join(cfg.RunCommand, " ")
		modifiedRunCommand = []string{"sh", "-c", fmt.Sprintf("%s < /app/in.txt", javaCmd)}
	default:
		// 其他语言，默认处理方式
		cmdStr := strings.Join(cfg.RunCommand, " ")
		modifiedRunCommand = []string{"sh", "-c", fmt.Sprintf("%s < /app/in.txt", cmdStr)}
	}

	var expected mmap.MMap
	var expectedErr error
	expectedFilePath := strings.Replace(testcasePath, ".in", ".out", 1)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		var file *os.File
		file, expectedErr = os.OpenFile(expectedFilePath, os.O_RDONLY, 0666)
		if expectedErr != nil {
			return
		}
		defer file.Close()
		expected, expectedErr = mmap.Map(file, mmap.RDONLY, 0)
	}()
	defer func() {
		if expectedErr == nil {
			expected.Unmap()
		}
	}()

	// 设置执行超时时间
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(float64(task.TimeLimit)*1.1)*time.Millisecond) // 10% 的余量防止误判
	defer cancel()

	// 启动内存监控goroutine
	memoryChan := make(chan int64, 1)
	go e.monitorMemoryUsage(runCtx, workerID, memoryChan)

	// 记录开始执行时间
	startAt := time.Now()
	// 在容器中执行程序
	stdout, stderr, _, err := e.execWithAttach(runCtx, workerID, modifiedRunCommand, workDirInContainer)
	// 计算执行耗时
	timeUsed := time.Since(startAt).Milliseconds()

	// 获取最大内存使用量
	var maxMemory = <-memoryChan

	// 构建执行结果
	result := &ExecuteResult{
		Stderr:     stderr,           // 标准错误
		TimeUsed:   timeUsed,         // 执行耗时（毫秒）
		MemoryUsed: maxMemory / 1024, // 内存使用（KB）
	}

	// 如果执行超时，标记运行超时
	if (err != nil && runCtx.Err() == context.DeadlineExceeded) || timeUsed >= int64(task.TimeLimit) {
		result.Result = ojmodel.SubmissionResultTimeLimitExceeded
		return result, nil
	}

	// 如果内存超限, 标记内存超限
	if maxMemory > int64(task.MemoryLimit)*1024*1024 {
		result.Result = ojmodel.SubmissionResultMemoryLimitExceeded
		return result, nil
	}

	wg.Wait()
	if expectedErr != nil {
		result.Result = ojmodel.SubmissionResultRuntimeError
		return result, fmt.Errorf("read expected output file failed: %w", expectedErr)
	}
	if runtime.GOOS == "windows" {
		stdout = strings.ReplaceAll(stdout, "\n", "\r\n")
	}
	e.log.DebugContext(ctx, "compare stdout and expected", logger.String("stdout", stdout), logger.String("expected", string(expected)))
	if bytes.Equal([]byte(stdout), expected) {
		result.Result = ojmodel.SubmissionResultAccepted
	} else {
		result.Result = ojmodel.SubmissionResultWrongAnswer
	}

	return result, nil
}

func (e *DockerExecutor) Close(ctx context.Context) error {
	for _, ch := range e.containerPool {
		close(ch)
		for id := range ch {
			if _, err := e.client.ContainerStop(ctx, id, client.ContainerStopOptions{}); err != nil {
				e.log.ErrorContext(ctx, "stop container failed", logger.String("containerID", id), logger.Error(err))
			}
			if _, err := e.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{}); err != nil {
				e.log.ErrorContext(ctx, "remove container failed", logger.String("containerID", id), logger.Error(err))
			}
		}
	}
	return nil
}

func (e *DockerExecutor) ensureImage(ctx context.Context, image string) error {
	// 首先检查本地是否已存在该镜像
	filters := client.Filters{}
	filters.Add("reference", image)
	images, err := e.client.ImageList(ctx, client.ImageListOptions{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	// 如果本地已存在镜像，直接使用本地镜像
	if len(images.Items) > 0 {
		return nil
	}

	// 本地不存在镜像时才尝试拉取
	e.log.Info("Local image not found, pulling from registry", logger.String("image", image))
	reader, err := e.client.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (e *DockerExecutor) acquireWorker(ctx context.Context, lang ojmodel.SubmissionLanguage, image string, memoryLimitMB int) (string, error) {
	ch, ok := e.containerPool[lang]
	if !ok {
		ch = make(chan string, e.containerPoolSize)
		e.containerPool[lang] = ch
		for i := 0; i < e.containerPoolSize; i++ {
			id, err := e.startWorkerContainer(ctx, image)
			if err != nil {
				return "", fmt.Errorf("start worker failed: %w", err)
			}
			ch <- id
		}
	}
	id := <-ch
	e.updateContainerMemory(ctx, id, memoryLimitMB)
	return id, nil
}

func (e *DockerExecutor) updateContainerMemory(ctx context.Context, containerID string, memoryLimitMB int) error {
	_, err := e.client.ContainerUpdate(ctx, containerID, client.ContainerUpdateOptions{
		Resources: &container.Resources{
			Memory:     int64(memoryLimitMB) * 1024 * 1024,
			MemorySwap: -1,         // 禁用 swap
			NanoCPUs:   1000000000, // 限制为1个CPU
		},
	})
	if err != nil {
		return fmt.Errorf("update container memory failed: %w", err)
	}
	return nil
}

func (e *DockerExecutor) releaseWorker(ctx context.Context, lang ojmodel.SubmissionLanguage, id string, healthy bool, image string) {
	ch := e.containerPool[lang]
	if healthy {
		ch <- id
		return
	}
	_, err := e.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
	if err != nil {
		e.log.ErrorContext(ctx, "remove worker failed", logger.Error(err))
		return
	}
	newID, err := e.startWorkerContainer(ctx, image)
	if err != nil {
		e.log.ErrorContext(ctx, "restart worker failed", logger.Error(err))
		return
	}
	ch <- newID
}

func (e *DockerExecutor) startWorkerContainer(ctx context.Context, image string) (string, error) {
	cfg := &container.Config{
		Image:      image,
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: "/app",
	}
	host := &container.HostConfig{
		Resources: container.Resources{
			Memory:     e.defaultMemoryLimit,
			MemorySwap: -1,         // 禁用 swap
			NanoCPUs:   1000000000, // 限制为1个CPU
		},
	}
	resp, err := e.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     cfg,
		HostConfig: host,
	})
	if err != nil {
		return "", err
	}
	if _, err := e.client.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		_, err := e.client.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		if err != nil {
			e.log.Error("remove worker failed", logger.Error(err))
		}
		return "", err
	}
	return resp.ID, nil
}

func (e *DockerExecutor) copyFileToContainer(ctx context.Context, containerID, containerDir, filename string, content []byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	// 将Windows路径转换为Unix路径，确保Docker容器能正确识别
	unixPath := filepath.Join(containerDir, filename)
	unixPath = filepath.ToSlash(unixPath)
	// 根据文件类型设置权限：如果是可执行文件，设置为755，否则为644
	mode := int64(0644)
	if filename == "main" || filepath.Ext(filename) == "" {
		mode = 0755 // 可执行文件需要执行权限
	}
	hdr := &tar.Header{
		Name: unixPath,
		Mode: mode,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	_, err := e.client.CopyToContainer(ctx, containerID, client.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		DestinationPath:           "/",
		Content:                   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *DockerExecutor) copyDirToContainer(ctx context.Context, containerID, containerDir, hostDir string) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	base := containerDir
	// 将Windows路径转换为Unix路径，确保Docker容器能正确识别
	base = filepath.ToSlash(base)
	err := filepath.Walk(hostDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(hostDir, path)
		if err != nil {
			return err
		}
		// 将相对路径也转换为Unix风格
		rel = filepath.ToSlash(rel)
		target := filepath.Join(base, rel)
		// 确保目标路径也是Unix风格
		target = filepath.ToSlash(target)
		if info.IsDir() {
			hdr := &tar.Header{
				Name:     target + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(hdr)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// 根据文件类型设置权限：如果是可执行文件，设置为755，否则为644
		mode := int64(0644)
		if info.Name() == "main" || filepath.Ext(info.Name()) == "" {
			mode = 0755 // 可执行文件需要执行权限
		}
		hdr := &tar.Header{
			Name: target,
			Mode: mode,
			Size: int64(len(data)),
		}
		if err = tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	_, err = e.client.CopyToContainer(ctx, containerID, client.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		DestinationPath:           "/",
		Content:                   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return err
	}
	return nil
}

// monitorMemoryUsage 监控容器内存使用量
func (e *DockerExecutor) monitorMemoryUsage(ctx context.Context, containerID string, memoryChan chan<- int64) {
	defer close(memoryChan)
	var maxMemory int64
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			memoryChan <- maxMemory
			return
		case <-ticker.C:
			stats, err := e.client.ContainerStats(ctx, containerID, client.ContainerStatsOptions{})
			if err != nil {
				e.log.WarnContext(ctx, "get container stats failed", logger.Error(err))
				continue
			}
			var statsData container.StatsResponse
			if err := json.NewDecoder(stats.Body).Decode(&statsData); err != nil {
				stats.Body.Close()
				e.log.WarnContext(ctx, "decode stats failed", logger.Error(err))
				continue
			}
			stats.Body.Close()
			// 获取当前RSS内存使用量
			currentMemory := statsData.MemoryStats.Stats["rss"]
			if currentMemory > uint64(maxMemory) {
				maxMemory = int64(currentMemory)
			}
		}
	}
}

func (e *DockerExecutor) copyFromContainer(ctx context.Context, containerID, srcPath, hostDestDir string) error {
	resp, err := e.client.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{
		SourcePath: srcPath,
	})
	if err != nil {
		return err
	}
	defer resp.Content.Close()
	tr := tar.NewReader(resp.Content)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(hostDestDir, filepath.Base(hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			data, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *DockerExecutor) execWithAttach(ctx context.Context, containerID string, cmd []string, workDir string) (string, string, int, error) {
	created, err := e.client.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          cmd,
		WorkingDir:   workDir,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", "", -1, err
	}
	attach, err := e.client.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", "", -1, err
	}
	defer attach.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attach.Reader)
		done <- err
	}()

	select {
	case err = <-done:
		if err != nil && err != io.EOF {
			return "", "", -1, err
		}
	case <-ctx.Done():
		return stdoutBuf.String(), stderrBuf.String(), -1, ctx.Err()
	}

	inspect, err := e.client.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), -1, err
	}
	return stdoutBuf.String(), stderrBuf.String(), inspect.ExitCode, nil
}

func mustReadFile(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}
