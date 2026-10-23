package agentcfg

import (
	"fmt"
	"strings"
)

// Step is one manual onboarding step: the part a user has to do by hand
// (sign in, subscribe, activate). The wizard prints these; it does not try to
// do them, because they need the user's own account.
type Step struct {
	// Title says what to do.
	Title string `json:"title"`
	// Command is the command to run, if any.
	Command string `json:"command,omitempty"`
	// URL is where to go, if any.
	URL string `json:"url,omitempty"`
	// Note adds context.
	Note string `json:"note,omitempty"`
	// Optional marks a step that may be skipped.
	Optional bool `json:"optional,omitempty"`
}

// Guide returns the manual onboarding steps of an agent for a profile.
//
// The steps are deterministic commands, not instructions for another agent to
// interpret: guiding a user through a subscription does not need a language
// model, and a model would only make the outcome less predictable.
func Guide(label string, profile Profile) []Step {
	label = strings.TrimSpace(label)
	gateway := strings.TrimSpace(profile.BaseURL)
	switch label {
	case "claude":
		steps := []Step{
			{Title: "安装 Claude Code", Command: "npm install -g @anthropic-ai/claude-code", Note: "已安装则跳过"},
			{Title: "登录或激活订阅", Command: "claude", Note: "首次运行会给出登录链接,按提示在浏览器完成授权"},
		}
		if gateway != "" {
			steps = append(steps, Step{
				Title:    "确认已指向网关",
				Command:  fmt.Sprintf("roambench-agent config show --agent claude"),
				Note:     "网关地址 " + gateway + " 由向导写入 ~/.claude/settings.json",
				Optional: true,
			})
		}
		return steps
	case "codex":
		steps := []Step{
			{Title: "安装 Codex", Command: "npm install -g @openai/codex", Note: "已安装则跳过"},
			{Title: "登录或激活订阅", Command: "codex login", Note: "设备码登录,按提示在浏览器完成"},
		}
		if gateway != "" {
			steps = append(steps, Step{
				Title:    "确认已指向网关",
				Command:  "roambench-agent config show --agent codex",
				Note:     "网关地址 " + gateway + " 由向导写入 ~/.codex/config.toml",
				Optional: true,
			})
		}
		return steps
	case "opencode":
		steps := []Step{
			{Title: "安装 OpenCode", Command: "npm install -g opencode-ai", Note: "已安装则跳过"},
			{Title: "登录或激活订阅", Command: "opencode auth login", Note: "选择一个 provider 完成授权"},
		}
		if gateway != "" {
			steps = append(steps, Step{
				Title:    "确认已指向网关",
				Command:  "roambench-agent config show --agent opencode",
				Note:     "网关地址 " + gateway + " 由向导写入 ~/.config/opencode/opencode.json",
				Optional: true,
			})
		}
		return steps
	case "hermes":
		steps := []Step{
			{Title: "确认 hermes 可用", Command: "hermes --version", Note: "已安装则跳过"},
		}
		if gateway != "" {
			steps = append(steps, Step{
				Title:    "确认已指向网关",
				Command:  "roambench-agent config show --agent hermes",
				Note:     "向导在 ~/.hermes/config.yaml 里加一个 provider 并切换 model.provider;网关凭据经 key_cmd 提供,无需登录",
				Optional: true,
			})
		}
		return steps
	case "kiro-cli":
		return []Step{{Title: "Kiro 没有自定义模型端点", Note: "kiro-cli 的模型来自 Kiro 自身登录,没有指向网关的配置面;向导只把它当 worker 使用(领任务/交证据),模型流量不走网关"}}
	default:
		return []Step{{Title: "手动接入 " + label, Note: "该 agent 没有内置适配器,请参照其官方文档配置模型接入点"}}
	}
}
