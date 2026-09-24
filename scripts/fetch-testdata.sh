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
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="${1:-${repo_root}/testdata}"

libmysofa="https://raw.githubusercontent.com/hoene/libmysofa/648eed03472e6720a1ea45d1a1f86c4efb569ff9"
sofacoustics="https://sofacoustics.org/data"

# name | url | sha256
manifest=(
	"tester.sofa|${libmysofa}/tests/tester.sofa|38abbb518e4e43ec2efbd4765e763e764776f18ae0e0181f3684bff644d9c58b"
	"MIT_KEMAR_normal_pinna.sofa|${libmysofa}/share/MIT_KEMAR_normal_pinna.sofa|2768ac841213a7ae11d1ea7fd0f25a69b39216102dc5dd913ea6ba0f0dc57e28"
	"CIPIC_subject_003_hrir_final.sofa|${libmysofa}/tests/CIPIC_subject_003_hrir_final.sofa|0d31149c9893a209642fd65ea64cfe81f3ffd5c6cea74ce374936f2edd5628a1"
	"Mesh2HRTF.sofa|${libmysofa}/tests/Mesh2HRTF.sofa|e4ceee243e445e6bc873db41e61faec7f5ce75ecd9fe174310f1c8936f6b64aa"
	"GeneralTF_2.0.sofa|${sofacoustics}/examples/GeneralTF_2.0.sofa|"
	"GeneralTF-E_1.0.sofa|${sofacoustics}/examples/GeneralTF-E_1.0.sofa|"
	"FreeFieldHRTF_1.0.sofa|${sofacoustics}/examples/FreeFieldHRTF_1.0.sofa|"
	"SimpleFreeFieldHRSOS_1.0.sofa|${sofacoustics}/examples/SimpleFreeFieldHRSOS_1.0.sofa|"
	"FreeFieldHRTF_2.0.sofa|${sofacoustics}/sofatoolbox_test/demo_FreeFieldHRTF_2_SimpleFreeFieldHRTF.sofa|"
	"demo_FreeFieldHRTF_4_SH.sofa|${sofacoustics}/sofatoolbox_test/demo_FreeFieldHRTF_4_SH.sofa|"
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

for entry in "${manifest[@]}"; do
	IFS='|' read -r name url want <<<"${entry}"
	target="${dest}/${name}"

	if [[ -f ${target} && -n ${want} && "$(sha256 "${target}")" == "${want}" ]]; then
		echo "ok       ${name}"
		continue
	fi

	echo "fetch    ${name} <- ${url}"
	tmp="$(mktemp "${dest}/.${name}.XXXXXX")"
	if ! curl --fail --silent --show-error --location \
		--retry 5 --retry-delay 2 --retry-all-errors \
		--connect-timeout 20 --max-time 600 \
		--output "${tmp}" "${url}"; then
		rm -f "${tmp}"
		echo "ERROR    ${name}: download failed" >&2
		failed=1
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
if [[ ${failed} -ne 0 ]]; then
	echo "fetch-testdata: one or more files could not be fetched or verified" >&2
	exit 1
fi
echo "testdata complete in ${dest}"
