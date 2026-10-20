#!/bin/bash
. "$(dirname "$0")"/common.sh

# Script to copy ssz.go files from bazel build folder to appropriate location.
# Bazel builds to bazel-bin/... folder, script copies them back to original folder where target is.

bazel query 'kind(ssz_gen_marshal, //proto/...)' | xargs bazel build $@

# Get locations of the generated ssz files. The ssz_gen_marshal rule in
# tools/ssz.bzl writes "%{name}.go", and every target is named
# "ssz_generated_files", so the bazel output is "ssz_generated_files.go".
file_list=()
while IFS= read -d $'\0' -r file; do
    file_list=("${file_list[@]}" "$file")
done < <($findutil -L "$(bazel info bazel-bin)"/proto -type f -name "ssz_generated_files.go" -print0)

arraylength=${#file_list[@]}
searchstring="/bin/"

if [ "$arraylength" -eq 0 ]; then
    color "31" "No ssz_generated_files.go outputs found under bazel-bin/proto; nothing copied."
    exit 1
fi

# Copy generated ssz files from bazel-bin to the package folder where the
# target is located, under the checked-in name "generated.ssz.go".
for ((i = 0; i < arraylength; i++)); do
    destination="$(dirname "${file_list[i]#*$searchstring}")/generated.ssz.go"
    color "34" "$destination"

    # The checked-in files keep the "// Hash: ..." header line, so copy verbatim.
    # -f replaces a read-only destination instead of failing on it.
    cp -f -L "${file_list[i]}" "$destination"
    chmod 644 "$destination"
done
