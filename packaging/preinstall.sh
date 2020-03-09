#!/usr/bin/env bash
set -eu

if systemctl status cavpn-manager &> /dev/null; then
    systemctl stop cavpn-manager.service
    systemctl disable cavpn-manager.service
fi
