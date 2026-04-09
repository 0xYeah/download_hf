#!/bin/bash
set -e

product_name=$(grep ProjectName ./config/config.go | awk -F '"' '{print $2}' | sed 's/\"//g')
CURRENT_VERSION=$(grep ProjectVersion ./config/config.go | awk -F '"' '{print $2}' | sed 's/\"//g')
build_path=./build
RUN_MODE=release
upload_dir="${build_path}/upload_tmp_dir"

OS_TYPE="Unknown"
GetOSType() {
    uNames=$(uname -s)
    osName=${uNames: 0: 4}
    if [ "$osName" == "Darw" ]; then
        OS_TYPE="Darwin"
    elif [ "$osName" == "Linu" ]; then
        OS_TYPE="Linux"
    elif [ "$osName" == "MING" ]; then
        OS_TYPE="Windows"
    else
        OS_TYPE="Unknown"
    fi
}
GetOSType

# package_zip <src_dir> <os> <arch>
function package_zip() {
    local src_dir="$1" os="$2" arch="$3"
    local zip_name="${product_name}_${RUN_MODE}_${CURRENT_VERSION}_${os}_${arch}.zip"
    local stage="${src_dir}/../${product_name}"
    mkdir -p "${stage}"
    if [[ "$os" == "windows" ]]; then
        cp "${src_dir}/${product_name}.exe" "${stage}/${product_name}.exe"
    else
        cp "${src_dir}/${product_name}" "${stage}/${product_name}"
    fi
    (cd "${src_dir}/.." && zip -r "${upload_dir}/${zip_name}" "${product_name}" >/dev/null)
    rm -rf "${stage}"
    echo "==> packaged ${zip_name}"
}

function build_darwin() {
    echo "==> darwin amd64 + arm64"
    mkdir -p "${build_path}/${RUN_MODE}/darwin/amd64"
    mkdir -p "${build_path}/${RUN_MODE}/darwin/arm64"

    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o "${build_path}/${RUN_MODE}/darwin/amd64/${product_name}" -trimpath -ldflags "${ld_flag_master}" main.go &
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o "${build_path}/${RUN_MODE}/darwin/arm64/${product_name}" -trimpath -ldflags "${ld_flag_master}" main.go &
    wait

    chmod a+x "${build_path}/${RUN_MODE}/darwin/amd64/${product_name}"
    chmod a+x "${build_path}/${RUN_MODE}/darwin/arm64/${product_name}"

    if command -v lipo &>/dev/null; then
        mkdir -p "${build_path}/${RUN_MODE}/darwin/universal"
        lipo -create \
            -output "${build_path}/${RUN_MODE}/darwin/universal/${product_name}" \
            "${build_path}/${RUN_MODE}/darwin/amd64/${product_name}" \
            "${build_path}/${RUN_MODE}/darwin/arm64/${product_name}"
        chmod a+x "${build_path}/${RUN_MODE}/darwin/universal/${product_name}"
        echo "==> darwin universal merged"
        package_zip "${build_path}/${RUN_MODE}/darwin/universal" darwin universal
    else
        package_zip "${build_path}/${RUN_MODE}/darwin/amd64" darwin amd64
        package_zip "${build_path}/${RUN_MODE}/darwin/arm64" darwin arm64
    fi
}

function build_linux() {
    echo "==> linux amd64 + arm64"
    mkdir -p "${build_path}/${RUN_MODE}/linux/amd64"
    mkdir -p "${build_path}/${RUN_MODE}/linux/arm64"

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${build_path}/${RUN_MODE}/linux/amd64/${product_name}" -trimpath -ldflags "${ld_flag_master}" main.go &
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "${build_path}/${RUN_MODE}/linux/arm64/${product_name}" -trimpath -ldflags "${ld_flag_master}" main.go &
    wait

    chmod a+x "${build_path}/${RUN_MODE}/linux/amd64/${product_name}"
    chmod a+x "${build_path}/${RUN_MODE}/linux/arm64/${product_name}"

    package_zip "${build_path}/${RUN_MODE}/linux/amd64" linux amd64
    package_zip "${build_path}/${RUN_MODE}/linux/arm64" linux arm64
}

function build_windows() {
    echo "==> windows amd64 + arm64"
    mkdir -p "${build_path}/${RUN_MODE}/windows/amd64"
    mkdir -p "${build_path}/${RUN_MODE}/windows/arm64"

    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "${build_path}/${RUN_MODE}/windows/amd64/${product_name}.exe" -trimpath -ldflags "${ld_flag_master}" main.go &
    CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o "${build_path}/${RUN_MODE}/windows/arm64/${product_name}.exe" -trimpath -ldflags "${ld_flag_master}" main.go &
    wait

    package_zip "${build_path}/${RUN_MODE}/windows/amd64" windows amd64
    package_zip "${build_path}/${RUN_MODE}/windows/arm64" windows arm64
}

function toBuild() {
    upload_dir="$(pwd)/build/upload_tmp_dir"
    rm -rf "${build_path:?}/${RUN_MODE}"
    rm -rf "${upload_dir}"
    mkdir -p "${build_path}/${RUN_MODE}"
    mkdir -p "${upload_dir}"

    go_version=$(go version | awk '{print $3}')
    commit_hash=$(git show -s --format=%H)
    commit_date=$(git show -s --format="%ci")

    if [[ "$OS_TYPE" == "Darwin" ]]; then
        formatted_time=$(date -u -j -f "%Y-%m-%d %H:%M:%S %z" "${commit_date}" "+%Y-%m-%d_%H:%M:%S")
    else
        formatted_time=$(date -u -d "${commit_date}" "+%Y-%m-%d_%H:%M:%S")
    fi

    build_time=$(date -u +"%Y-%m-%d_%H:%M:%S")
    ld_flag_master="-X main.mGitCommitHash=${commit_hash} -X main.mGitCommitTime=${formatted_time} -X main.mGoVersion=${go_version} -X main.mPackageOS=${OS_TYPE} -X main.mPackageTime=${build_time} -X main.mRunMode=${RUN_MODE} -X main.mVersion=${CURRENT_VERSION} -s -w"

    build_darwin &
    build_linux &
    build_windows &
    wait

    echo ""
    echo "==> Done. Packages:"
    find "${upload_dir}" -name "*.zip" | sort
}

function handlerunMode() {
    if [[ "$1" == "release" || "$1" == "" ]]; then
        RUN_MODE=release
    elif [[ "$1" == "test" ]]; then
        RUN_MODE=test
    elif [[ "$1" == "debug" ]]; then
        RUN_MODE=debug
    else
        echo "Usage: bash build.sh [release|test|debug], default is release"
        exit 1
    fi
}

handlerunMode "$1" && toBuild
