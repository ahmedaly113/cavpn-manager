#!/usr/bin/env bash
set -eu

adduser --system wireguard-manager --no-create-home

systemctl enable "/etc/systemd/system/wireguard-manager.service"
