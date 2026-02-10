#!/usr/bin/env bash
#
# Tag images mentioned in infra/releng/release-tag.txt with release-
#
# Input environment variables:
# - _IMAGE_REPO_BASE

set -eu -o pipefail

function first() {
  echo $1
}

function is_tagged_release() {
  local image=$1
  local sha=$2

  gcloud artifacts docker tags list ${image} --filter "tag~release-${sha}" --limit 1
}

function filter_out_tagged() {
  local image=$1
  shift
  
  for sha in $*; do
    local tagged=$(is_tagged_release ${image} ${sha})

    if [[ -z "${tagged}" ]]; then
      echo $sha
    fi
  done
}

function tag_release() {
  local expected=$1
  shift

  for service in $*; do
    local sha=$(filter_out_tagged ${_IMAGE_REPO_BASE}/${service} ${expected})

    if [[ -z "${sha}" ]]; then
      continue
    fi

    gcloud artifacts docker tags add \
      ${_IMAGE_REPO_BASE}/${service}:${sha} \
      ${_IMAGE_REPO_BASE}/${service}:release-${sha}
  done
}

tag_release "$@"
