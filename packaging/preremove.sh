#!/usr/bin/env bash
set -eu

systemctl stop wireguard-manager.service || true
systemctl disable wireguard-manager.service || true
