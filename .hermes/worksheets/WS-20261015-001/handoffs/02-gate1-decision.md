# GATE-1 裁决（用户，2026-10-15）
1. Go 升级：同意官方 tarball 装 ~/.local/go（go.mod 是否改 1.18→1.22 未明说，按"go升级"理解为升级到 1.22+ 并统一 go.mod=1.22，见 C0 验收）
2. Hermes 适配：方案 A（只做 API 对接，Hermes 主动拉任务；server 不拉起 hermes 进程）
3. 模型：models 允许为空；推荐按 agentLabel+级别；映射表不进配置文件（硬编码默认即可）
4. repoPath：限制在某根目录之下
5. agentGetTask：废弃（保留路由返回 410 Gone + 文档说明，改用 claim）
