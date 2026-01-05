package source

import (
	"time"

	"github.com/pranshuparmar/witr/pkg/model"
)

func Detect(ancestry []model.Process) model.Source {
	// Prefer supervisor over systemd/launchd if both are present
	if src := detectContainer(ancestry); src != nil {
		return *src
	}
	if src := detectSupervisor(ancestry); src != nil {
		return *src
	}
	if src := detectSystemd(ancestry); src != nil {
		return *src
	}
	if src := detectLaunchd(ancestry); src != nil {
		return *src
	}
	if src := detectCron(ancestry); src != nil {
		return *src
	}
	if src := detectShell(ancestry); src != nil {
		return *src
	}

	return model.Source{
		Type:       model.SourceUnknown,
		Confidence: 0.2,
	}
}

func Warnings(p []model.Process) []string {
	var w []string

	last := p[len(p)-1]

	// Restart count detection (count consecutive same-command entries)
	restartCount := 0
	lastCmd := ""
	for _, proc := range p {
		if proc.Command == lastCmd {
			restartCount++
		}
		lastCmd = proc.Command
	}
	if restartCount > 5 {
		w = append(w, "进程或其祖先重启超五次")
	}

	// Health warnings
	switch last.Health {
	case "zombie":
		w = append(w, "进程是僵尸 (defunct)")
	case "stopped":
		w = append(w, "进程已停止 (T state)")
	case "high-cpu":
		w = append(w, "进程占用高CPU(共超过2小时)")
	case "high-mem":
		w = append(w, "进程正在使用高内存(>1GB RSS)")
	}

	if IsPublicBind(last.BindAddresses) {
		w = append(w, "进程正在监听公共接口")
	}

	if last.User == "root" {
		w = append(w, "")
	}

	if Detect(p).Type == model.SourceUnknown {
		w = append(w, "未检测到管或服理")
	}

	// Warn if process is very old (>90 days)
	if time.Since(last.StartedAt).Hours() > 90*24 {
		w = append(w, "该进程已运行超过90天")
	}

	// Warn if working dir is suspicious
	suspiciousDirs := map[string]bool{"/": true, "/tmp": true, "/var/tmp": true}
	if suspiciousDirs[last.WorkingDir] {
		w = append(w, "进程正在从可疑的工作目录运行: "+last.WorkingDir)
	}

	// Warn if container and no healthcheck (placeholder, as healthcheck not detected)
	if last.Container != "" {
		w = append(w, "未检测到容器的健康状态")
	}

	// Warn if service name and process name mismatch
	if last.Service != "" && last.Command != "" && last.Service != last.Command {
		w = append(w, "服务名称与进程名称不匹配")
	}

	return w
}
