# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

> AI coding session を走らせ続けたまま、どこからでも復帰できる軽量リモート作業台。

- `tmux` を使って terminal セッションを維持
- `2 / 4` ペインで Codex、Claude Code、Kimi-CLI、OpenCode などを並行実行しやすい
- 重いブラウザ IDE なしで、別の端末から復帰し、出力確認や軽い編集ができる

RoamBench は、単一ユーザー向けのセルフホスト型リモート作業台です。

`RoamBench` は公開向けの製品名です。起動コマンドと設定パスは、ローカルの実体名に置き換えてください。

SSH と重いブラウザ IDE のあいだにある「必要十分な層」と考えてください。どこからでも自分のマシンに入り、長時間動いている terminal を再接続し、ファイルを確認し、ちょっとした修正だけを素早く済ませるための道具です。

## これを開く理由

- 別の端末から Codex / Claude Code / Kimi-CLI / OpenCode の session に戻りたい
- `2 / 4` ペインで複数の terminal-first agent を並べて動かしたい
- SSH を張りっぱなしにせず、スクリプトや長時間ジョブの進行を見たい
- 席を離れているときにファイル確認、出力のコピー、軽い修正をしたい
- `openclaw` のような terminal-first ツールを重い IDE なしで扱いたい

## SSH やブラウザ IDE ではなく、なぜ RoamBench か

RoamBench は意図的に狭い製品です。リモート作業で価値が高い部分だけを残し、フルブラウザ IDE にはしません。

| やりたいこと | SSH | VS Code Remote / ブラウザ IDE | RoamBench |
| --- | --- | --- | --- |
| 長時間動いている session に別の端末から戻る | 手間が多い | できるが重い | ここが主目的 |
| `tmux` ベースの復帰を楽にする | 手作業 | 主眼ではない | 組み込み |
| `2 / 4` ペインで agent / CLI ワークフローを回す | 手動セットアップ | IDE 寄り | 組み込み |
| terminal の横でファイルを見て軽く直す | 別ツールが必要 | できるがオーバーヘッドあり | 組み込み |
| 単一ユーザーのセルフホストを軽く保つ | できる | たいてい重い | できる |

マルチユーザー基盤ではありません。大きな編集のためにローカルエディタを置き換えるつもりもありません。モバイルでも扱いやすいように、速さと小ささを優先しています。

## スクリーンショット

長時間タスクや agent を並べる `4` terminal workspace:

![RoamBench workspace screenshot](docs/screenshot-main.png)

同じ workspace へモバイルから再接続:

![RoamBench mobile screenshot](docs/screenshot-mobile.jpg)

## ワークフローの要点

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
cp configs/roambench.quickstart.toml roambench.toml
APP_BIN=<path-to-binary>         # e.g. ./roambench
APP_CONFIG=<path-to-config-file>  # e.g. ./roambench.toml
"$APP_BIN" --password-hash
export ROAMBENCH_USER="$(whoami)"
export ROAMBENCH_PASSWORD_HASH='<generated hash>'
APP_CONFIG=${APP_CONFIG:-roambench.toml}
"$APP_BIN" --config "$APP_CONFIG"
```

この quickstart はローカル / LAN 向けの最短経路です。`allow_all_ips = true` と `allow_insecure_http = true` を使うため、安全な公開運用には英語版 README のフルセットアップと [Deployment Hardening](docs/deployment-hardening.md) を使ってください: [README.md](README.md)

## Positioning

RoamBench は「SSH とフルブラウザ IDE の中間にある、ちょうどよい作業台」を目指しています。特に別端末やモバイルから入るときは、全部入りよりも「すぐ入れる、すぐ状況を見られる、必要なら少しだけ直せる」ことを優先しています。
