#!/bin/bash
set -e

if [[ "$1" == "gui" ]]; then
  echo "Starting GUI with Xvfb..."
  exec xvfb-run --auto-servernum --server-args="-screen 0 1024x768x24" gogg gui
else
  exec gogg "$@"
fi
