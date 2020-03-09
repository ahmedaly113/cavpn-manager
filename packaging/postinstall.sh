#!/usr/bin/env bash
set -eu

adduser --system cavpn-manager --no-create-home

systemctl enable "/etc/systemd/system/cavpn-manager.service"
