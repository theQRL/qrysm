#!/bin/bash

set -eu

# Use this script to download the latest Qrysm release binary.
# Usage: ./qrysm.sh PROCESS [--download-only] FLAGS
#   PROCESS can be one of beacon-chain or validator.
#   FLAGS are the flags or arguments passed to the PROCESS.
#   If --download-only flag is passed, binaries are checked for updates,
#   downloaded if necessary, no process is started.
# Downloaded binaries are saved to ./dist.
# Use USE_QRYSM_VERSION to specify a specific release version.
#   Example: USE_QRYSM_VERSION=v0.3.3 ./qrysm.sh beacon-chain

readonly THEQRL_SIGNING_KEY=C6A8251946339065ECE9B1553D2EF77E42EB27AE

function color() {
    # Usage: color "31;5" "string"
    # Some valid values for color:
    # - 5 blink, 1 strong, 4 underlined
    # - fg: 31 red,  32 green, 33 yellow, 34 blue, 35 purple, 36 cyan, 37 white
    # - bg: 40 black, 41 red, 44 blue, 45 purple
    printf '\033[%sm%s\033[0m\n' "$@"
}

# `readlink -f` that works on OSX too.
function get_realpath() {
    if [ "$(uname -s)" == "Darwin" ]; then
        local queue="$1"
        if [[ "${queue}" != /* ]]; then
            # Make sure we start with an absolute path.
            queue="${PWD}/${queue}"
        fi
        local current=""
        while [ -n "${queue}" ]; do
            # Removing a trailing /.
            queue="${queue#/}"
            # Pull the first path segment off of queue.
            local segment="${queue%%/*}"
            # If this is the last segment.
            if [[ "${queue}" != */* ]]; then
                segment="${queue}"
                queue=""
            else
                # Remove that first segment.
                queue="${queue#*/}"
            fi
            local link="${current}/${segment}"
            if [ -h "${link}" ]; then
                link="$(readlink "${link}")"
                queue="${link}/${queue}"
                if [[ "${link}" == /* ]]; then
                    current=""
                fi
            else
                current="${link}"
            fi
        done

        echo "${current}"
    else
        readlink -f "$1"
    fi
}

# Complain if no arguments were provided.
if [ "$#" -lt 1 ]; then
    color "31" "Usage: ./qrysm.sh PROCESS FLAGS."
    color "31" "       ./qrysm.sh PROCESS --download-only."
    color "31" "PROCESS can be beacon-chain, validator, or client-stats."
    exit 1
fi

readonly wrapper_dir="$(dirname "$(get_realpath "${BASH_SOURCE[0]}")")/dist"


arch=$(uname -m)
arch=${arch/x86_64/amd64}
arch=${arch/aarch64/arm64}

readonly os_arch_suffix="$(uname -s | tr '[:upper:]' '[:lower:]')-$arch"

system=""
case "$OSTYPE" in
darwin*) system="darwin" ;;
linux*) system="linux" ;;
msys*) system="windows" ;;
cygwin*) system="windows" ;;
*) exit 1 ;;
esac
readonly system

if [ "$system" == "windows" ]; then
    arch="amd64.exe"
elif [[ "$os_arch_suffix" == *"arm64"* ]]; then
    arch="arm64"
fi

if [[ "$arch" == "armv7l" ]]; then
    color "31" "32-bit ARM is not supported. Please install a 64-bit operating system."
    exit 1
fi

mkdir -p "$wrapper_dir"

function get_qrysm_version() {
    if [[ -n ${USE_QRYSM_VERSION:-} ]]; then
        readonly reason="specified in \$USE_QRYSM_VERSION"
        readonly qrysm_version="${USE_QRYSM_VERSION}"
    else
        # Find the latest Qrysm version available for download.
        readonly reason="automatically selected latest available version"
        #qrysm_version=$(curl -f -s https://prysmaticlabs.com/releases/latest) || (color "31" "Starting qrysm requires an internet connection. If you are being blocked by your antivirus, you can download the beacon chain and validator executables from our releases page on Github here https://github.com/theQRL/qrysm/releases/" && exit 1)
        qrysm_version=$(curl -f -s https://api.github.com/repos/theQRL/qrysm/releases/latest -s | jq .name -r) || (color "31" "Starting qrysm requires an internet connection. If you are being blocked by your antivirus, you can download the beacon chain and validator executables from our releases page on Github here https://github.com/theQRL/qrysm/releases/" && exit 1)
        readonly qrysm_version
    fi
}

function verify() {
    file=$1

    skip=${QRYSM_ALLOW_UNVERIFIED_BINARIES-0}
    if [[ $skip == 1 ]]; then
        return 0
    fi
    checkSum="shasum -a 256"
    hash shasum 2>/dev/null || {
        checkSum="sha256sum"
        hash sha256sum 2>/dev/null || {
            echo >&2 "SHA checksum utility not available. Either install one (shasum or sha256sum) or run with QRYSM_ALLOW_UNVERIFIED_BINARIES=1."
            exit 1
        }
    }
    hash gpg 2>/dev/null || {
        echo >&2 "gpg is not available. Either install it or run with QRYSM_ALLOW_UNVERIFIED_BINARIES=1."
        exit 1
    }

    color "37" "Verifying binary integrity."

    gpg --list-keys "$THEQRL_SIGNING_KEY" >/dev/null 2>&1 || curl --silent https://prysmaticlabs.com/releases/pgp_keys.asc | gpg --import
    (
        cd "$wrapper_dir"
        $checkSum -c "${file}.sha256" || failed_verification
    )
    (
        cd "$wrapper_dir"
        gpg -u "$THEQRL_SIGNING_KEY" --verify "${file}.sig" "$file" || failed_verification
    )

    color "32;1" "Verified ${file} has been signed by the QRL."
}

function failed_verification() {
    MSG=$(
        cat <<-END
Failed to verify Qrysm binary. Please erase downloads in the
dist directory and run this script again. Alternatively, you can use a
A prior version by specifying environment variable USE_QRYSM_VERSION
with the specific version, as desired. Example: USE_QRYSM_VERSION=v1.0.0-alpha.5
If you must wish to continue running an unverified binary, specific the
environment variable QRYSM_ALLOW_UNVERIFIED_BINARIES=1
END
    )
    color "31" "$MSG"
    exit 1
}

get_qrysm_version

color "37" "Latest Qrysm version is $qrysm_version."

BEACON_CHAIN_REAL="${wrapper_dir}/beacon-chain-${qrysm_version}-${system}-${arch}"
VALIDATOR_REAL="${wrapper_dir}/validator-${qrysm_version}-${system}-${arch}"
CLIENT_STATS_REAL="${wrapper_dir}/client-stats-${qrysm_version}-${system}-${arch}"

if [[ $1 == beacon-chain ]]; then
    if [[ ! -x $BEACON_CHAIN_REAL ]]; then
        color "34" "Downloading beacon chain@${qrysm_version} to ${BEACON_CHAIN_REAL} (${reason})"
        file=beacon-chain-${qrysm_version}-${system}-${arch}
        #res=$(curl -w '%{http_code}\n' -f -L "https://prysmaticlabs.com/releases/${file}"  -o "$BEACON_CHAIN_REAL" | ( grep 404 || true ) )
        res=$(curl -w '%{http_code}\n' -f -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}"  -o "$BEACON_CHAIN_REAL" | ( grep 404 || true ) )
        if [[ $res == 404 ]];then
            echo "No qrysm beacon chain found for ${qrysm_version},(${file}) exit"
            exit 1
        fi
        #curl --silent -L "https://prysmaticlabs.com/releases/${file}.sha256" -o "${wrapper_dir}/${file}.sha256"
        #curl --silent -L "https://prysmaticlabs.com/releases/${file}.sig" -o "${wrapper_dir}/${file}.sig"
        curl --silent -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}.sha256" -o "${wrapper_dir}/${file}.sha256"
        curl --silent -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}.sig" -o "${wrapper_dir}/${file}.sig"
        chmod +x "$BEACON_CHAIN_REAL"
    else
        color "37" "Beacon chain is up to date."
    fi
fi

if [[ $1 == validator ]]; then
    if [[ ! -x $VALIDATOR_REAL ]]; then
        color "34" "Downloading validator@${qrysm_version} to ${VALIDATOR_REAL} (${reason})"

        file=validator-${qrysm_version}-${system}-${arch}
        #res=$(curl -w '%{http_code}\n' -f -L "https://prysmaticlabs.com/releases/${file}" -o "$VALIDATOR_REAL" | ( grep 404 || true ) )
        res=$(curl -w '%{http_code}\n' -f -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}" -o "$VALIDATOR_REAL" | ( grep 404 || true ) )
        if [[ $res == 404 ]];then
            echo "No qrysm validator found for ${qrysm_version}, (${file}) exit"
            exit 1
        fi
        #curl --silent -L "https://prysmaticlabs.com/releases/${file}.sha256" -o "${wrapper_dir}/${file}.sha256"
        #curl --silent -L "https://prysmaticlabs.com/releases/${file}.sig" -o "${wrapper_dir}/${file}.sig"
        curl --silent -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}.sha256" -o "${wrapper_dir}/${file}.sha256"
        curl --silent -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}.sig" -o "${wrapper_dir}/${file}.sig"
        chmod +x "$VALIDATOR_REAL"
    else
        color "37" "Validator is up to date."
    fi
fi

if [[ $1 == client-stats ]]; then
    if [[ ! -x $CLIENT_STATS_REAL ]]; then
        color "34" "Downloading client-stats@${qrysm_version} to ${CLIENT_STATS_REAL} (${reason})"

        file=client-stats-${qrysm_version}-${system}-${arch}
        #res=$(curl -w '%{http_code}\n' -f -L "https://prysmaticlabs.com/releases/${file}" -o "$CLIENT_STATS_REAL" | ( grep 404 || true ) )
        res=$(curl -w '%{http_code}\n' -f -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}" -o "$CLIENT_STATS_REAL" | ( grep 404 || true ) )
        if [[ $res == 404 ]];then
            echo "No qrysm client stats found for ${qrysm_version},(${file}) exit"
            exit 1
        fi
        #curl --silent -L "https://prysmaticlabs.com/releases/${file}.sha256" -o "${wrapper_dir}/${file}.sha256"
        #curl --silent -L "https://prysmaticlabs.com/releases/${file}.sig" -o "${wrapper_dir}/${file}.sig"
        curl --silent -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}.sha256" -o "${wrapper_dir}/${file}.sha256"
        curl --silent -L "https://github.com/theQRL/qrysm/releases/download/{qrysm_version}/${file}.sig" -o "${wrapper_dir}/${file}.sig"
        chmod +x "$CLIENT_STATS_REAL"
    else
        color "37" "Client-stats is up to date."
    fi
fi

if [[ $1 == slasher ]]; then
    color "41" "The slasher binary is no longer available. Please use the --slasher flag with your beacon node. See: https://docs.prylabs.network/docs/prysm-usage/slasher/"
    exit 1
fi

case $1 in
beacon-chain)
    readonly process=$BEACON_CHAIN_REAL
    ;;

validator)
    readonly process=$VALIDATOR_REAL
    ;;

client-stats)
    readonly process=$CLIENT_STATS_REAL
    ;;

*)
    color "31" "Process '$1' is not found!"
    color "31" "Usage: ./qrysm.sh PROCESS FLAGS."
    color "31" "       ./qrysm.sh PROCESS --download-only."
    color "31" "PROCESS can be beacon-chain, validator, or client-stats."
    exit 1
    ;;
esac

verify "$process"

if [[ "$#" -gt 1 ]] && [[ $2 == --download-only ]]; then
    color "37" "Only download operation is requested, done."
    exit 0
fi

color "36" "Starting Qrysm $1 ${*:2}"
exec -a "$0" "${process}" "${@:2}"
