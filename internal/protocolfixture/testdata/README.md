# CLI 审批协议 fixture

这些文件是手机控制面 Adapter 的阶段 0 基线。每个 `approval.json` 保存一个脱敏的供应商请求和对应响应，使用统一 transcript 外壳，同时保留供应商原始 payload。

验证命令：

```bash
go test ./internal/protocolfixture -run '^TestApprovalFixturesAreReplayable$' -count=1
```

版本含义：

- `minimumSupportedVersion` 是 RoamBench 第一版声明支持的下限，不代表供应商首次引入该功能的历史版本；
- `capturedVersion` 记录本机生成/核对版本，或明确说明只依据官方文档；
- `validationLevel` 区分本机 schema/OpenAPI 生成、已安装版本配合官方文档，以及未安装供应商的官方文档 fixture。

更新供应商最低版本时，必须同时更新 fixture、来源和验证测试。不要把真实工作目录、token、cookie、用户名、私有仓库路径或生产命令写入 fixture。
