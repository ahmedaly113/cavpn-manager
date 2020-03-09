#!/usr/bin/env bash
set -eu

systemctl stop cavpn-manager.service || true
systemctl disable cavpn-manager.service || true
