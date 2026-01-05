//go:build linux || darwin

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pranshuparmar/witr/internal/output"
	procpkg "github.com/pranshuparmar/witr/internal/proc"
	"github.com/pranshuparmar/witr/internal/process"
	"github.com/pranshuparmar/witr/internal/source"
	"github.com/pranshuparmar/witr/internal/target"
	"github.com/pranshuparmar/witr/pkg/model"
)

var version = ""
var commit = ""
var buildDate = ""

func printHelp() {
	fmt.Println("Usage: witr [--pid N | --port N | name] [--short] [--tree] [--json] [--warnings] [--no-color] [--env] [--help] [--version]")
	fmt.Println("  --pid <n>         特定的PID")
	fmt.Println("  --port <n>        端口使用情况")
	fmt.Println("  --short           单行摘要输出")
	fmt.Println("  --tree            显示完整的进程树")
	fmt.Println("  --json            以JSON格式输出结果")
	fmt.Println("  --warnings        仅显示警告信息")
	fmt.Println("  --no-color        禁用彩色输出")
	fmt.Println("  --env             仅显示进程的环境变量")
	fmt.Println("  --help            帮助信息")
	fmt.Println("  --version         显示版本并退出")
}

// Helper: which flags need a value (not bool flags)?
func flagNeedsValue(flag string) bool {
	switch flag {
	case "--pid", "-pid", "--port", "-port":
		return true
	}
	return false
}

