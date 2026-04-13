# Business Strategy v0.1

## 1. 开源价值分析

### 1.1 为什么继续开源

开源的价值不在于"把代码免费放出去"，而在于"把你对未来工作流的判断公开摆到市场上，让市场替你放大、检验、传播和定价"。

四类价值：

| 类型 | 说明 |
|---|---|
| **分发资产** | 降低试用门槛，被正确的人和系统发现 |
| **认知资产** | 在"terminal-first agent workflow 轻量控制台"这个类别里占词 |
| **产品资产** | 加快识别真实需求，获取比口头反馈更有价值的使用信号 |
| **职业资产** | 证明判断力和产品感觉，展示对场景、边界、取舍的控制能力 |

### 1.2 开源价值的四种兑现方式

1. **被采用** — 用户真的在自己机器上部署和使用
2. **被引用** — 别人写文章、做视频、放进工具列表里提到
3. **被复用** — 别人用了设计或部分组件
4. **被自己拿来升级** — 开源项目逼你把模糊直觉变成清晰对象

### 1.3 值得争取的硬信号

- 有人真的在自己机器上部署
- 有人愿意拿它管理长任务或 agent session
- 有人开始把它和 SSH、tmux、browser IDE、OpenClaw 之类放在同一讨论里
- 自己能从它演化出下一层控制面产品

## 2. 开源与商业边界

### 核心原则

> 不要卖代码，卖"让 agent 在真实组织里可控、可审计、可协作地运行"的能力。

商业护城河不来自"代码看不到"，更有效的护城河是：

- 托管与运维复杂度
- 团队协作与治理能力
- 安全、审计、权限、合规
- 工作流集成
- 品牌与默认选择权
- 持续交付速度

### 开源版 = 分发层

**长期开源**的部分：

- 单用户
- Local runtime
- Basic remote runtime
- 基础任务创建
- 基础 session persistence
- 基础 terminal attach
- 基础 checkpoint（含手动触发 + 简单规则触发，如 destructive command 拦截；高级策略引擎规则属于商业版）
- 基础 file view
- 基础 audit log

### 商业版 = 控制层

**未来收费**的部分：

- 多用户组织空间
- 团队级 runtime orchestration
- 策略引擎（高级规则）
- 复杂审批流
- 高级审计与回放
- SSO / SAML
- 权限模型
- 任务模板与组织规范
- 集群级 runtime 管理
- Usage analytics
- Hosted cloud

## 3. 商业分层设计

### 3.1 免费开源版

**目标**：最大化传播和 adoption

包含：

- 单用户
- 本地或单机远程
- 基础 session persistence
- 基础 terminal / file / editor
- 基础 mobile access
- 基础 agent task view

### 3.2 Pro 个人版

**目标**：抓住重度个人用户

收费点：

- 高级 session timeline
- 自动摘要
- 多模型/多 agent connectors
- 增强恢复
- 使用统计
- 加密 secrets 管理
- 本地 app 高级功能

### 3.3 Team / Enterprise 版

**目标**：真正商业化

收费点：

- 多用户组织
- SSO / SAML
- 权限模型
- 审批流
- 审计日志
- 会话回放
- 策略引擎
- 多机与多 runtime 管理
- Usage quotas
- 组织级 dashboard
- Private cloud / on-prem deployment pack

### 3.4 Cloud 托管版

**目标**：最强商业模式

卖点：

- 免部署
- 免升级
- 免公网安全配置
- 统一远程入口
- 集中 agent runtime
- 团队共享与治理

## 4. 变现路径分析

### 4.1 最现实的路径：Open Core / 商业增强版

核心单机能力开源，团队和治理能力收费。参考模型：GitLab（Free / Premium / Ultimate）、Grafana（OSS / Enterprise）。

### 4.2 很强的路径：托管服务

代码开源，卖 Cloud / Hosted / Managed。参考模型：MongoDB（Community + Atlas）、Sentry（self-hosted + SaaS）。

### 4.3 可以做但别当主线：支持 / 咨询 / 定制

部署服务、私有化落地、安全审计、企业定制、团队培训。适合作为早期收入补充，不适合作为核心商业模式（规模性差）。

### 4.4 可以有但不高估：赞助与捐赠

GitHub Sponsors。更像社区信号和心智加成，不是主业务模型。

### 4.5 有争议但有时有效：许可证防云厂

防止被别人直接拿去做托管服务。当前阶段不建议太早走这条，更需要分发和 adoption。

## 5. 必须避免的三条错误路线

1. **全开源 + 纯靠赞助** — 大概率不够
2. **太早闭源** — 会失去分发和可信度
3. **把最值钱的团队功能也放进免费层** — 后面几乎没法收费

## 6. 当前阶段策略

### 现在（0-6 个月）

- 继续开源，但要有边界
- 目标不是"完全免费送掉未来价值"
- 用开源把单机基础层做成默认入口

### 中期（6-12 个月）

不急着收钱，先验证三件事：

1. 真的有人持续用它吗
2. 用的人是在个人场景还是团队场景
3. 他们最痛的点是在"执行层"还是"治理层"

### 验证到团队信号后

立刻把商业层收敛到 **Team control plane**，而不是继续做"更强的 terminal"。

## 7. 本地 App 方向

### 7.1 为什么做本地版

- 用户门槛大幅下降（下载即用 vs 服务器+配置+网络）
- 覆盖更大用户群（本地 AI CLI 用户、不愿自托管的人）
- 使用频率更高（本地工具每天用 vs 远程工具偶尔用）

### 7.2 本地版的正确形态

不是 "Electron 版 terminal"，而是 "AI Agent Workspace（本地版）"。

### 7.3 技术路径

| 路径 | 评价 |
|---|---|
| Electron | 不推荐（重、内存大、与轻量定位冲突） |
| Tauri | 推荐（轻、安全、适合 Go + Web 架构） |
| 纯 CLI + browser | 最简单（启动本地 server，自动打开浏览器） |

### 7.4 统一模式

最优解不是两个产品，而是一个产品两种模式：

```text
打开 app → 选择：
  - Local machine
  - Remote machine
```

## 8. 流量分析（参考）

当前 GitHub Traffic 特征（截至 2026-04）：

| 指标 | 值 | 含义 |
|---|---|---|
| Unique visitors | 7 | 很少真实浏览 |
| Unique cloners | 110 | 很多"下载行为" |
| Clones | 242 | 14 天内 |

特征分析：

- "高 clone、低 visitor" = 被机器/系统消费，而非人类浏览
- 可能来源：AI/benchmark 生态扫描、GitHub crawler、工具聚合器、内部评估系统
- 4 月 6 日左右的 spike 是典型的"批量触发事件"
- 这是正向信号（Level 1: 被机器抓 → Level 2: 被开发者尝试 的临界点）

### 从 Level 1 跃迁到 Level 2 的关键

1. **改一句话定位**（传播级 hook）
2. **加 demo GIF / 截图**（一眼看懂）
3. **做对比表**（vs SSH / VSCode Remote / roambench）
