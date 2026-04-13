#!/usr/bin/env bash

set -euo pipefail

REPO="ianfksyd/roambench"
REQUESTED_TAG="${1:-latest}"
TARGET_DIR="${2:-/usr/local/bin}"

case "$(uname -m)" in
	x86_64 | amd64)
		ARCH="amd64"
		;;
	aarch64 | arm64)
		ARCH="arm64"
		;;
	*)
		echo "不支持的架构: $(uname -m)" >&2
		exit 1
		;;
esac

ARCHIVE_SUFFIX="linux-${ARCH}.tar.gz"
CHECKSUM_SUFFIX="linux-${ARCH}.tar.gz.sha256"

if ! command -v curl >/dev/null 2>&1; then
	echo "请先安装 curl" >&2
	exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
	echo "请先安装 jq" >&2
	exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
	echo "请先安装 sha256sum(coreutils)" >&2
	exit 1
fi

if [[ "${REQUESTED_TAG}" == "latest" ]]; then
	API_URL="https://api.github.com/repos/${REPO}/releases/latest"
else
	API_URL="https://api.github.com/repos/${REPO}/releases/tags/${REQUESTED_TAG}"
fi

RELEASE_JSON="$(mktemp)"
TMP_DIR="$(mktemp -d)"
trap 'rm -f "${RELEASE_JSON}"; rm -rf "${TMP_DIR}"' EXIT

echo "获取 ${REQUESTED_TAG} 发布清单..."
curl -fsSL "${API_URL}" -o "${RELEASE_JSON}"

RESOLVED_TAG="$(jq -r '.tag_name' "${RELEASE_JSON}")"
if [[ -z "${RESOLVED_TAG}" || "${RESOLVED_TAG}" == "null" ]]; then
	echo "未能解析发布版本，请检查 GitHub API 响应" >&2
	exit 1
fi

ARCHIVE="$(jq -r --arg s "${ARCHIVE_SUFFIX}" '.assets[] | select(.name | endswith($s)) | .name' "${RELEASE_JSON}" | head -n 1)"
CHECKSUM_FILE="$(jq -r --arg s "${CHECKSUM_SUFFIX}" '.assets[] | select(.name | endswith($s)) | .name' "${RELEASE_JSON}" | head -n 1)"

if [[ -z "${ARCHIVE}" || "${ARCHIVE}" == "null" ]]; then
	echo "未找到适用于架构 ${ARCH} 的发布包，请确认 release 已上传 ${ARCHIVE_SUFFIX}" >&2
	exit 1
fi
if [[ -z "${CHECKSUM_FILE}" || "${CHECKSUM_FILE}" == "null" ]]; then
	echo "未找到适用于架构 ${ARCH} 的校验文件，请确认 release 已上传 ${CHECKSUM_SUFFIX}" >&2
	exit 1
fi

ARCHIVE_URL="$(jq -r --arg n "${ARCHIVE}" '.assets[] | select(.name == $n) | .browser_download_url' "${RELEASE_JSON}")"
CHECKSUM_URL="$(jq -r --arg n "${CHECKSUM_FILE}" '.assets[] | select(.name == $n) | .browser_download_url' "${RELEASE_JSON}")"

if [[ -z "${ARCHIVE_URL}" || "${ARCHIVE_URL}" == "null" || -z "${CHECKSUM_URL}" || "${CHECKSUM_URL}" == "null" ]]; then
	echo "未能解析到下载链接，请检查 GitHub API 响应" >&2
	exit 1
fi

echo "下载 ${ARCHIVE}..."
curl -fsSL "${ARCHIVE_URL}" -o "${TMP_DIR}/${ARCHIVE}"
echo "下载校验和..."
curl -fsSL "${CHECKSUM_URL}" -o "${TMP_DIR}/${CHECKSUM_FILE}"

expected="$(awk '{print $1}' "${TMP_DIR}/${CHECKSUM_FILE}")"
actual="$(sha256sum "${TMP_DIR}/${ARCHIVE}" | awk '{print $1}')"
if [[ "${expected}" != "${actual}" ]]; then
	echo "SHA256 校验失败：" >&2
	echo "  expected: ${expected}" >&2
	echo "  actual:   ${actual}" >&2
	exit 1
fi

tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "${TMP_DIR}"

if [[ ! -x "${TMP_DIR}/roambench" ]]; then
	echo "提取后的 roambench 不存在或不可执行" >&2
	exit 1
fi

if [[ ! -w "${TARGET_DIR}" ]]; then
	if ! command -v sudo >/dev/null 2>&1; then
		echo "目标路径 ${TARGET_DIR} 不可写，请使用 sudo 或提供可写目录" >&2
		exit 1
	fi
	sudo install -m 0755 "${TMP_DIR}/roambench" "${TARGET_DIR}/roambench"
else
	install -m 0755 "${TMP_DIR}/roambench" "${TARGET_DIR}/roambench"
fi

BINARY_PATH="${TARGET_DIR%/}/roambench"

cat <<EOF
安装完成: ${BINARY_PATH}
发布版本: ${RESOLVED_TAG}

使用示例:
  cp ${TMP_DIR}/roambench.example.toml roambench.toml
  ${BINARY_PATH} --password-hash
  ${BINARY_PATH} --config roambench.toml
EOF
