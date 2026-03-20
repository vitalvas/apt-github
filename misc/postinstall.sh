#!/bin/sh
set -e

if [ ! -f /etc/apt/keyrings/apt-transport-github.gpg ]; then
    /usr/lib/apt/methods/github setup
fi
