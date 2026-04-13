# Information Architecture

## 设计原则

1. **不破坏现有 Terminal 体验** — 用户打开 RoamBench 仍然看到熟悉的终端工作区
2. **通过标签切换进入项目面板** — 顶层只有两个模式：Terminal 和 Project Panel
3. **项目面板内是逐层递进** — Project → Workstream → Task → Session，不是平铺 8 个导航
4. **Terminal 同时也是 Session 的执行视图** — 点击某个 Task 的 Session 时，可以直接 attach 到对应终端

## 1. 顶层模式切换

```mermaid
graph LR
    APP["RoamBench"]
    APP --> TW["🖥 Terminal Workspaces<br/><i>现有功能，默认视图</i>"]
    APP --> PP["📋 Project Panel<br/><i>新增，项目控制视图</i>"]
    APP --> BADGE["🔔 Approvals Badge<br/><i>全局待办计数</i>"]

    style APP fill:#1a1a2e,color:#fff
    style TW fill:#0f3460,color:#fff
    style PP fill:#533483,color:#fff
    style BADGE fill:#e94560,color:#fff
```

- **Terminal Workspaces**：现有的 1/2/4 分屏终端，完全保留，是默认首页
- **Project Panel**：新增标签，切入项目控制视图
- **Approvals Badge**：全局悬浮或嵌入 header，显示待审批数量，点击进入审批列表

## 2. 项目面板：逐层递进结构

```mermaid
graph TD
    PP["Project Panel"]

    PP --> PL["Project List<br/><i>选择项目</i>"]
    PL --> PD["Project Dashboard<br/><i>当前项目总览</i>"]

    PD --> WS_BOARD["Workstream Board<br/><i>看板：多条工作线</i>"]
    PD --> APPROVALS["Approvals Inbox<br/><i>待审批事项</i>"]
    PD --> RUNTIMES["Runtime Status<br/><i>运行环境健康</i>"]

    WS_BOARD --> TD["Task Detail<br/><i>单个任务详情</i>"]

    TD --> SD["Session Detail<br/><i>单次执行实例</i>"]

    SD --> TERM["→ Attach Terminal<br/><i>跳回 Terminal 模式</i>"]

    style PP fill:#533483,color:#fff
    style PL fill:#16213e,color:#fff
    style PD fill:#16213e,color:#fff
    style WS_BOARD fill:#0f3460,color:#fff
    style TD fill:#0f3460,color:#fff
    style SD fill:#0f3460,color:#fff
    style TERM fill:#e94560,color:#fff
    style APPROVALS fill:#e94560,color:#fff
    style RUNTIMES fill:#16213e,color:#fff
```

用户的浏览路径是：

```
Project Panel → 选择项目 → Project Dashboard → 点击某条工作线
→ Workstream Board → 点击某个任务 → Task Detail → 点击某个 Session
→ Session Detail → Attach Terminal（跳回终端模式）
```

## 3. 每一层看到什么

### 3.1 Project Dashboard（项目总览）

从 Project List 点击某个项目后进入。一屏展示项目态势。

```mermaid
graph TD
    PD["Project Dashboard"]

    PD --> M1["Running Workstreams<br/>进行中的工作线"]
    PD --> M2["Running Tasks / Blocked Tasks<br/>任务状态分布"]
    PD --> M3["Pending Approvals<br/>待审批计数 + 快捷入口"]
    PD --> M4["Recent Failures<br/>最近失败"]
    PD --> M5["Recent Decisions<br/>最近裁决"]
    PD --> M6["Runtime Health<br/>运行环境状态"]
    PD --> M7["Project Timeline<br/>最近事件流（折叠）"]

    style PD fill:#16213e,color:#fff
```

### 3.2 Workstream Board（工作线看板）

点击 Dashboard 中某条工作线，或直接从 Dashboard 的工作线列表进入。

```mermaid
graph LR
    subgraph "Workstream Board"
        P["Planned"] --> R["Running"] --> W["Waiting Human"] --> B["Blocked"] --> D["Done"]
    end

    R --> TC["点击 Task 卡片<br/>→ Task Detail"]

    style P fill:#1a1a2e,color:#fff
    style R fill:#0f3460,color:#fff
    style W fill:#e94560,color:#fff
    style B fill:#533483,color:#fff
    style D fill:#16213e,color:#fff
```

每张 Task 卡片显示：标题、agent、runtime、状态、风险、最近摘要。

### 3.3 Task Detail（任务详情）

点击 Board 中某个 Task 卡片后进入。这是信息最密的页面。

```mermaid
graph TD
    TD["Task Detail"]

    TD --> TAB1["Overview<br/>一句话状态 + 风险 + 下一步"]
    TD --> TAB2["Timeline<br/>事件流"]
    TD --> TAB3["Evidence<br/>证据审阅"]
    TD --> TAB4["Files & Diff<br/>变更与比较"]
    TD --> TAB5["Sessions<br/>执行实例列表"]
    TD --> TAB6["Audit<br/>审批 / 拒绝 / 恢复记录"]

    TAB5 --> SS["点击 Session<br/>→ Session Detail"]

    style TD fill:#0f3460,color:#fff
    style TAB5 fill:#533483,color:#fff
    style SS fill:#e94560,color:#fff
```

