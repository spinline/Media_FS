#!/usr/bin/env bash
set -e

MOUNT_DIR="${MOUNT_POINT:-/mnt/jellyfin-remote}"

echo "=================================================="
echo "   Media_FS - Remote Jellyfin FUSE Mount (ARM64)   "
echo "=================================================="

# 1. Check FUSE device
if [ ! -c /dev/fuse ]; then
  echo "[-] HATA: /dev/fuse aygıtı konteyner içinde bulunamadı."
  echo "    Lütfen konteyneri '--device /dev/fuse --cap-add SYS_ADMIN' ile başlatın."
  exit 1
fi

# 2. Ensure /etc/fuse.conf has user_allow_other enabled
if [ -f /etc/fuse.conf ]; then
  if ! grep -q "^user_allow_other" /etc/fuse.conf; then
    echo "user_allow_other" >> /etc/fuse.conf
  fi
else
  echo "user_allow_other" > /etc/fuse.conf
fi

# 3. Create mountpoint directory if not exists
mkdir -p "$MOUNT_DIR"

# 4. Log Configuration (masking secret)
echo "[+] Hedef Mount Dizini: $MOUNT_DIR"
if [ -n "$JELLYFIN_URL" ]; then
  echo "[+] Jellyfin Sunucu: $JELLYFIN_URL"
  echo "[+] Jellyfin Kullanıcı: ${JELLYFIN_USERNAME:-[API Key Modu]}"
  if [ -n "$JELLYFIN_PASSWORD" ]; then
    echo "[+] Parola: ******** (yapılandırıldı)"
  fi
  if [ -n "$JELLYFIN_API_KEY" ]; then
    echo "[+] API Anahtarı: ******** (yapılandırıldı)"
  fi
else
  echo "[i] JELLYFIN_URL ortam değişkeni verilmedi; mevcut config.yaml kullanılacak."
fi

# 6. Graceful shutdown handler
cleanup() {
  echo ""
  echo "[*] Kapatma sinyali alındı. FUSE mount temizleniyor..."
  if mountpoint -q "$MOUNT_DIR" 2>/dev/null; then
    fusermount -u "$MOUNT_DIR" 2>/dev/null || true
  fi
  if [ -n "$CHILD_PID" ]; then
    kill -TERM "$CHILD_PID" 2>/dev/null || true
    wait "$CHILD_PID" 2>/dev/null || true
  fi
  echo "[+] Başarıyla sonlandırıldı."
  exit 0
}

trap cleanup SIGTERM SIGINT SIGHUP

# 7. Start Media_FS
echo "[+] Media_FS başlatılıyor..."
if [ "$1" = "mount" ] || [ $# -eq 0 ]; then
  mediafs mount &
  CHILD_PID=$!
else
  mediafs "$@" &
  CHILD_PID=$!
fi

# Wait for process and handle signals
wait "$CHILD_PID"
EXIT_CODE=$?

# Final cleanup on normal exit
if mountpoint -q "$MOUNT_DIR" 2>/dev/null; then
  fusermount -u "$MOUNT_DIR" 2>/dev/null || true
fi

exit $EXIT_CODE
