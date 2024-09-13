#!/bin/bash
unset AWS_PROFILE

echo "$@"

# The command is called with exec directly to replace this shell process
# with the requested one. This ensures that signals, like SIGTERM, in our
# k8s pods handle graceful termination.
#
# If we did not exec directly, then bash would handle the SIGTERM and terminate
# immediately, causing the pod to terminate immediately.
exec "$@"
