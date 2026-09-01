#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_ROOT="$(mktemp -d /tmp/roambench-release-symbol-test.XXXXXX)"

cleanup() {
	case "${TEST_ROOT}" in
	/tmp/roambench-release-symbol-test.*)
		rm -rf -- "${TEST_ROOT}"
		;;
	esac
}
trap cleanup EXIT

readonly TEST_TAG=v9.9.9-test
readonly TEST_VERSION=9.9.9-test
readonly TEST_DATE=20000101
readonly TEST_COMMIT=deadbee
readonly TEST_ARCH=arm64
readonly ASSET_DIR="${TEST_ROOT}/assets"
readonly BUNDLE_NAME="roambench-release-${TEST_COMMIT}-${TEST_DATE}-linux-${TEST_ARCH}"
readonly ARCHIVE_PATH="${ASSET_DIR}/${BUNDLE_NAME}.tar.gz"
readonly EXTRACT_DIR="${TEST_ROOT}/extracted"
readonly BINARY_PATH="${EXTRACT_DIR}/roambench"
readonly SYMBOLS_PATH="${TEST_ROOT}/symbols.txt"
readonly NM_ERROR_PATH="${TEST_ROOT}/nm.stderr"

mkdir -p "${EXTRACT_DIR}"

"${REPO_ROOT}/scripts/package-roambench-release.sh" \
	--tag "${TEST_TAG}" \
	--output-dir "${ASSET_DIR}" \
	--arches "${TEST_ARCH}" \
	--date "${TEST_DATE}" \
	--commit "${TEST_COMMIT}"

tar -C "${EXTRACT_DIR}" -xzf "${ARCHIVE_PATH}"

if ! go tool nm "${BINARY_PATH}" >"${SYMBOLS_PATH}" 2>"${NM_ERROR_PATH}"; then
	printf 'release binary must retain Go symbols for precise vulnerability scanning\n' >&2
	cat "${NM_ERROR_PATH}" >&2
	exit 1
fi

symbol_count="$(wc -l <"${SYMBOLS_PATH}")"
if ((symbol_count < 1000)); then
	printf 'release binary contains too few Go symbols: %s\n' "${symbol_count}" >&2
	exit 1
fi

if ! strings "${BINARY_PATH}" | grep -Fx "${TEST_VERSION}" >/dev/null; then
	printf 'release binary does not embed version %s\n' "${TEST_VERSION}" >&2
	exit 1
fi

printf 'release binary retains %s Go symbols and embeds version %s\n' \
	"${symbol_count}" "${TEST_VERSION}"