func main() {
	// Sanity check: fail build if version is not injected
	// if version == "" {
	// 	fmt.Fprintln(os.Stderr, "ERROR: version not set. Use -ldflags '-X main.version=...' when building.")
	// 	os.Exit(2)
	// }
	versionFlag := flag.Bool("version", false, "show version and exit")

	// Reorder os.Args so all flags (with their values) come before positional arguments
	reordered := []string{os.Args[0]}
	var positionals []string
	i := 1
	for i < len(os.Args) {
		arg := os.Args[i]
		if len(arg) > 0 && arg[0] == '-' {
			reordered = append(reordered, arg)
			// If this flag takes a value (not a bool flag), keep the value with it
			if flagNeedsValue(arg) && i+1 < len(os.Args) && os.Args[i+1][0] != '-' {
				reordered = append(reordered, os.Args[i+1])
				i++
			}
		} else {
			positionals = append(positionals, arg)
		}
		i++
	}
	reordered = append(reordered, positionals...)
	os.Args = reordered

	pidFlag := flag.String("pid", "", "pid to explain")
	portFlag := flag.String("port", "", "port to explain")
	shortFlag := flag.Bool("short", false, "short output")
	treeFlag := flag.Bool("tree", false, "tree output")
	jsonFlag := flag.Bool("json", false, "output as JSON")
	warnFlag := flag.Bool("warnings", false, "show only warnings")
	noColorFlag := flag.Bool("no-color", false, "disable colorized output")
	envFlag := flag.Bool("env", false, "show only environment variables for the process")
	helpFlag := flag.Bool("help", false, "show help")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("witr %s (commit %s, built %s)\n", version, commit, buildDate)
		os.Exit(0)
	}
	// To embed version, commit, and build date, use:
	// go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X 'main.buildDate=$(date +%Y-%m-%d)'" -o witr ./cmd/witr
	if *envFlag {
		var t model.Target
		switch {
		case *pidFlag != "":
			t = model.Target{Type: model.TargetPID, Value: *pidFlag}
		case *portFlag != "":
			t = model.Target{Type: model.TargetPort, Value: *portFlag}
		case len(flag.Args()) > 0:
			t = model.Target{Type: model.TargetName, Value: flag.Args()[0]}
		default:
			printHelp()
			os.Exit(1)
		}

		pids, err := target.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(pids) > 1 {
			fmt.Print("发现多个匹配的进程:\n\n")
			for i, pid := range pids {
				cmdline := procpkg.GetCmdline(pid)
				fmt.Printf("[%d] 进程PID %d   %s\n", i+1, pid, cmdline)
			}
			fmt.Println("\n使用以下命令运行:")
			fmt.Println("  witr --pid <进程pid> --env")
			os.Exit(1)
		}
		pid := pids[0]
		procInfo, err := procpkg.ReadProcess(pid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if *jsonFlag {
			type envOut struct {
				Command string   `json:"Command"`
				Env     []string `json:"Env"`
			}
			out := envOut{Command: procInfo.Cmdline, Env: procInfo.Env}
			enc, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(enc))
		} else {
			output.RenderEnvOnly(procInfo, !*noColorFlag)
		}
		return
	}

	if *helpFlag {
		printHelp()
		os.Exit(0)
	}

	var t model.Target

	switch {
	case *pidFlag != "":
		t = model.Target{Type: model.TargetPID, Value: *pidFlag}
	case *portFlag != "":
		t = model.Target{Type: model.TargetPort, Value: *portFlag}
	case len(flag.Args()) > 0:
		t = model.Target{Type: model.TargetName, Value: flag.Args()[0]}
	default:
		printHelp()
		os.Exit(1)
	}

	pids, err := target.Resolve(t)
	if err != nil {
		errStr := err.Error()
		fmt.Println()
		fmt.Println("Error:")
		fmt.Printf("  %s\n", errStr)
		if strings.Contains(errStr, "socket found but owning process not detected") {
			fmt.Println("\n已找到该端口的套接字,但无法检测到拥有该进程的进程。")
			fmt.Println("这可能是由于权限不足。请尝试使用sudo运行:")
			// Print the actual command the user entered, prefixed with sudo
			fmt.Print("  sudo ")
			for i, arg := range os.Args {
				if i > 0 {
					fmt.Print(" ")
				}
				fmt.Print(arg)
			}
			fmt.Println()
		} else {
			fmt.Println("\n未找到匹配的进程或服务。请检查您的查询或尝试不同的名称/端口/进程ID。")
		}
		fmt.Println("查看用法和选项,请运行: witr --help")
		os.Exit(1)
	}

	if len(pids) > 1 {
		fmt.Print("发现多个匹配的进程:\n\n")
		for i, pid := range pids {
			cmdline := procpkg.GetCmdline(pid)
			fmt.Printf("[%d] 进程PID %d   %s\n", i+1, pid, cmdline)
		}
		fmt.Println("\n使用以下命令重新运行:")
		fmt.Println("  witr --pid <进程ID>")
		os.Exit(1)
	}

	pid := pids[0]

	ancestry, err := process.BuildAncestry(pid)
	if err != nil {
		fmt.Println()
		fmt.Println("Error:")
		fmt.Printf("  %s\n", err.Error())
		fmt.Println("\n未找到匹配的进程或服务。请检查您的查询或尝试不同的名称/端口/进程ID。")
		fmt.Println("查看用法和选项,请运行: witr --help")
		os.Exit(1)
	}

	src := source.Detect(ancestry)

	var proc model.Process
	resolvedTarget := "unknown"
	if len(ancestry) > 0 {
		proc = ancestry[len(ancestry)-1]
		resolvedTarget = proc.Command
	}

	// Calculate restart count (consecutive same-command entries)
	restartCount := 0
	lastCmd := ""
	for _, procA := range ancestry {
		if procA.Command == lastCmd {
			restartCount++
		}
		lastCmd = procA.Command
	}

	res := model.Result{
		Target:         t,
		ResolvedTarget: resolvedTarget,
		Process:        proc,
		RestartCount:   restartCount,
		Ancestry:       ancestry,
		Source:         src,
		Warnings:       source.Warnings(ancestry),
	}

	// Add socket state info for port queries
	if t.Type == model.TargetPort {
		portNum := 0
		fmt.Sscanf(t.Value, "%d", &portNum)
		if portNum > 0 {
			res.SocketInfo = procpkg.GetSocketStateForPort(portNum)
		}
	}

	// Add resource context (thermal state, sleep prevention)
	res.ResourceContext = procpkg.GetResourceContext(pid)

	// Add file context (open files, locks)
	res.FileContext = procpkg.GetFileContext(pid)

	if *jsonFlag {
		importJson, _ := output.ToJSON(res)
		fmt.Println(importJson)
	} else if *warnFlag {
		output.RenderWarnings(res.Warnings, !*noColorFlag)
	} else if *treeFlag {
		output.PrintTree(res.Ancestry, !*noColorFlag)
	} else if *shortFlag {
		output.RenderShort(res, !*noColorFlag)
	} else {
		output.RenderStandard(res, !*noColorFlag)
	}
}
