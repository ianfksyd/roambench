# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

> スマホから Codex や Claude Code などの terminal-first coding ワークフローに復帰できる、軽量リモート作業台。

- `tmux` を使って terminal セッションを維持
- `2 / 4` ペインで Codex、Claude Code、Kimi-CLI、OpenCode などを並行実行しやすい
- 重いブラウザ IDE なしで、低オーバーヘッドのまま長時間タスクの開始、監視、再開、軽い編集ができる

RoamBench は、単一ユーザー向けのセルフホスト型リモート作業台です。

`RoamBench` は公開向けの製品名です。現在のリポジトリ名、バイナリ名、設定名、環境変数名は、コードレベルの改名が終わるまで引き続き `liteterm` を使います。

`rstudio-server` のようなリモート作業環境から発想を得ていますが、かなり大胆に機能を絞っています。目的はフル IDE をブラウザへ持ち込むことではなく、本当に必要な部分だけを残すことです。

- terminal
- ファイル閲覧
- 軽い編集
- セッション復帰
- 端末やデバイスをまたいで戻れる workspace

こんな用途に向いています:

- vibe coding
- agent 主導の開発フロー
- Codex、Claude Code、Kimi-CLI、OpenCode などの長時間 CLI ワークフローを並べて動かす運用
- リモートのスクリプト実行や長時間ジョブの監視
- 外出先やスマホからの軽い修正
- terminal から `openclaw` のようなツールへ指示を出す運用

向いていないもの:

- マルチユーザー基盤
- 重いブラウザ IDE の代替
- 大規模な GUI 中心ワークフロー

## Main Features

- `tmux` ベースの永続 terminal セッション
- `1 / 2 / 4` レイアウトの workspace と端末間同期
- ファイルブラウザ、テキスト編集、画像ビューア
- コピー、リネーム、アップロード、ダウンロードなどの軽量ファイル操作
- 下書き復元、未保存警告、find / replace、go-to-line
- パンくずナビゲーションとディレクトリ内フィルタ
- 各 terminal ペインの右側に見えるスクロールバー
- 軽量で低オーバーヘッドな動作

## Quick Start

```bash
make build
cp configs/liteterm.quickstart.toml liteterm.toml
./liteterm --password-hash
export LITETERM_USER="$(whoami)"
export LITETERM_PASSWORD_HASH='<generated hash>'
./liteterm --config liteterm.toml
```

この quickstart はローカル / LAN 向けの最短経路です。`allow_all_ips = true` と `allow_insecure_http = true` を使うため、安全な公開運用には英語版 README のフルセットアップと [Deployment Hardening](docs/deployment-hardening.md) を使ってください: [README.md](README.md)

## Positioning

RoamBench は「SSH とフルブラウザ IDE の中間にある、ちょうどよい作業台」を目指しています。特にモバイル環境では、全部入りよりも「すぐ入れる、すぐ指示できる、すぐ状況を見られる」ことを優先しています。
