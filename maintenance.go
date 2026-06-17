package main

// 一次性运维子命令。集成进主程序二进制（与线上同镜像），在容器里执行：
//   docker exec <容器> /new-api fix-video-result-url task_xxx [task_yyy ...]          # dry-run
//   docker exec <容器> /new-api fix-video-result-url --apply task_xxx [task_yyy ...]  # 实际写库
// 本地 SQLite 测试：
//   go run . fix-video-result-url task_xxx
//
// main() 在 InitResources()（DB / options / R2 配置已就绪）之后调用 runMaintenance，
// 命中子命令则执行完毕直接退出，不会启动 Web 服务。

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/storage"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// runMaintenance 根据命令行参数分发运维子命令。
// 返回 true 表示命中了某个子命令（调用方应结束进程、不启动 Web 服务）。
func runMaintenance(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "fix-video-result-url":
		fixVideoResultURL(args[1:])
		return true
	default:
		return false
	}
}

// 兼容新老 ZeroCut 格式 + 火山官方格式，从 task.Data 中解析视频直链。
type storedVideoResult struct {
	Data struct {
		Output struct {
			VideoURL string `json:"video_url"` // ZeroCut 新格式
			URL      string `json:"url"`       // ZeroCut 老格式
		} `json:"output"`
	} `json:"data"`
	Content struct {
		VideoURL string `json:"video_url"` // 火山官方格式
	} `json:"content"`
}

func extractUpstreamVideoURL(raw []byte) string {
	var r storedVideoResult
	if err := common.Unmarshal(raw, &r); err != nil {
		return ""
	}
	for _, u := range []string{
		r.Data.Output.VideoURL,
		r.Data.Output.URL,
		r.Content.VideoURL,
	} {
		if s := strings.TrimSpace(u); s != "" {
			return s
		}
	}
	return ""
}

// fixVideoResultURL 修复因 ZeroCut 上游把成功响应字段从 output.url 改名为
// output.video_url 而导致 ResultURL 落空的视频任务。这些任务已是 SUCCESS 终态，
// 轮询/归档 worker 都不会再处理，但上游直链仍保存在 task.Data 里：从中取出直链，
// 走与正常任务一致的 R2 归档（拿到 cdn 地址，不暴露上游），再回写 ResultURL。
func fixVideoResultURL(args []string) {
	apply := false
	var taskIDs []string
	for _, arg := range args {
		switch {
		case arg == "--apply":
			apply = true
		case strings.HasPrefix(arg, "task_"):
			taskIDs = append(taskIDs, arg)
		default:
			fmt.Printf("忽略无法识别的参数: %s\n", arg)
		}
	}
	if len(taskIDs) == 0 {
		fmt.Println("用法: /new-api fix-video-result-url [--apply] task_xxx [task_yyy ...]")
		return
	}

	mode := "DRY-RUN（不下载/不写库）"
	if apply {
		mode = "APPLY（写库）"
	}
	fmt.Printf("[fix-video-result-url] 模式: %s，目标任务数: %d\n\n", mode, len(taskIDs))

	ctx := context.Background()
	for _, taskID := range taskIDs {
		fmt.Printf("==== %s ====\n", taskID)
		task, exist, err := model.GetByOnlyTaskId(taskID)
		if err != nil {
			fmt.Printf("  ✗ 查询失败: %v\n\n", err)
			continue
		}
		if !exist || task == nil {
			fmt.Printf("  ✗ 任务不存在\n\n")
			continue
		}
		if task.Status != model.TaskStatusSuccess {
			fmt.Printf("  ✗ 任务非 SUCCESS（当前 %s），跳过\n\n", task.Status)
			continue
		}

		upstreamURL := extractUpstreamVideoURL(task.Data)
		if upstreamURL == "" {
			fmt.Printf("  ✗ 无法从 task.Data 提取视频直链，跳过\n\n")
			continue
		}

		willArchive := service.ShouldArchiveVideo(upstreamURL)
		fmt.Printf("  当前 ResultURL: %s\n", task.GetResultURL())
		fmt.Printf("  上游直链:       %s\n", upstreamURL)
		fmt.Printf("  计划:           %s\n", map[bool]string{
			true:  "下载并归档到 R2",
			false: "脱敏后写入上游 URL（未启用 R2 归档）",
		}[willArchive])

		if !apply {
			fmt.Printf("  → DRY-RUN，未做任何下载/写库\n\n")
			continue
		}

		// 计算目标 ResultURL：优先 R2 归档，回退脱敏后的上游 URL
		var finalURL string
		archived := false
		if willArchive {
			r2url, aerr := storage.ArchiveVideoToR2(ctx, task.TaskID, task.UserId, task.SubmitTime, upstreamURL)
			if aerr != nil {
				finalURL = operation_setting.ApplyTaskURLRewrite(upstreamURL)
				fmt.Printf("  ! R2 归档失败(%v)，回退脱敏直链\n", aerr)
			} else {
				finalURL = r2url
				archived = true
			}
		} else {
			finalURL = operation_setting.ApplyTaskURLRewrite(upstreamURL)
		}

		task.PrivateData.ResultURL = finalURL
		// 用 map + 显式列名更新，TaskPrivateData 实现了 driver.Valuer，GORM 写入时
		// 会自动序列化为 JSON；只动 private_data 一列，不碰计费/状态等其它字段。
		if err := model.DB.Model(task).Update("private_data", task.PrivateData).Error; err != nil {
			fmt.Printf("  ✗ 写库失败: %v\n\n", err)
			continue
		}
		fmt.Printf("  ✓ 已更新 ResultURL: %s （archived=%t）\n\n", finalURL, archived)
	}
}
