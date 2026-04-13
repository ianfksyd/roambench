# Policy And Decision Rules v0.2

## 1. 核心原则

这部分讨论的中心结论是：

> `Automate policy. Escalate judgment.`

也就是：

- 规则化决策自动化
- 创造性判断与业务验收人工化

系统不应该“模拟人类做所有决定”，而应该执行项目策略，并在策略边界之外及时升级。

## 2. 为什么需要独立的规则文件

如果没有规则文件和策略引擎，系统会退化成下面这种状态：

- 多个 agent 同时运行
- 人不断回答大量 yes/no
- 人依然很累
- 系统只是在制造更多噪音

所以策略不应只是 prompt 的一部分，而应成为系统核心对象，具备：

- 版本化
- 可审计
- 可回放
- 可执行

## 3. 哪些问题适合自动化

### 3.1 程序性问题

- 测试失败后先重试还是先 review
- 是否需要开 verifier session
- 是否需要回滚上一步
- 是否需要把 claim 送审

### 3.2 安全边界问题

- 是否允许 destructive command
- 是否允许写保护目录
- 是否允许外部通信
- 是否允许修改核心模块

### 3.3 质量门槛问题

- 测试覆盖率是否达到阈值
- 所有相关测试是否通过
- 是否存在关键 reviewer objection
- 变更范围是否触发更高等级审批

### 3.4 资源和预算问题

- 并发 session 数是否超限
- 会话持续时间是否超限
- 成本预算是否超限
- 长时间无进展是否需要升级

## 4. 哪些问题不该自动化

### 4.1 目标重定义

- 原任务目标本身是错的
- 需要调整产品方向
- 发现更优架构路线
- 原需求有缺陷

### 4.2 创造性方案选择

- 多个都可行但 trade-off 不同的方案取舍
- 为长期演进牺牲短期速度
- 是否引入新的抽象层

### 4.3 业务验收

- 这个结果是否真的满足预期
- 这个 patch 是否值得 merge
- 团队是否愿意承担后续维护成本

### 4.4 高后果判断

- 大规模删除
- 跨模块重构
- 安全敏感改动
- 发布级决策

## 5. 规则文件应包含的最低结构

建议用结构化文件加少量自然语言说明，不要全部写成自由文本。

推荐至少包含 6 类规则：

### 5.1 Completion Rules

定义什么叫“完成”。

示例：

- relevant tests pass
- coverage `>= 80%`
- public API 变化时文档必须更新
- 不存在高严重度 reviewer objection

### 5.2 Review Rules

定义什么情况下必须送 review。

示例：

- changed files `> 10`
- touched core modules `> 2`
- 命中安全敏感文件
- confidence `< 0.7`
- 任意测试失败

### 5.3 Approval Rules

定义什么动作必须升级给人类。

示例：

- 删除 workspace 外文件
- 修改部署配置
- 创建超过阈值的占槽 session
- 改动生产相关路径

### 5.4 Autonomy Limits

定义执行边界。

示例：

- allowed runtimes
- writable paths
- internet access allowed / denied
- max session duration
- max concurrent sessions

并发上限的解释应固定为：

- `Runtime.max_concurrent_sessions` 是 runtime-local 的硬上限
- `policy.autonomy_limits.max_concurrent_sessions` 是 policy / project scope 的调度上限
- 新 session 只有在两者都满足时才允许进入实际执行
- 占用执行槽位的 session 状态至少包括：
  `starting`、`active`、`paused`、`waiting_review`、`waiting_human`、`reconnecting`
- `queued`、`crashed`、`completed`、`terminated` 不计入占槽并发

### 5.5 Quality Gates

定义阶段门槛。

示例：

- implementation 阶段不能在测试失败时关闭
- validation 阶段必须达到覆盖率阈值
- review 阶段不能在 reviewer 明确反对时自动通过

### 5.6 Escalation Rules

定义什么时候必须把问题抛给人类。

示例：

- repeated failure count `>= 3`
- reviewer verdict 冲突
- no progress for `60 min`
- budget exceeded
- task drift detected

这些规则一旦真的触发“进入人类处理队列”，建议统一物化成 `Checkpoint`。
例如：

- conflicting reviews -> `Checkpoint(trigger_type=conflicting_reviews)`
- budget exceeded -> `Checkpoint(trigger_type=budget_exceeded)`
- final acceptance -> `Checkpoint(trigger_type=final_acceptance)`