**注意**：Terminal 不再作为 Task Detail 的一个 tab。而是在 Session Detail 里提供 "Attach Terminal" 操作，自然地跳回终端模式。

### 3.4 Session Detail（会话详情）

点击 Task Detail 的 Sessions 列表中某个 Session 后进入。

```mermaid
graph TD
    SD["Session Detail"]

    SD --> SI["Session Info<br/>agent、runtime、role、状态、时长"]
    SD --> SL["Session Log<br/>关键输出摘要"]
    SD --> SC["Claims<br/>本次会话的主张"]
    SD --> SA["Artifacts<br/>本次会话的产物"]
    SD --> AT["⏎ Attach Terminal<br/>跳回终端模式，连接到此 Session"]

    style SD fill:#0f3460,color:#fff
    style AT fill:#e94560,color:#fff
```

`Attach Terminal` 是 Project Panel 和 Terminal Workspaces 之间的桥梁：

- 点击后切换回 Terminal 模式，自动连接到该 Session 对应的 tmux session
- 用户可以直接操作终端，完成后切回 Project Panel 继续追踪

### 3.5 Approvals Inbox（审批收件箱）

从 Project Dashboard 的 "Pending Approvals" 或全局 Badge 进入。

```mermaid
graph TD
    AI["Approvals Inbox"]

    AI --> CP["Pending Checkpoints<br/><i>canonical record source</i>"]

    CP --> F1["🔴 destructive_command"]
    CP --> F2["🟡 conflicting_reviews"]
    CP --> F3["🟠 budget_exceeded"]
    CP --> F4["🟣 final_acceptance"]
    CP --> F5["⚪ protected_path_change"]

    F1 --> ACT["Approve / Reject / Reroute"]

    style AI fill:#e94560,color:#fff
    style CP fill:#16213e,color:#fff
```

所有审批项都是 `pending checkpoints` 的过滤视图，不是独立数据源。

## 4. 两个模式之间的桥接

```mermaid
graph LR
    TW["Terminal Workspaces<br/><i>现有终端</i>"]
    PP["Project Panel<br/><i>项目控制</i>"]

    TW -->|"顶部标签切换"| PP
    PP -->|"顶部标签切换"| TW
    PP -->|"Session Detail → Attach Terminal"| TW
    TW -->|"未来: 终端内识别 Task 上下文"| PP

    style TW fill:#0f3460,color:#fff
    style PP fill:#533483,color:#fff
```

关键设计：

- 两个模式是**平级标签切换**，不是层级嵌套
- Project Panel → Terminal 的桥是 "Attach Terminal"
- Terminal → Project Panel 的桥是顶部标签（未来可做：终端内识别当前 Task 上下文后，提供快捷跳转）

## 5. 核心对象关系

```mermaid
erDiagram
    Project ||--o{ Workstream : contains
    Project ||--o{ Task : contains
    Project ||--o{ Runtime : registers
    Project ||--o{ Policy : governs
    Project ||--o{ Event : logs

    Workstream ||--o{ Task : organizes

    Task ||--o{ Task : "parent/child"
    Task ||--o{ Session : executes
    Task ||--o{ Claim : produces
    Task ||--o{ Review : receives
    Task ||--o{ Decision : resolves
    Task ||--o{ Checkpoint : raises
    Task ||--o{ Event : logs

    Session ||--o{ Claim : submits
    Session ||--o{ Artifact : generates
    Session ||--o{ Event : emits
    Session }o--|| Runtime : "runs on"

    Claim ||--o{ Review : reviewed_by
    Claim ||--o{ Artifact : "evidence_refs"

    Review }o--|| Decision : "informs"
    Checkpoint }o--|| Decision : "resolved_by"

    Policy ||--o{ Task : "governs via policy_version_id"
```

## 6. 三条核心链

```mermaid
graph LR
    subgraph "执行链 Execution Chain"
        T1[Task] --> S1[Session] --> A1[Artifact]
    end

    subgraph "协作链 Collaboration Chain"
        C1[Claim] --> R1[Review] --> D1[Decision]
    end

    subgraph "历史链 History Chain"
        ANY[Any Action] --> E1[Event]
    end

    style T1 fill:#0f3460,color:#fff
    style S1 fill:#0f3460,color:#fff
    style A1 fill:#0f3460,color:#fff
    style C1 fill:#533483,color:#fff
    style R1 fill:#533483,color:#fff
    style D1 fill:#533483,color:#fff
    style ANY fill:#e94560,color:#fff
    style E1 fill:#e94560,color:#fff
```

## 7. Task 状态机

