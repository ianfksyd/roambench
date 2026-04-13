# UI Information Architecture v0.1

> 注：这是较早期的平铺导航草图。当前导航与页面层级设计请以 `information-architecture.md` 为准。

## 1. 设计目标

把产品从"终端工作台"升级为"agent 项目控制台"。首页不再是 terminal grid，而是项目态势。

## 2. 一级导航

一级导航固定为以下 8 项：

1. **Projects** — 项目容器，最高层对象
2. **Workstreams** — 工作线看板
3. **Tasks** — 任务列表与详情
4. **Timeline** — 事件时间线
5. **Evidence** — 结构化证据审阅
6. **Approvals** — 统一审批收件箱
7. **History / Replay** — 历史回放
8. **Runtimes** — 运行环境管理

Terminal 只能作为 Task Detail 里的一个 tab，不放首页。

## 3. 关键页面设计

### 3.1 Project Dashboard（首页）

首页必须在 10 秒内回答：

- 项目有没有失控
- 该先处理哪条线
- 哪个 agent 最需要关注
- 哪个决策不能再拖

页面模块：

| 模块 | 内容 |
|---|---|
| Running workstreams | 当前进行中的工作线 |
| Blocked tasks | 被阻塞的任务 |
| Pending approvals | 待审批事项 |
| High-risk changes | 高风险改动 |
| Recent failures | 最近失败 |
| Runtime health | 运行环境健康状态 |
| Recent decisions | 最近裁决 |
| Recent completions | 最近完成的任务 |

### 3.2 Workstream Board

解决"大型项目很多条线同时推进"的问题。

采用 Kanban + tree 混合结构：

| 列 | 含义 |
|---|---|
| Planned | 已规划 |
| Running | 进行中 |
| Waiting human | 等待人类 |
| Blocked | 被阻塞 |
| Done | 已完成 |

同时支持父子任务关系：

```text
Release 1.3
├── Auth refactor
├── Dependency bump
├── Regression tests
└── Docs update
```

每个 task 卡片展示：

- title
- agent
- runtime
- status
- recent summary
- risk
- files changed count

### 3.3 Task Detail

固定 6 个 tab，**顺序不能改**：

1. **Overview** — 一句话状态、风险等级、推荐下一步
2. **Timeline** — 事件流
3. **Evidence** — 结构化证据
4. **Files & Diff** — 变更与比较
5. **Terminal** — 必要时接管（放后面是刻意设计）
6. **Audit** — 审批、拒绝、恢复、重试记录

Terminal 放后面的原因：强迫系统先给用户"方向感、证据感、决策感"，再给"接管入口"。

### 3.4 Timeline View

每个事件结构化显示：

| 字段 | 说明 |
|---|---|
| 时间 | 事件发生时间 |
| actor | agent / human / system |
| action | 具体动作 |
| target | 作用对象 |
| result | 结果 |
| risk | 风险等级 |
| next step | 建议下一步 |

示例：

```text
10:12  Claude Code  started refactor plan
10:19  Claude Code  ran 24 tests, 3 failed
10:24  Claude Code  proposed deleting deprecated config
10:24  System       checkpoint raised: destructive file action
10:26  Human        rejected, requested safer migration
10:40  Claude Code  resumed with rollback-safe plan
```

### 3.5 Evidence View

专门用来"纠错"。固定显示：

- Files changed
- Key commands run
- Test summary
- Errors / warnings
- Agent claims（主张）
- Supporting artifacts（证据）
- Human comments

最重要的是把 **agent 的主张** 和 **可验证证据** 分开：

- 主张：Auth bug fixed
- 证据：3 files modified, 12 tests passed, 1 flaky test skipped

### 3.6 Approvals Inbox

把所有需要人工介入的节点统一收口。

人不该在海量输出里找"哪里需要自己"。系统应主动推送：

- 待审批的 checkpoints（canonical record source）
- conflicting reviews（作为 `pending checkpoints` 中 `trigger_type=conflicting_reviews` 的过滤视图）
- budget exceeded tasks（作为 `pending checkpoints` 中 `trigger_type=budget_exceeded` 的过滤视图）
- final acceptance candidates（作为 `pending checkpoints` 中 `trigger_type=final_acceptance` 的过滤视图，不是额外的独立 payload）

原则：**人处理待决问题，不处理原始噪音。**

### 3.7 Replay View

回放对象：

- 状态变化
- 关键输出
- 文件变更
- checkpoint 决策
- 人工批注
- 切换 agent 或 runtime 的时点

先做事件回放（step through events），不做复杂视频式回放。

### 3.8 Runtime Manager

展示：

- local / remote machines
- online / offline 状态
- active tasks per runtime
- health status
- capabilities

## 4. 三层摘要原则

永远不要让人类先读全文：

### Layer 1：一句话状态

> "已完成依赖升级，测试剩 2 项失败，等待是否继续自动修复。"

### Layer 2：结构化摘要

| 指标 | 值 |
|---|---|
| Files changed | 4 |
| Tests passed / failed | 18 / 2 |
| Risk | medium |
| Needs human decision | yes |

### Layer 3：完整原始输出

只在用户展开时看。

## 5. 四类历史记录

### 5.1 Task History

任务从创建到完成的全过程：谁创建、选了哪个 agent、在哪台 runtime 上跑、中途暂停了几次、谁批准了 checkpoint、最终是否完成。

### 5.2 Decision History

所有关键人工与系统决策：AI 提议了什么、人拒绝了什么、为什么改路线、哪次人工接管改变了结果。

### 5.3 Evidence History

保留每轮 diff、测试、摘要、命令、失败证据。

### 5.4 Runtime History

在哪个 runtime 上，经历了哪些切换与重试。

## 6. Graph View（Phase 2）

不是第一阶段必做，但值得列入第二阶段：

- 任务依赖
- 子任务拆分
- agent spawn 关系
- blocked / unblocked 路径
- 失败分支与恢复分支

## 7. 实施优先级

### Phase 1：可用的信息架构

- Project Dashboard
- Workstream Board
- Task Detail（6 tabs）
- Timeline
- Evidence
- History list

### Phase 2：结构化历史

- Decision records
- Checkpoint history
- Replay mode
- Task-to-task linkage

### Phase 3：高级可视化

- Dependency graph
- Branch / recovery path
- Multi-agent relation graph
- Runtime topology

## 8. 查询视图与后端接口映射

### 8.1 Project Dashboard Query

依赖接口：

```text
GET /api/v1/projects/:project_id/dashboard
```

返回：running workstreams、running tasks、blocked tasks、waiting_human tasks、pending checkpoints、recent decisions、runtime health、recent failures。

### 8.2 Task Detail Query

依赖接口：

```text
GET /api/v1/projects/:project_id/tasks/:task_id
```

返回：task、current sessions、latest claims、latest reviews、latest decision、latest acceptance decision、pending checkpoints、relevant artifacts、timeline tail。

### 8.3 Approvals Inbox Query

依赖接口：

```text
GET /api/v1/projects/:project_id/approvals
```

返回：pending checkpoints、waiting_human tasks（不与 checkpoint 重复）、conflicting reviews、budget exceeded tasks。

### 8.4 Replay Query

依赖接口：

```text
GET /api/v1/projects/:project_id/events?since=&until=&type=
```

按 event 顺序返回：state changes、claims、reviews、checkpoints、decisions、artifact snapshot refs。
