#!/usr/bin/env bash
set -eu

if systemctl status wireguard-manager &> /dev/null; then
    systemctl stop wireguard-manager.service
    systemctl disable wireguard-manager.service
fi
