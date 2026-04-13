# Autonomy Policy v0.1

## 1. 总目标

在提高复杂项目执行效率与保持人类对方向、风险、成本和最终结果的控制之间保持平衡。

总原则：

> 允许自治，但禁止无边界自治。（Autonomy is allowed; unbounded autonomy is forbidden.）

## 2. 自治层级

### Level 0：Human

- 定义项目目标
- 设定约束与预算
- 审批高风险动作
- 对冲突结论作最终裁决

### Level 1：Orchestrator

- 拆任务
- 分配 agent
- 分配 runtime
- 控制 session 数量
- 触发 review
- 汇总结果
- 请求 checkpoint

### Level 2：Task / Subtask Agents

- 分析
- 编码
- 测试
- 审查
- 提出 claim
- 请求更多执行资源

### Level 3：Sessions

具体执行，不拥有项目级自治权。只负责完成本次执行目标，不应自己改变项目全局方向。

## 3. 自治对象

系统只允许对以下三个对象进行自治操作：

1. **Task spawn** — 创建新的子 task
2. **Session spawn** — 为某个 task 或 subtask 创建新的执行 session
3. **Review trigger** — 主动发起一个 reviewer agent 或验证 session

系统不得在第一版中自主创建新的 Project，也不得自主修改全局产品策略。

## 4. Task Spawn Policy

### 4.1 允许自动新增子 task 的条件

只有满足以下任一条件时，系统才允许自动创建子 task：

| 条件 | 说明 |
|---|---|
| 工作天然可分解 | 不同模块、不同文件组、不同验证目标 |
| 主 task 明显过大 | 涉及多个核心模块、需要多个阶段 |
| 需要独立验证路径 | 实现、测试、风险审查分离 |
| 需要替代方案并行探索 | 保守方案与激进方案 |

### 4.2 新增子 task 的必要字段

任何自动生成的子 task 必须包含：

- `parent_task_id`
- `title`
- `goal`
- `scope`
- `expected_output`
- `reason_for_split`
- `dependency_relation`
- `estimated_cost`
- `estimated_risk`

如果系统不能生成这些核心字段，就不允许自动拆分。

### 4.3 子 task 数量限制

| 参数 | 值 |
|---|---|
| soft limit | 每个 task 最多自动生成 2 个子 task |
| hard limit | 超过 3 个必须触发 human checkpoint |

### 4.4 禁止自动拆分的情况

- 任务范围不清晰
- 没有明确边界
- 拆分后会造成强耦合写冲突
- 当前项目已有太多未完成分支
- 系统无法解释拆分理由

## 5. Session Spawn Policy

### 5.1 允许自动新增 session 的场景

| 场景 | 说明 |
|---|---|
| 执行与验证分离 | 一个实现，一个跑测试 |
| 双路径探索 | 保守改法与激进改法 |
| runtime 分离 | 本地快速分析，远程完整测试 |
| 失败恢复 | 当前 session 偏航，开新 session clean retry |

### 5.2 每个 task 的 session 上限

| 参数 | 值 |
|---|---|
| 默认 active sessions | 1 |
| soft burst | 2 |
| hard limit（需审批） | 3 |

### 5.3 多 session 的角色要求

每个新增 session 必须标记角色：

- `implement`
- `test`
- `review`
- `verify`
- `alternative_path`
- `rollback_candidate`

没有角色说明的 session 不允许自动生成。

### 5.4 冲突控制

**允许**：

- 不同模块
- 不同 worktree
- 不同验证路径
- 只读审查 session

**禁止**：

- 多个写 session 同时修改同一核心文件集合
- 多个 session 在无隔离的同一工作区写同一模块

如果检测到潜在写冲突，应自动降为只读 review、触发 checkpoint、或要求 worktree 隔离。

## 6. Review Trigger Policy

### 6.1 必须自动触发 review 的场景

