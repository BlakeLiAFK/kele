package prompt

import (
	"strings"
	"testing"
)

func TestBuild_ToolNames(t *testing.T) {
	result := Build(BuildParams{
		ToolNames: []string{"bash", "read", "write"},
	})

	for _, name := range []string{"bash", "read", "write"} {
		if !strings.Contains(result, "**"+name+"**") {
			t.Errorf("输出应包含工具名 %q", name)
		}
	}
}

func TestBuild_ToolDescriptions(t *testing.T) {
	result := Build(BuildParams{
		ToolNames: []string{"bash", "send_message", "cron_create"},
	})

	// 验证已知工具有正确描述
	if !strings.Contains(result, "执行 shell 命令") {
		t.Error("bash 工具应包含描述")
	}
	if !strings.Contains(result, "发送消息到 Telegram") {
		t.Error("send_message 工具应包含描述")
	}
}

func TestBuild_UnknownTool(t *testing.T) {
	result := Build(BuildParams{
		ToolNames: []string{"unknown_tool"},
	})

	if !strings.Contains(result, "（无描述）") {
		t.Error("未知工具应显示无描述")
	}
}

func TestBuild_WorkDir(t *testing.T) {
	result := Build(BuildParams{
		WorkDir: "/home/user/project",
	})

	if !strings.Contains(result, "/home/user/project") {
		t.Error("输出应包含工作目录路径")
	}
	if !strings.Contains(result, "相对路径基于此目录") {
		t.Error("输出应包含路径说明")
	}
}

func TestBuild_WorkspaceName(t *testing.T) {
	result := Build(BuildParams{
		WorkDir:       "/home/user/project",
		WorkspaceName: "my-project",
	})

	if !strings.Contains(result, "my-project") {
		t.Error("输出应包含工作空间名")
	}
}

func TestBuild_WorkspaceNameWithoutWorkDir(t *testing.T) {
	// WorkDir 为空时，WorkspaceName 不应出现
	result := Build(BuildParams{
		WorkspaceName: "my-project",
	})

	if strings.Contains(result, "my-project") {
		t.Error("无工作目录时不应显示工作空间名")
	}
}

func TestBuild_Memories(t *testing.T) {
	result := Build(BuildParams{
		Memories: []string{"用户偏好 Vim", "项目使用 Go 1.22"},
	})

	if !strings.Contains(result, "用户长期记忆") {
		t.Error("输出应包含记忆标题")
	}
	if !strings.Contains(result, "用户偏好 Vim") {
		t.Error("输出应包含第一条记忆")
	}
	if !strings.Contains(result, "项目使用 Go 1.22") {
		t.Error("输出应包含第二条记忆")
	}
}

func TestBuild_InjectedContext(t *testing.T) {
	result := Build(BuildParams{
		InjectedCtx: "这是一个 Go 微服务项目",
	})

	if !strings.Contains(result, "工作区上下文") {
		t.Error("输出应包含上下文标题")
	}
	if !strings.Contains(result, "这是一个 Go 微服务项目") {
		t.Error("输出应包含注入的上下文内容")
	}
}

func TestBuild_EmptyParams(t *testing.T) {
	// 空参数不应 panic
	result := Build(BuildParams{})

	if result == "" {
		t.Error("空参数也应返回基础 prompt")
	}
	if !strings.Contains(result, "Kele") {
		t.Error("输出应包含身份标识")
	}
}

func TestBuild_MustCallTools(t *testing.T) {
	result := Build(BuildParams{})

	if !strings.Contains(result, "必须调用工具") {
		t.Error("输出应包含必须调用工具的指导语")
	}
}

func TestBuild_CronLimitations(t *testing.T) {
	result := Build(BuildParams{})

	if !strings.Contains(result, "定时任务注意事项") {
		t.Error("输出应包含 cron 限制说明标题")
	}
	if !strings.Contains(result, "无法调用 send_message") {
		t.Error("输出应包含 cron 不能调用工具的说明")
	}
	if !strings.Contains(result, "curl") {
		t.Error("输出应包含 curl 替代方案")
	}
}

func TestBuild_EmptyMemories(t *testing.T) {
	result := Build(BuildParams{
		Memories: []string{},
	})

	if strings.Contains(result, "用户长期记忆") {
		t.Error("空记忆列表不应显示记忆标题")
	}
}

func TestBuild_EmptyInjectedCtx(t *testing.T) {
	result := Build(BuildParams{
		InjectedCtx: "",
	})

	if strings.Contains(result, "工作区上下文") {
		t.Error("空上下文不应显示上下文标题")
	}
}

func TestBuild_FullParams(t *testing.T) {
	// 完整参数组合测试
	result := Build(BuildParams{
		ToolNames:     []string{"bash", "read", "write", "send_message", "cron_create"},
		WorkDir:       "/tmp/workspace",
		WorkspaceName: "test-ws",
		Memories:      []string{"记忆1", "记忆2"},
		InjectedCtx:   "项目描述信息",
	})

	checks := []string{
		"Kele",
		"**bash**",
		"**read**",
		"**write**",
		"**send_message**",
		"**cron_create**",
		"/tmp/workspace",
		"test-ws",
		"记忆1",
		"记忆2",
		"项目描述信息",
		"必须调用工具",
		"定时任务注意事项",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("完整参数输出应包含 %q", check)
		}
	}
}

func TestBuild_MultiStepExamples(t *testing.T) {
	result := Build(BuildParams{})

	if !strings.Contains(result, "多步骤任务示例") {
		t.Error("输出应包含多步骤示例标题")
	}
}

func TestBuild_AgentTools(t *testing.T) {
	result := Build(BuildParams{
		ToolNames: []string{"spawn_agent", "agent_status", "agent_result"},
	})

	tests := []struct {
		tool string
		desc string
	}{
		{"spawn_agent", "启动子 agent"},
		{"agent_status", "查看子 agent 状态"},
		{"agent_result", "等待子 agent 完成"},
	}

	for _, tt := range tests {
		if !strings.Contains(result, "**"+tt.tool+"**") {
			t.Errorf("输出应包含工具名 %q", tt.tool)
		}
		if !strings.Contains(result, tt.desc) {
			t.Errorf("%s 应包含描述 %q", tt.tool, tt.desc)
		}
	}

	// 多步骤示例中应有并行任务示例
	if !strings.Contains(result, "并行执行多任务") {
		t.Error("输出应包含并行任务示例")
	}
}
