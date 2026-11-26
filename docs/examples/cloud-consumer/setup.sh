#!/bin/sh
set -eu
revision=47e82b256ad1e73456b50b55734f514eea2a130f
bin_dir="$PWD/.tools/$revision"
if [ ! -x "$bin_dir/agnostic-ai" ]; then
    mkdir -p "$bin_dir"
    GOBIN="$bin_dir" go install "github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@$revision"
fi
PATH="$bin_dir:$PATH" agnostic-ai project --check
