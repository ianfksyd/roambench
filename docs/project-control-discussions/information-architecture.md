# Information Architecture

## 1. 一级导航与页面层级

```mermaid
graph TD
    APP["RoamBench Control Plane"]

    APP --> PROJ["Projects"]
    APP --> WS["Workstreams"]
    APP --> TASK["Tasks"]
    APP --> TL["Timeline"]
    APP --> EV["Evidence"]
    APP --> APR["Approvals"]
    APP --> HIST["History / Replay"]
    APP --> RT["Runtimes"]

    PROJ --> PD["Project Dashboard"]
    PD --> PD_RW["Running Workstreams"]
    PD --> PD_RT["Running Tasks"]
    PD --> PD_BT["Blocked Tasks"]
    PD --> PD_PA["Pending Approvals"]
    PD --> PD_HR["High-Risk Changes"]
    PD --> PD_RF["Recent Failures"]
    PD --> PD_RH["Runtime Health"]
    PD --> PD_RD["Recent Decisions"]

    WS --> WSB["Workstream Board"]
    WSB --> WSB_P["Planned"]
    WSB --> WSB_R["Running"]
    WSB --> WSB_W["Waiting Human"]
    WSB --> WSB_B["Blocked"]
    WSB --> WSB_D["Done"]

    TASK --> TD["Task Detail"]
    TD --> TD_OV["Overview"]
    TD --> TD_TL["Timeline"]
    TD --> TD_EV["Evidence"]
    TD --> TD_DF["Files & Diff"]
    TD --> TD_TM["Terminal"]
    TD --> TD_AU["Audit"]

    APR --> AI["Approvals Inbox"]
    AI --> AI_CP["Pending Checkpoints"]
    AI --> AI_CR["Conflicting Reviews"]
    AI --> AI_BE["Budget Exceeded"]
    AI --> AI_FA["Final Acceptance"]

    RT --> RM["Runtime Manager"]
    RM --> RM_L["Local"]
    RM --> RM_R["Remote SSH"]
    RM --> RM_C["Container"]

    style APP fill:#1a1a2e,color:#fff
    style PROJ fill:#16213e,color:#fff
    style WS fill:#16213e,color:#fff
    style TASK fill:#16213e,color:#fff
    style TL fill:#16213e,color:#fff
    style EV fill:#16213e,color:#fff
    style APR fill:#16213e,color:#fff
    style HIST fill:#16213e,color:#fff
    style RT fill:#16213e,color:#fff
    style TD_TM fill:#e94560,color:#fff
```

## 2. 核心对象关系

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

## 3. 三条核心链

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

## 4. Task 状态机

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

## 5. Acceptance Status 生命周期

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

## 6. Session 状态机

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

## 7. 协作协议流程

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

## 8. 自治与预算控制

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

## 9. 三层摘要

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

## 10. 实施阶段

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
