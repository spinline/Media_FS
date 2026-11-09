# Fygos NAS (Raspberry Pi 5 ARM64) Media_FS Kurulum Rehberi

Bu rehber, **Media_FS** projesini Raspberry Pi 5 (Linux ARM64 / aarch64) tabanlı **Fygos NAS** üzerinde Docker konteyneri ile uzak Jellyfin sunucusunu `/mnt/jellyfin-remote` altına yerel salt-okunur (read-only) klasör olarak bağlamak (FUSE mount) için hazırlanmıştır.

---

## 🚀 Özellikler

- **Linux ARM64 Desteği**: Raspberry Pi 5 mimarisiyle %100 uyumlu, çok aşamalı (multi-stage) Docker yapısı.
- **Güvenli Yapılandırma**: Sunucu URL'si, kullanıcı adı, parola veya API anahtarı `.env` dosyası veya ortam değişkenleri üzerinden yönetilir.
- **Mount Propagation (`rshared`)**: Konteyner içinde FUSE ile bağlanan medya arşivi, doğrudan NAS ana makinesi (host) üzerinden `/mnt/jellyfin-remote` altında normal bir klasör gibi görünür.
- **Salt-Okunur (Read-Only) Güvenlik**: VFS düzeyinde tüm yazma ve silme işlemleri engellenmiştir (`-fuse.EPERM`) ve `-o ro,allow_other` parametreleriyle bağlanır.
- **Akıllı Akış ve İleri Sarma (HTTP Range & Seek)**: VLC, MPV veya diğer medya oynatıcıların video indekslerini (MKV cues, MP4 moov atom) okuması ve istenen dakikaya anında atlaması için optimize edilmiş bellek penceresi önbelleği (WindowCache).
- **Otomatik Yeniden Başlatma**: `restart: unless-stopped` politikası ile NAS yeniden başlatıldığında otomatik devreye girer.
- **İzole Çalışma**: NAS üzerindeki mevcut diğer Docker konteynerlerine ve servislere asla müdahale etmez.

---

## 🛠️ Kurulum Adımları

### 1. Ana Makine (Fygos NAS) Hazırlığı

FUSE mount'unun konteyner dışından (NAS üzerinden) okunabilmesi için `/mnt/jellyfin-remote` dizini shared mount yapılmalıdır:

```bash
# Otomatik hazırlık scriptini çalıştırın:
sudo bash scripts/init-host.sh /mnt/jellyfin-remote
```

*(Bu işlem `/dev/fuse` modülünün yüklendiğinden ve dizinin `rshared` olarak işaretlendiğinden emin olur.)*

---

### 2. Ortam Değişkenlerini Yapılandırın

Örnek `.env.example` dosyasını `.env` olarak kopyalayın:

```bash
cp .env.example .env
nano .env
```

`.env` dosyasındaki ayarları kendi Jellyfin sunucunuza göre düzenleyin:

```env
# Uzak Jellyfin Sunucunuzun Adresi
JELLYFIN_URL=http://192.168.1.100:8096
JELLYFIN_ALIAS=Jellyfin

# Kullanıcı adı ve parola (veya alternatif olarak JELLYFIN_API_KEY)
JELLYFIN_USERNAME=kullanici_adiniz
JELLYFIN_PASSWORD=guclu_parolaniz

# NAS Mount Noktası
HOST_MOUNT_POINT=/mnt/jellyfin-remote

# FUSE Seçenekleri
MOUNT_OPTIONS=ro,allow_other

# Akış / Önbellek Ayarları
CHUNK_SIZE_MB=64
PARALLEL_CHUNKS=4
MAX_CACHE_MB=2048
CACHE_TTL_MIN=120
```

---

### 3. Konteyneri Başlatın

```bash
# Docker Compose ile konteyneri derleyin ve arka planda başlatın:
docker compose up -d --build
```

Logları takip etmek için:

```bash
docker compose logs -f
```

---

### 4. Bağlantıyı Doğrulayın

NAS üzerinde mount noktasını kontrol edin:

```bash
ls -la /mnt/jellyfin-remote
```

Jellyfin kütüphaneleriniz (`Filmler`, `Diziler` vb.) doğrudan klasör olarak listelenecektir:

```
/mnt/jellyfin-remote/
└── kullanici@Jellyfin/
    ├── Filmler/
    │   └── Inception (2010)/
    │       ├── Inception (2010).mkv
    │       └── Inception (2010).nfo
    └── Diziler/
```

Herhangi bir video dosyasını NAS üzerinden açabilir veya doğrudan oynatabilirsiniz:

```bash
head -c 1024 /mnt/jellyfin-remote/*/*/*/*.mkv > /dev/null && echo "Başarıyla okundu!"
```

---

### 5. Durumu Kontrol Etme veya Yeniden Başlatma

- **Konteyner Durumu:**
  ```bash
  docker compose ps
  ```

- **Uzak Sunucu Erişilebilirlik Testi:**
  ```bash
  docker compose exec mediafs-jellyfin mediafs status
  ```

- **Durdurma:**
  ```bash
  docker compose down
  ```
