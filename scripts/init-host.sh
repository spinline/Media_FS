#!/usr/bin/env bash
# ==============================================================================
# Fygos NAS / Raspberry Pi 5 Host Hazırlık Scripti
# /mnt/jellyfin-remote dizinini shared bind-mount olarak yapılandırır.
# ==============================================================================
set -euo pipefail

MOUNT_DIR="${1:-/vol1/1000/Docker/mediafs}"

echo ">>> Fygos NAS FUSE Mount Noktası Yapılandırılıyor: $MOUNT_DIR"

# 1. Dizini oluştur
sudo mkdir -p "$MOUNT_DIR"

# 2. Host FUSE modülünü yükle
if ! lsmod | grep -q "^fuse"; then
  echo ">>> fuse çekirdek modülü yükleniyor..."
  sudo modprobe fuse || true
fi

# 3. Mount propagation için bind mount ve rshared yap
if ! mountpoint -q "$MOUNT_DIR"; then
  echo ">>> $MOUNT_DIR dizini shared bind mount yapılıyor..."
  sudo mount --bind "$MOUNT_DIR" "$MOUNT_DIR"
fi

sudo mount --make-rshared "$MOUNT_DIR"

# 4. Host fuse.conf ayarını doğrula
if [ -f /etc/fuse.conf ]; then
  if ! grep -q "^user_allow_other" /etc/fuse.conf; then
    echo "user_allow_other" | sudo tee -a /etc/fuse.conf >/dev/null
  fi
fi

echo ">>> Hazırlık tamamlandı! Şimdi 'docker compose up -d' çalıştırabilirsiniz."
