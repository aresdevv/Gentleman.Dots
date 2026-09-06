#!/usr/bin/env bash
# Regression test for issue #194.
#
# docker-test.sh pipes command output into `tee` (see the `run_image ... |
# tee "$test_output_file"` line used by the e2e runner) and relies on
# `set -e` to stop the script when a step fails. Without `set -o pipefail`,
# the exit status of that pipeline is `tee`'s, which is always 0, so a real
# failure upstream is reported as success.
#
# This test does not run Docker. It re-uses the exact `set` line(s)
# declared at the top of docker-test.sh, so it fails again if pipefail is
# ever removed, and applies them to a pipeline shaped like the one in
# docker-test.sh: a failing command piped into a sink that always exits 0.

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DOCKER_TEST_SH="$SCRIPT_DIR/docker-test.sh"

if [ ! -f "$DOCKER_TEST_SH" ]; then
  echo "FAIL: $DOCKER_TEST_SH not found" >&2
  exit 1
fi

# Extract every top-level `set ...` line declared in docker-test.sh and
# replay it in this shell.
set_lines=$(grep -E '^set ' "$DOCKER_TEST_SH")
if [ -z "$set_lines" ]; then
  echo "FAIL: no 'set' options found in $DOCKER_TEST_SH" >&2
  exit 1
fi
eval "$set_lines"

case "$set_lines" in
  *"pipefail"*) ;;
  *)
    echo "FAIL: docker-test.sh no longer declares 'set -o pipefail'" >&2
    exit 1
    ;;
esac

# Simulate the pipeline pattern used by docker-test.sh: a failing command
# piped into `tee`, whose own exit status is always 0.
if (false | tee /dev/null >/dev/null); then
  echo "FAIL: pipeline failure was masked by tee (pipefail not active)" >&2
  exit 1
fi

echo "PASS: docker-test.sh's pipefail correctly propagates pipeline failures"
