# RoamBench v0.2.1 发布说明（中文）

## 变更概述

`v0.2.1` 完成了 RoamBench 的命名收口：  
- 对外统一使用 `roambench`（命令、产物、示例配置与服务模板）。  
- 默认配置搜索路径、会话标识与持久化目录均已统一到 `roambench`。  

## 发布包

本次发布附带两个架构的静态包：

- Linux `amd64`
  - `roambench-release-a3a1cf1-20260407-linux-amd64.tar.gz`
  - `SHA256: 18f3fc6901cf2ed1aecc93eebfb4c84d5685359c256d4ac0bf8b9ab13c1c84c1`
- Linux `arm64`
  - `roambench-release-a3a1cf1-20260407-linux-arm64.tar.gz`
  - `SHA256: fbcd6462428972c7d89c54895389b3c1822ebeb7c88a538eb509fe778fd768da`

## 安装与运行

```bash
# 下载并解压（按你的机器架构选择对应文件）
tar -xzf roambench-release-a3a1cf1-20260407-linux-<arch>.tar.gz

# 进入解压目录后
chmod +x roambench
cp roambench.example.toml roambench.toml
./roambench --password-hash
./roambench --config roambench.toml
```

或者直接使用仓库内附带的中文安装脚本（会从 GitHub API 自动匹配该版本正确文件名）：

```bash
bash scripts/install-roambench-v0.2.1.sh v0.2.1 /usr/local/bin
```

## 快速部署建议

- 生产/公网推荐使用 `configs/roambench.example.toml`。  
- 如使用 quickstart（`configs/roambench.quickstart.toml`），请仅用于本机/内网快速体验。  
- 密码使用 `--password-hash` 生成后放入配置文件或安全的环境变量中。  

## 清理建议

- 建议在发布说明中提示用户验证 sha256 后再部署。  