这样 approvals inbox 的规范记录源始终只有一套，不会同时维护“规则命中列表”和“checkpoint 列表”两份待办。

## 6. 不要用单一指标定义完成

“覆盖率 `> 80%` 才算完成”是一个合理例子，但不应该成为唯一标准。

更稳的做法是使用阶段门槛组合：

### Implementation Complete

- 代码可编译
- 必需文件已改动
- 没有明显语法或运行时错误

### Validation Complete

- 相关测试通过
- 覆盖率达到阈值
- 没有关键 reviewer objection

### Ready For Human Acceptance

- 所有必要检查完成
- 证据齐全
- 冲突已处理
- 满足送交验收条件
- 验收通过后必须生成显式的 final acceptance decision

### Acceptance Status Lifecycle

建议把最终验收状态单独建模，而不是混进执行状态。

- `not_ready`：还没有达到送交验收条件
- `ready_for_acceptance`：系统判断已经满足送交验收门槛
- `under_human_review`：已经进入人类验收队列，等待显式裁决
- `accepted`：人类明确验收通过，并生成 final acceptance decision
- `rejected`：人类明确拒绝验收，并生成 final acceptance decision

补充约束：

- `ready_for_acceptance` 不等于 `accepted`
- `under_human_review` 只能从 `ready_for_acceptance` 进入，不能跳过前置状态
- `under_human_review` 应同时对应一个 `Checkpoint(trigger_type=final_acceptance, status=pending)`，以便统一进入人类审批队列
- `accepted` 和 `rejected` 只能由显式 final acceptance decision 写入
- `rejected` 表示当前这次验收被明确拒绝，不代表任务自动再次“准备好验收”
- `rejected` 后任务通常应回到 `running` 或 `blocked`，而不是停留在表面完成状态
- 只有在返工完成并重新满足门槛后，`acceptance_status` 才能从 `not_ready` 再次进入 `ready_for_acceptance`

## 7. Decision Classifier

建议在 orchestrator 前面加一层分类器，先判断问题属于哪一类，再决定由谁处理。

### Class A：Policy-resolvable

完全由规则文件解决。

### Class B：Evidence-resolvable

可以通过补测试、开 reviewer、查看 diff、补充证据自动解决。

**B 类的边界判定规则**：B 类仅在"需要什么证据"是确定性的情况下成立。即系统能用规则推导出具体动作（如"运行测试"、"检查覆盖率"、"diff 指定文件"），而不需要判断"应该调查什么"。

- B 类示例：测试失败 → 重跑测试 → 结果确定
- B 类示例：覆盖率不达标 → 补充指定模块的测试
- **不是 B 类**：reviewer 说"这个方案有隐患" → 系统不知道该调查什么 → 升级为 C 类
- **不是 B 类**：agent 报告"不确定这个改动是否安全" → 无确定性证据路径 → 升级为 C 类

一句话判定：如果系统必须*决定调查什么*，那就是 C 类，不是 B 类。

### Class C：Judgment-required

必须升级给人类。

### Class D：Goal-redefinition

需要人类重新定义目标或方向。

这样做的结果是：

- 人类不会被海量低价值 yes/no 打断
- 系统不会把“需要证据”误判成“需要人类”
- judgment call 会被显式区分出来

## 8. 规则文件建议结构

```yaml
policy:
  version: "0.1"
  project: "example-project"

  completion_rules:
    coverage_threshold: 0.8
    require_relevant_tests_pass: true
    require_docs_on_public_api_change: true
    block_on_high_severity_objection: true

  review_rules:
    max_changed_files_before_review: 10
    max_core_modules_before_review: 2
    review_on_any_test_failure: true
    review_on_low_confidence_below: 0.7

  approval_rules:
    require_human_for_destructive_commands: true
    require_human_for_deployment_changes: true
    require_human_for_protected_path_changes: true

  autonomy_limits:
    allowed_runtimes: ["local", "ssh_remote"]
    max_concurrent_sessions: 2
    max_session_duration_minutes: 90
    internet_access: "restricted"
    writable_paths:
      - "/workspace"

  escalation_rules:
    repeated_failure_threshold: 3
    no_progress_minutes: 60
    escalate_on_conflicting_reviews: true
    escalate_on_budget_exceeded: true
```

## 9. 执行逻辑

建议把执行逻辑收束成下面这条：

