#!/usr/bin/env bash
# Download the third-party SOFA reference files used by the test suite into
# testdata/. The files are not committed (licences vary, see
# testdata/PROVENANCE.md); this script is the single source of truth for where
# they come from and what their content must be.
#
# Usage: scripts/fetch-testdata.sh [DEST_DIR]   (default: <repo>/testdata)
#
# Files already present with the expected SHA-256 are skipped. A file whose
# hash does not match is re-downloaded; if the download still does not match,
# the script fails. Entries with an empty hash are not yet pinned: they are
# downloaded and their hash is printed so it can be pinned here and in
# testdata/PROVENANCE.md.
#
# Entries marked "optional" come from hosts that are not reachable from every
# environment (sofacoustics.org answers 403 to GitHub Actions runners): a
# failed download is reported as a warning, not an error, and the tests that
# need the file skip (they are listed in optionalTestdata in
# testhelpers_test.go).
#
# curl retries only transient failures (timeouts, 408, 429, 5xx); any other
# 4xx fails immediately.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="${1:-${repo_root}/testdata}"

libmysofa="https://raw.githubusercontent.com/hoene/libmysofa/648eed03472e6720a1ea45d1a1f86c4efb569ff9"
mesh2hrtf="https://raw.githubusercontent.com/Any2HRTF/Mesh2HRTF/e45d0436a6fbeca3db13828cbae23ca109225be3"

sofacoustics="https://sofacoustics.org/data"

# name | url | sha256 | [optional]
manifest=(
	"tester.sofa|${libmysofa}/tests/tester.sofa|38abbb518e4e43ec2efbd4765e763e764776f18ae0e0181f3684bff644d9c58b"
	"MIT_KEMAR_normal_pinna.sofa|${libmysofa}/share/MIT_KEMAR_normal_pinna.sofa|2768ac841213a7ae11d1ea7fd0f25a69b39216102dc5dd913ea6ba0f0dc57e28"
	"CIPIC_subject_003_hrir_final.sofa|${libmysofa}/tests/CIPIC_subject_003_hrir_final.sofa|0d31149c9893a209642fd65ea64cfe81f3ffd5c6cea74ce374936f2edd5628a1"
	"Mesh2HRTF.sofa|${libmysofa}/tests/Mesh2HRTF.sofa|e4ceee243e445e6bc873db41e61faec7f5ce75ecd9fe174310f1c8936f6b64aa"
	"Mesh2HRTF_HRTF_FourPointHorPlane_r100cm.sofa|${mesh2hrtf}/tests/resources/SHTF/Output2HRTF/HRTF_FourPointHorPlane_r100cm.sofa|e209f2de064f1b82558644e06f5535033839062cf9e1102f53fc2b3e556a2fdf"
	"GeneralTF_2.0.sofa|${sofacoustics}/examples/GeneralTF_2.0.sofa||optional"
	"GeneralTF-E_1.0.sofa|${sofacoustics}/examples/GeneralTF-E_1.0.sofa||optional"
	"FreeFieldHRTF_1.0.sofa|${sofacoustics}/examples/FreeFieldHRTF_1.0.sofa||optional"
	"SimpleFreeFieldHRSOS_1.0.sofa|${sofacoustics}/examples/SimpleFreeFieldHRSOS_1.0.sofa||optional"
	"demo_FreeFieldHRTF_4_SH.sofa|${sofacoustics}/sofatoolbox_test/demo_FreeFieldHRTF_4_SH.sofa||optional"
)

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

mkdir -p "${dest}"
failed=0
unpinned=0
missing_optional=0

for entry in "${manifest[@]}"; do
	IFS='|' read -r name url want optional <<<"${entry}"
	target="${dest}/${name}"

	if [[ -f ${target} && -n ${want} && "$(sha256 "${target}")" == "${want}" ]]; then
		echo "ok       ${name}"
		continue
	fi

	echo "fetch    ${name} <- ${url}"
	tmp="$(mktemp "${dest}/.${name}.XXXXXX")"
	if ! curl --fail --silent --show-error --location \
		--retry 3 --retry-delay 2 \
		--connect-timeout 20 --max-time 600 \
		--output "${tmp}" "${url}"; then
		rm -f "${tmp}"
		if [[ ${optional:-} == optional ]]; then
			echo "WARNING  ${name}: optional file not available from ${url}; tests needing it will be skipped" >&2
			missing_optional=1
		else
			echo "ERROR    ${name}: download failed" >&2
			failed=1
		fi
		continue
	fi

	got="$(sha256 "${tmp}")"
	if [[ -z ${want} ]]; then
		echo "WARNING  ${name}: hash not pinned yet, got sha256=${got}" >&2
		unpinned=1
	elif [[ ${got} != "${want}" ]]; then
		rm -f "${tmp}"
		echo "ERROR    ${name}: sha256 mismatch: got ${got}, want ${want}" >&2
		failed=1
		continue
	fi
	mv -f "${tmp}" "${target}"
done

if [[ ${unpinned} -ne 0 ]]; then
	echo "Some files are not pinned; add their sha256 to scripts/fetch-testdata.sh and testdata/PROVENANCE.md." >&2
fi
if [[ ${missing_optional} -ne 0 ]]; then
	echo "Some optional files could not be fetched (see testdata/PROVENANCE.md)." >&2
fi
if [[ ${failed} -ne 0 ]]; then
	echo "fetch-testdata: one or more files could not be fetched or verified" >&2
	exit 1
fi
echo "required testdata present in ${dest}"
