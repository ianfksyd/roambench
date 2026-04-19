# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

> 長時間走る AI agent の作業を、どこからでも開始・監視・復帰できる軽量ワークベンチ。

RoamBench は、複数の AI coding agent を長時間タスクで並行運用する開発者向けのセルフホスト型ワークベンチです。terminal セッションを生かし続け、どの端末からでも再接続でき、重いブラウザ IDE を持ち出さなくても最低限のファイル操作ができます。

## 解決する課題

Codex、Claude Code、OpenCode、Kimi-CLI などの terminal-first ツールをリモートマシンで動かしていると、こういう問題が繰り返し起きます:

- ノート PC を閉じたり端末を変えるとセッションが切れる
- 複数 agent を同時に走らせると追跡しきれない
- 数時間〜数日に及ぶ長時間タスクは、状態を失わずに再接続できる必要がある
- スマホから出力確認、ファイル閲覧、軽い修正をしたい

RoamBench は `tmux` の上に薄い web レイヤーを乗せることで、すべてを生きたまま・アクセス可能な状態に保ちます。

## SSH やブラウザ IDE ではなく、なぜ RoamBench か

RoamBench は意図的に狭い製品です。リモート作業で価値が高い部分だけを残し、フルブラウザ IDE にはしません。

| やりたいこと | SSH | VS Code Remote / ブラウザ IDE | RoamBench |
| --- | --- | --- | --- |
| 長時間セッションにスマホから戻る | 手間が多い | できるが重い | ここが主目的 |
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

## 現在の機能

### Terminal とセッション管理

- `password` または `pam` の単一ユーザー認証、IP 許可リスト対応
- `tmux` が使える環境では、ページ更新・再接続・サーバー再起動後もセッションを復元
- terminal メタデータをディスクに永続化（ストレージ上限あり）
- `1 / 2 / 4` レイアウトの workspace タブ、ブラウザ間同期

### ファイルワークスペース

- ディレクトリ一覧（ソート、隠しファイル切替、パンくずナビ、フィルタ）
- テキスト編集（下書き復元、find / replace、go-to-line、行番号）
- 新規ファイル / フォルダ、名前を付けて保存、リネーム / 移動、コピー、アップロード、ダウンロード、削除
- 画像プレビュー

### Agent ワークフロー

- Codex、Claude Code、Kimi-CLI、OpenCode など複数の terminal-first ツールを並行運用
- `openclaw` のようなツールを重い IDE なしで扱える
- 別端末やスマホから長時間 agent セッションに復帰
- スクリプト、データジョブ、長時間 CLI タスクの進行監視

### その他

- 軽量・低オーバーヘッド設計
- ヘッダーにリアルタイムメモリ表示
- 英語・簡体字中国語・日本語の UI 切り替え

## ロードマップ: プロジェクト制御レイヤーへ

RoamBench は現在、実行レイヤーを提供しています: 永続 terminal、マルチペイン workspace、ファイルツール。次の大きな進化は、この基盤の上に**プロジェクト制御レイヤー**を追加し、複雑なマルチ agent 作業の管理をアドホックではなく構造化することです。

計画中の方向性:

- **タスク優先モデル** — terminal タブではなく、目標・状態・証拠を持つタスクで作業を整理
- **タイムラインと証拠** — 長い CLI 出力を読まなくても、何が起きたか・何が変わったか・agent が何を主張しているかが分かる
- **ヒューマンチェックポイント** — 本当に人間の判断が必要なときだけ通知
- **共有プロジェクト履歴** — agent やセッションをまたいで判断・失敗・復旧を追跡
- **ローカル + リモート Runtime** — 同一インターフェースからローカルマシンとリモートサーバーの agent を管理
- **Agent 中立** — 特定の AI プロバイダーに依存しない; terminal-first な agent なら何でも

terminal レイヤーはなくなりません。タスク内の一つのビューになり、製品表面全体ではなくなります。

詳しい設計議論は [`docs/project-control-discussions/`](docs/project-control-discussions/) を参照。

## 必要環境

- Go `1.22+`
- `tmux` 推奨（terminal セッションの永続化に必要）
- Linux またはPTY対応の Unix 系 OS

`tmux` がなくても動きますが、セッション永続化の信頼性が下がります。

## Quick Start

```bash
make build
cp configs/roambench.quickstart.toml roambench.toml
APP_BIN=<path-to-binary>         # e.g. ./roambench
APP_CONFIG=<path-to-config-file>  # e.g. ./roambench.toml
"$APP_BIN" --password-hash
export ROAMBENCH_USER="$(whoami)"
export ROAMBENCH_PASSWORD_HASH='<生成されたハッシュ>'
APP_CONFIG=${APP_CONFIG:-roambench.toml}
"$APP_BIN" --config "$APP_CONFIG"
```

この quickstart はローカル / LAN 向けの最短経路です。`allow_all_ips = true` と `allow_insecure_http = true` を使うため、安全な公開運用には英語版 README のフルセットアップと [Deployment Hardening](docs/deployment-hardening.md) を参照してください。

## 完全セットアップ

1. バイナリをビルド:

   ```bash
   make build
   ```

2. 設定例をコピー:

   ```bash
   cp configs/roambench.example.toml roambench.toml
   ```

3. バイナリと設定ファイルをセット:

   ```bash
   APP_BIN=<path-to-binary>         # e.g. ./roambench
   APP_CONFIG=${APP_CONFIG:-roambench.toml}
   ```

4. `roambench.toml` を編集:

   - `[auth].single_user` を現在 `$APP_BIN` を実行している Unix ユーザーに設定
   - `[server].allowed_ips` を設定、または信頼できるテスト環境のみ `allow_all_ips = true`
   - terminal 永続化設定を確認

5. パスワードハッシュを生成:

   ```bash
   "$APP_BIN" --password-hash
   ```

6. 生成されたハッシュを `roambench.toml` の `password_hash` に記入

7. RoamBench を起動:

   ```bash
   "$APP_BIN" --config "$APP_CONFIG"
   ```

8. ブラウザでサーバーを開く

## ビルドと実行

```bash
make build
make run
go test ./...
```

PAM ビルド:

```bash
make build-pam
```

## セキュリティ

- 単一ユーザー専用
- デフォルトで IP 許可リストを有効に
- loopback 以外では `allow_insecure_http = true` を明示しない限り TLS を要求

## プロジェクト構成

- [cmd/roambench](cmd/roambench) - CLI エントリポイント
- [internal/auth](internal/auth) - 認証とセッション
- [internal/server](internal/server) - HTTP サーバーと API
- [internal/terminal](internal/terminal) - terminal セッション管理
- [internal/filebrowser](internal/filebrowser) - ファイルブラウザバックエンド
- [web](web) - 埋め込みフロントエンド

## その他

- [Roadmap](docs/roadmap.md)
- [Configuration Guide](docs/configuration.md)
- [Authentication Guide](docs/authentication.md)
- [Deployment Hardening](docs/deployment-hardening.md)
- [All Documentation](docs/README.md)
- [Contributing](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)

## 現在のステータス

RoamBench は単一ユーザーのセルフホスト運用にすでに使えます。特に terminal-first のコーディング、agent の監視、軽量なリモート介入に向いています。現在はコンパクトで意見の強い設計を保ちつつ、マルチ agent 開発作業のプロジェクト制御レイヤーへと進化する明確な道筋を持っています。
