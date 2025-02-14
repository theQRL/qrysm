#!/bin/bash

# -----------------------------------------------------------------------------
# This script builds and uploads the docker images to the registries.
#
# This script is intended to be a workaround until the rules_oci project supports
# targets with multiple repositories like rules_docker does. See: https://github.com/bazel-contrib/rules_oci/issues/248
# -----------------------------------------------------------------------------

# Validate that the tag argument exists.
if [ "$1" = "" ]
then
  echo "Usage: $0 <tag>"
  exit
fi
TAG=$1

# Sanity check that all targets can build before running them.
bazel build --config=release \
  //cmd/beacon-chain:push_oci_image \
  //cmd/validator:push_oci_image \
  //cmd/qrysmctl:push_oci_image

# Push the images to the registry.
### Beacon chain
bazel run --config=release \
  //cmd/beacon-chain:push_oci_image -- --tag=$TAG

### Validator
bazel run --config=release \
  //cmd/validator:push_oci_image -- --tag=$TAG

### Qrysmctl
bazel run --config=release \
  //cmd/qrysmctl:push_oci_image -- --tag=$TAG