- 改动核心模块
- 改动文件数量超阈值
- 测试未全过
- claim 置信度低
- 多路径结果冲突
- 高风险命令执行后
- 准备提交最终结果前

### 6.2 Review 的目标

不是再次"做任务"，而是判断：

- claim 是否成立
- evidence 是否充分
- 风险是否被低估
- 是否应批准进入下一阶段

## 7. Budget Policy

### 7.1 四类预算

| 预算类型 | 说明 |
|---|---|
| Task budget | 每个 project 同时运行的 task 数上限 |
| Session budget | 每个 task 同时运行的 session 数上限 |
| Time budget | 单个 task / session 最长运行时间 |
| Change budget | 单次任务允许改动的文件数、核心模块数 |

### 7.2 第一版推荐默认值

| 参数 | 值 |
|---|---|
| active tasks per project | 5 |
| auto-spawned subtasks per task | 2 |
| active sessions per subtask | 2 |
| max runtime per session before review | 60-90 min |
| max changed files before forced review | 15 |
| max core-module changes before checkpoint | 3 |

## 8. Checkpoint Policy

### 8.1 必须进入 waiting_human 的情况

- 想创建第 4 个子 task
- 想创建第 3 个 active session
- 想同时写同一高风险模块
- 想执行 destructive command
- 想做大规模删除/迁移
- 多个 reviewer 结论冲突
- 测试失败但想继续推进
- 超出时间预算或修改预算

### 8.2 checkpoint 审批页需要展示的内容

- 为什么要扩张
- 当前已有多少 task / session
- 新增对象的目标是什么
- 预期收益
- 预期风险
- 证据是什么
- 不批准的替代方案是什么

审批不是弹窗，而是结构化决策界面。

### 8.3 Checkpoint 统一规则

以上所有触发 `waiting_human` 的条件，都必须物化为一条 `Checkpoint` 记录（使用对应的 `trigger_type`），以保证 Approvals Inbox 只有一个 canonical record source。不允许同时维护"规则命中列表"和"checkpoint 列表"两份独立待办。

### 8.4 依赖级联

`blocks` 类型的 Task 依赖是 **level-triggered**，而不是"一次满足后永久放行"。如果上游任务在下游完成验收前回退了状态，下游任务必须级联回退（例如 `running → blocked`），pending 的 `final_acceptance` checkpoint 必须同步失效。

## 9. Merge Policy

### 9.1 收敛到 Claim

每个 session 的结果最终都要形成 claim，而不是只留下日志。

### 9.2 Claim 必须绑定 Evidence

没有 evidence 的 claim 不进入最终决策。

### 9.3 多结果冲突时必须显式记录

系统不得静默覆盖冲突结果，必须进入 review → decision 或 waiting_human。

## 10. Replay / Audit Policy

所有自治动作都必须进入历史：

- task spawn
- session spawn
- review trigger
- approval
- rejection
- reroute
- rollback
- merge

以后用户可以回看：为什么拆出这个子 task、为什么多开一个 session、为什么让 reviewer 介入、为什么最后采用方案 A 而不是 B。

## 11. 第一版产品建议：只做受控自治

### 允许

- 父 task 自动拆出最多 2 个子 task
- 每个 subtask 自动开最多 2 个 session
- 自动触发 1 个 reviewer session
- 自动进入 waiting_human

### 不允许

- 无限递归拆分
- session 无限复制
- 无证据的自治扩张
- 无审批的高风险扩张
- 自主修改全局预算

## 12. 核心条文

系统可以自主分解任务并派生执行会话，但所有自治必须满足四个条件：

1. **可解释** — 必须说明为何拆分或扩张
2. **可预算** — 必须受 task/session/time/change 预算约束
3. **可审查** — 必须在高风险或冲突场景进入 checkpoint
4. **可回放** — 所有自治动作必须进入 timeline 与 history

一句话收束：

> 可控自治优先于最大自治。
