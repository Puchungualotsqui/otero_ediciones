#!/usr/bin/env sh
set -eu

tailwindcss -i styles/input.css -o static/assets/app.css --minify
exec go run .
