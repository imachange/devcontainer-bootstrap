#!/usr/bin/env bash
set -eu

gh release create "$GITHUB_REF_NAME" --draft --verify-tag --title "$GITHUB_REF_NAME" --generate-notes
gh release upload "$GITHUB_REF_NAME" dist/*
gh release edit "$GITHUB_REF_NAME" --draft=false