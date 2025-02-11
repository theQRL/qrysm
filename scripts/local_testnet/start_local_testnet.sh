#!/usr/bin/env bash

# Requires `docker`, `kurtosis`, `yq`

set -Eeuo pipefail

SCRIPT_DIR="$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
ENCLAVE_NAME=local-testnet
NETWORK_PARAMS_FILE=$SCRIPT_DIR/network_params.yaml
# TODO(now.youtrack.cloud/issue/TQ-35)
# ZOND_PKG_VERSION=main
ZOND_PKG_VERSION=630ba876d69fc58b812553b1400ccf62259090f3

BUILD_IMAGE=true
BUILDER_PROPOSALS=false
CI=false
KEEP_ENCLAVE=false

# Get options
while getopts "e:b:n:phck" flag; do
  case "${flag}" in
    e) ENCLAVE_NAME=${OPTARG};;
    b) BUILD_IMAGE=${OPTARG};;
    n) NETWORK_PARAMS_FILE=${OPTARG};;
    p) BUILDER_PROPOSALS=true;;
    c) CI=true;;
    k) KEEP_ENCLAVE=true;;
    h)
        echo "Start a local testnet with kurtosis."
        echo
        echo "usage: $0 <Options>"
        echo
        echo "Options:"
        echo "   -e: enclave name                                default: $ENCLAVE_NAME"
        echo "   -b: whether to build Qrysm docker image         default: $BUILD_IMAGE"
        echo "   -n: kurtosis network params file path           default: $NETWORK_PARAMS_FILE"
        echo "   -p: enable builder proposals"
        echo "   -c: CI mode, run without other additional services like Grafana and explorer"
        echo "   -k: keeping enclave to allow starting the testnet without destroying the existing one"
        echo "   -h: this help"
        exit
        ;;
  esac
done

if ! command -v docker &> /dev/null; then
    echo "Docker is not installed. Please install Docker and try again."
    exit 1
fi

if ! command -v kurtosis &> /dev/null; then
    echo "kurtosis command not found. Please install kurtosis and try again."
    exit
fi

if ! command -v yq &> /dev/null; then
    echo "yq not found. Please install yq and try again."
fi

if [ "$BUILDER_PROPOSALS" = true ]; then
  yq eval '.participants[0].vc_extra_params = ["--builder-proposals"]' -i $NETWORK_PARAMS_FILE
  echo "--builder-proposals VC flag added to network_params.yaml"
fi

if [ "$CI" = true ]; then
  # TODO: run assertoor tests
  yq eval '.additional_services = []' -i $NETWORK_PARAMS_FILE
  echo "Running without additional services (CI mode)."
fi

if [ "$BUILD_IMAGE" = true ]; then
    echo "Building Qrysm Docker images."
    ROOT_DIR="$SCRIPT_DIR/../.."
    ARCH="$(uname -m)"
    if [ "$ARCH" = "arm64" ]; then
        # Beacon node
        bazel build //cmd/beacon-chain:oci_image_tarball --platforms=@io_bazel_rules_go//go/toolchain:linux_arm64_cgo --config=release
        docker load -i $ROOT_DIR/bazel-bin/cmd/beacon-chain/oci_image_tarball/tarball.tar
        # Validator client
        bazel build //cmd/validator:oci_image_tarball  --platforms=@io_bazel_rules_go//go/toolchain:linux_arm64_cgo --config=release
        docker load -i $ROOT_DIR/bazel-bin/cmd/validator/oci_image_tarball/tarball.tar
    else
        # Beacon node
        bazel build //cmd/beacon-chain:oci_image_tarball --config=release
        docker load -i $ROOT_DIR/bazel-bin/cmd/beacon-chain/oci_image_tarball/tarball.tar
        # Validator client
        bazel build //cmd/validator:oci_image_tarball --config=release
        docker load -i $ROOT_DIR/bazel-bin/cmd/validator/oci_image_tarball/tarball.tar
    fi
else
    echo "Not rebuilding Qrysm Docker images."
fi

if [ "$KEEP_ENCLAVE" = false ]; then
  # Stop local testnet
  kurtosis enclave rm -f $ENCLAVE_NAME 2>/dev/null || true
fi

# TODO(now.youtrack.cloud/issue/TQ-35)
kurtosis run --enclave $ENCLAVE_NAME github.com/rgeraldes24/zond-package@$ZOND_PKG_VERSION --args-file $NETWORK_PARAMS_FILE

echo "Started!"