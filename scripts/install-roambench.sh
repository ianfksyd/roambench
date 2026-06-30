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
		echo "Unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
esac

ARCHIVE_SUFFIX="linux-${ARCH}.tar.gz"
CHECKSUM_SUFFIX="linux-${ARCH}.tar.gz.sha256"

if ! command -v curl >/dev/null 2>&1; then
	echo "Please install curl first." >&2
	exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
	echo "Please install jq first." >&2
	exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
	echo "Please install sha256sum from coreutils first." >&2
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

echo "Fetching ${REQUESTED_TAG} release manifest..."
curl -fsSL "${API_URL}" -o "${RELEASE_JSON}"

RESOLVED_TAG="$(jq -r '.tag_name' "${RELEASE_JSON}")"
if [[ -z "${RESOLVED_TAG}" || "${RESOLVED_TAG}" == "null" ]]; then
	echo "Could not resolve release tag from the GitHub API response." >&2
	exit 1
fi

ARCHIVE="$(jq -r --arg s "${ARCHIVE_SUFFIX}" '.assets[] | select(.name | endswith($s)) | .name' "${RELEASE_JSON}" | head -n 1)"
CHECKSUM_FILE="$(jq -r --arg s "${CHECKSUM_SUFFIX}" '.assets[] | select(.name | endswith($s)) | .name' "${RELEASE_JSON}" | head -n 1)"

if [[ -z "${ARCHIVE}" || "${ARCHIVE}" == "null" ]]; then
	echo "Could not find a release archive for architecture ${ARCH} ending with ${ARCHIVE_SUFFIX}." >&2
	exit 1
fi
if [[ -z "${CHECKSUM_FILE}" || "${CHECKSUM_FILE}" == "null" ]]; then
	echo "Could not find a checksum file for architecture ${ARCH} ending with ${CHECKSUM_SUFFIX}." >&2
	exit 1
fi

ARCHIVE_URL="$(jq -r --arg n "${ARCHIVE}" '.assets[] | select(.name == $n) | .browser_download_url' "${RELEASE_JSON}")"
CHECKSUM_URL="$(jq -r --arg n "${CHECKSUM_FILE}" '.assets[] | select(.name == $n) | .browser_download_url' "${RELEASE_JSON}")"

if [[ -z "${ARCHIVE_URL}" || "${ARCHIVE_URL}" == "null" || -z "${CHECKSUM_URL}" || "${CHECKSUM_URL}" == "null" ]]; then
	echo "Could not resolve release download URLs from the GitHub API response." >&2
	exit 1
fi

echo "Downloading ${ARCHIVE}..."
curl -fsSL "${ARCHIVE_URL}" -o "${TMP_DIR}/${ARCHIVE}"
echo "Downloading checksum..."
curl -fsSL "${CHECKSUM_URL}" -o "${TMP_DIR}/${CHECKSUM_FILE}"

expected="$(awk '{print $1}' "${TMP_DIR}/${CHECKSUM_FILE}")"
actual="$(sha256sum "${TMP_DIR}/${ARCHIVE}" | awk '{print $1}')"
if [[ "${expected}" != "${actual}" ]]; then
	echo "SHA256 checksum verification failed:" >&2
	echo "  expected: ${expected}" >&2
	echo "  actual:   ${actual}" >&2
	exit 1
fi

tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "${TMP_DIR}"

if [[ ! -x "${TMP_DIR}/roambench" ]]; then
	echo "Extracted roambench binary is missing or not executable." >&2
	exit 1
fi

if [[ ! -d "${TARGET_DIR}" ]]; then
	if ! mkdir -p "${TARGET_DIR}" 2>/dev/null; then
		if ! command -v sudo >/dev/null 2>&1; then
			echo "Target directory ${TARGET_DIR} does not exist and could not be created." >&2
			exit 1
		fi
		sudo mkdir -p "${TARGET_DIR}"
	fi
fi

if [[ ! -w "${TARGET_DIR}" ]]; then
	if ! command -v sudo >/dev/null 2>&1; then
		echo "Target directory ${TARGET_DIR} is not writable. Use sudo or pass a writable directory." >&2
		exit 1
	fi
	sudo install -m 0755 "${TMP_DIR}/roambench" "${TARGET_DIR}/roambench"
else
	install -m 0755 "${TMP_DIR}/roambench" "${TARGET_DIR}/roambench"
fi

BINARY_PATH="${TARGET_DIR%/}/roambench"

cat <<EOF
Installed: ${BINARY_PATH}
Release: ${RESOLVED_TAG}

Quickstart example:
  curl -fsSLo roambench.toml https://raw.githubusercontent.com/${REPO}/${RESOLVED_TAG}/configs/roambench.quickstart.toml
  ${BINARY_PATH} --password-hash
  export ROAMBENCH_USER="\$(whoami)"
  export ROAMBENCH_PASSWORD_HASH='<paste the generated hash>'
  ${BINARY_PATH} --config roambench.toml
EOF