```mermaid
stateDiagram-v2
    [*] --> planned
    planned --> queued
    queued --> planned
    queued --> running

    running --> waiting_review
    running --> waiting_human
    running --> blocked
    running --> failed
    running --> execution_complete

    waiting_review --> running
    waiting_review --> blocked
    waiting_human --> running
    waiting_human --> blocked
    blocked --> running
    failed --> running

    execution_complete --> running : rejection
    execution_complete --> blocked : dependency reverted
    execution_complete --> archived : accepted

    state "execution_complete" as execution_complete
    note right of execution_complete
        执行完成 ≠ 业务已验收
        验收由 acceptance_status 独立跟踪
    end note
```

## 8. Acceptance Status 生命周期

```mermaid
stateDiagram-v2
    [*] --> not_ready
    not_ready --> ready_for_acceptance : 满足送交验收门槛
    ready_for_acceptance --> under_human_review : 进入人类验收队列
    under_human_review --> accepted : 人类显式批准
    under_human_review --> rejected : 人类显式拒绝
    rejected --> not_ready : 返工后重新申请

    note right of accepted
        必须生成显式
        final acceptance Decision
    end note
```

## 9. Session 状态机

```mermaid
stateDiagram-v2
    [*] --> queued
    queued --> starting : runtime 槽位可用
    queued --> terminated : 取消

    starting --> active

    active --> waiting_review
    active --> waiting_human
    active --> paused
    active --> reconnecting
    active --> crashed
    active --> completed

    paused --> active
    waiting_review --> active
    waiting_human --> active
    reconnecting --> active
    crashed --> terminated
```

## 10. 协作协议流程

```mermaid
sequenceDiagram
    participant W as Worker Agent
    participant S as System
    participant R as Reviewer Agent
    participant H as Human

    W->>S: Submit Claim + Evidence
    S->>S: Decision Classifier

    alt Class A: Policy-resolvable
        S->>S: Policy Engine auto-decides
        S->>W: Decision(approve/reject)
    else Class B: Evidence-resolvable
        S->>R: Dispatch Reviewer Session
        R->>S: Submit Review(verdict)
        S->>S: Policy Engine evaluates
        S->>W: Decision
    else Class C: Judgment-required
        S->>H: Raise Checkpoint
        H->>S: Decision(approve/reject/reroute)
        S->>W: Decision
    else Class D: Goal-redefinition
        S->>H: Raise Checkpoint(goal_redefinition)
        H->>S: New goal / direction
    end
```

## 11. 自治与预算控制

```mermaid
graph TB
    subgraph "自治层级 Autonomy Levels"
        L0["Level 0: Human<br/>目标、约束、预算、最终裁决"]
        L1["Level 1: Orchestrator<br/>拆任务、分配 agent/runtime、触发 review"]
        L2["Level 2: Task Agents<br/>分析、编码、测试、提 claim"]
        L3["Level 3: Sessions<br/>具体执行，无项目级自治权"]
    end

    L0 --> L1 --> L2 --> L3

    subgraph "预算约束 Budget Limits"
        B1["Task: max 5 active / project"]
        B2["Subtask: max 2 auto-spawned / task"]
        B3["Session: max 2 active / task"]
        B4["Time: 60-90 min / session before review"]
        B5["Change: 15 files → forced review"]
    end

    L1 -.->|受约束| B1
    L1 -.->|受约束| B2
    L2 -.->|受约束| B3
    L2 -.->|受约束| B4
    L2 -.->|受约束| B5

    style L0 fill:#1a1a2e,color:#fff
    style L1 fill:#16213e,color:#fff
    style L2 fill:#0f3460,color:#fff
    style L3 fill:#533483,color:#fff
```

## 12. 三层摘要

```mermaid
graph TB
    subgraph "人类看到的层级"
        L1_S["Layer 1: 一句话状态<br/>'已完成依赖升级，测试 2 项失败'"]
        L2_S["Layer 2: 结构化摘要<br/>Files: 4 | Tests: 18/2 | Risk: medium"]
        L3_S["Layer 3: 完整原始输出<br/>展开查看 CLI 全文"]
    end

    L1_S -->|点击展开| L2_S -->|点击展开| L3_S

    style L1_S fill:#0f3460,color:#fff
    style L2_S fill:#16213e,color:#fff
    style L3_S fill:#1a1a2e,color:#fff
```

## 13. 实施阶段

```mermaid
gantt
    title 12 周实施计划
    dateFormat YYYY-MM-DD
    axisFormat %m/%d

    section 阶段 1: 定义
    产品定义与边界           :a1, 2026-04-14, 14d

    section 阶段 2: 数据层 + 本地
    数据模型 + 本地 Runtime  :a2, after a1, 14d

    section 阶段 3: 远程
    统一远程 Runtime         :a3, after a2, 14d

    section 阶段 4: 控制台
    Dashboard + Board + Detail :a4, after a3, 14d

    section 阶段 5: 历史
    History + Replay          :a5, after a4, 14d

    section 阶段 6: 发布
    打磨 + Alpha 发布        :a6, after a5, 14d
```
