#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

TAG=""
OUTPUT_DIR="${REPO_ROOT}/dist/releases"
ARCHES="amd64,arm64"
DATE_OVERRIDE=""
COMMIT_OVERRIDE=""

usage() {
	echo "Usage: ${0} [--tag v0.3.1] [--output-dir dist/releases] [--arches amd64,arm64] [--date YYYYMMDD] [--commit SHORTSHA]" >&2
}

while [[ $# -gt 0 ]]; do
	case "${1}" in
	--tag)
		TAG="${2}"
		shift 2
		;;
	--output-dir)
		OUTPUT_DIR="${2}"
		shift 2
		;;
	--arches)
		ARCHES="${2}"
		shift 2
		;;
	--date)
		DATE_OVERRIDE="${2}"
		shift 2
		;;
	--commit)
		COMMIT_OVERRIDE="${2}"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown argument: ${1}" >&2
		usage
		exit 1
		;;
	esac
done

if [[ -z "${TAG}" ]]; then
	TAG="$(git -C "${REPO_ROOT}" describe --tags --abbrev=0 2>/dev/null || true)"
fi

if [[ -z "${TAG}" ]]; then
	echo "No tag supplied and no git tag found. Pass --tag vX.Y.Z." >&2
	exit 1
fi

VERSION="${TAG#v}"
RELEASE_DATE="${DATE_OVERRIDE:-$(date -u +%Y%m%d)}"
COMMIT_SHORT="${COMMIT_OVERRIDE:-$(git -C "${REPO_ROOT}" rev-parse --short HEAD)}"

IFS=',' read -r -a ARCH_LIST <<< "${ARCHES}"
for arch in "${ARCH_LIST[@]}"; do
	case "${arch}" in
	amd64 | arm64)
		;;
	*)
		echo "Unsupported arch: ${arch}" >&2
		exit 1
		;;
	esac
done

for relpath in \
	README.md \
	LICENSE \
	configs/roambench.example.toml \
	configs/roambench.quickstart.toml \
	configs/roambench.service
do
	if [[ ! -f "${REPO_ROOT}/${relpath}" ]]; then
		echo "Missing required release file: ${relpath}" >&2
		exit 1
	fi
done

mkdir -p "${OUTPUT_DIR}"
STAGE_ROOT="$(mktemp -d)"
trap 'rm -rf "${STAGE_ROOT}"' EXIT

echo "Packaging RoamBench ${TAG} from commit ${COMMIT_SHORT}"

for arch in "${ARCH_LIST[@]}"; do
	bundle_name="roambench-release-${COMMIT_SHORT}-${RELEASE_DATE}-linux-${arch}"
	bundle_dir="${STAGE_ROOT}/${bundle_name}"
	archive_path="${OUTPUT_DIR}/${bundle_name}.tar.gz"
	checksum_path="${archive_path}.sha256"

	mkdir -p "${bundle_dir}"

	echo "Building ${bundle_name}..."
	(
		cd "${REPO_ROOT}"
		CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" \
			go build -trimpath -ldflags "-X main.version=${VERSION}" \
			-o "${bundle_dir}/roambench" ./cmd/roambench
	)

	cp "${REPO_ROOT}/README.md" "${bundle_dir}/README.md"
	cp "${REPO_ROOT}/LICENSE" "${bundle_dir}/LICENSE"
	cp "${REPO_ROOT}/configs/roambench.example.toml" "${bundle_dir}/roambench.example.toml"
	cp "${REPO_ROOT}/configs/roambench.quickstart.toml" "${bundle_dir}/roambench.quickstart.toml"
	cp "${REPO_ROOT}/configs/roambench.service" "${bundle_dir}/roambench.service"

	tar -C "${bundle_dir}" -czf "${archive_path}" .
	sha256sum "${archive_path}" > "${checksum_path}"

	echo "  wrote ${archive_path}"
	echo "  wrote ${checksum_path}"
done

echo "Release artifacts are ready under ${OUTPUT_DIR}"