1. session 提出问题或主张
2. Decision Classifier 对问题分类
3. 如果属于 A 类，直接由 policy engine 决定
4. 如果属于 B 类，系统先补证据或补执行
5. 如果属于 C 或 D 类，升级给人类
6. 所有关键动作都生成 `Decision`、`Artifact` 和 `Event`

其中，最终人类验收也必须写成显式 `Decision`，并回填到任务对象上的 acceptance 引用字段。
当系统判断任务已经满足送交验收门槛时，应先把 `Task.acceptance_status` 从 `not_ready` 推进到 `ready_for_acceptance`。
只有在任务真正进入人类验收队列或被人类打开审阅时，才允许从 `ready_for_acceptance` 进入 `under_human_review`，并同步创建 `Checkpoint(trigger_type=final_acceptance)`。
系统和人类都不能跳过这一步直接把任务写成 `accepted`。
如果人类明确拒绝验收，系统应先写入 `final_acceptance_rejected` decision，解析并关闭该 `final_acceptance` checkpoint，再把任务执行状态推进到 `running` 或 `blocked`，并把后续验收生命周期从 `not_ready` 重新开始。
如果任务在 `ready_for_acceptance` 或 `under_human_review` 期间，因为依赖失效、策略失效或其他系统性回退而不再满足验收前提，系统必须把 `acceptance_status` 回退到 `not_ready`；若存在 pending 的 `final_acceptance` checkpoint，则应将其标记为 `expired` 并移出 approvals inbox，而不是继续保留一条可点击但已无效的验收待办。

## 10. 最终判断

规则文件不是可选优化，而是系统从“多个 agent 的噪音源”进化为“真正控制层”的关键。

最终收束成一句话：

> 不是让主 agent 模仿人类做所有决定，而是把大量低价值判断固化成规则，让系统自动执行；只把真正需要人类智慧的部分留给人。

## 11. 策略版本绑定

### 核心规则

Task 在创建时绑定当时生效的 Policy 版本（通过 `Task.policy_version_id`）。

- 新创建的 Task 自动绑定当前 active Policy
- Policy 版本更新后，已有 Task 继续使用原版本，不自动迁移
- 如果需要将进行中的 Task 迁移到新 Policy，这是一个 Class D（Goal-redefinition）决策，必须由人类显式裁决

### 为什么不自动迁移

- 规则变化可能导致进行中的 Task 突然不合规（例如覆盖率阈值提高）
- 证据和裁决的审计链会断裂——同一个 Task 前半段用旧规则、后半段用新规则
- 自动迁移会制造隐式行为变化，违背"显式优于隐式"的设计原则

### Policy 版本在 YAML 中的体现

```yaml
policy:
  version: "0.2"
  supersedes: "0.1"
  effective_for: "new_tasks_only"
```

## 12. 规则冲突解决顺序

当多条规则同时评估且结论矛盾时，系统按以下优先级从高到低执行：

1. **Escalation Rules**（最高优先级）—— 如果触发升级条件，立即升级，不管其他规则怎么说
2. **Approval Rules** —— 如果需要人类审批，阻塞流程
3. **Review Rules** —— 如果需要 review，阻塞完成判定
4. **Quality Gates** —— 如果质量门槛未达标，阻塞状态迁移
5. **Completion Rules** —— 只有在以上全部通过后，才允许标记完成
6. **Autonomy Limits（调度类）**（最低优先级）—— 约束并发数、session 时长等调度边界

**重要说明**：Autonomy Limits 中的 **硬安全约束**（`writable_paths`、`protected_paths`、`allowed_runtimes`、`internet_access`）在执行时始终强制检查，不受上述优先级排序影响。它们不是"冲突解决"层面的规则，而是执行层面的前置守卫。上述第 6 位仅适用于调度类约束（如 `max_concurrent_sessions`、`max_session_duration`）。

### 核心原则

- 高优先级规则的"阻塞"结论不可被低优先级规则的"通过"覆盖
- 同级规则之间取"最严格"结论（即任一条阻塞则阻塞）
- 冲突解决的结果必须记录在 `Decision` 中，包含触发了哪些规则

### YAML 中的建议表达

```yaml
policy:
  rule_precedence:
    - escalation_rules
    - approval_rules
    - review_rules
    - quality_gates
    - completion_rules
    - autonomy_limits
  conflict_resolution: "most_restrictive_wins"
```
