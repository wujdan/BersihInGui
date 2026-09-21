# Bersihin – Storage Optimizer

Bersihin adalah aplikasi pembersih penyimpanan untuk Windows yang memindai drive,
mengidentifikasi file yang berpotensi tidak penting, lalu memberi kontrol penuh
kepada pengguna untuk memilih file yang akan dikarantina sebelum dihapus permanen.

Dibangun dengan **Wails v2** (Go + Web) untuk GUI dan **Cobra** untuk mode CLI.

![GitHub License](https://img.shields.io/github/license/wujdan/BersihInGui)

## Download

- **End-user:** unduh installer siap pakai dari [Releases](https://github.com/wujdan/BersihInGui/releases).
  > Versi installer untuk pengguna non-teknis - wizard instalasi, shortcut otomatis, uninstall lengkap.
- **Developer:** clone repo & build dari source (lihat [Build (Production)](#build-production)).
- **Mode CLI (tanpa GUI):** repo khusus → [wujdan/BersihInCli](https://github.com/wujdan/BersihInCli.git)

> Catatan: versi installer end-user akan menyusul. Halaman Releases diperbarui saat rilis baru dipublikasikan.

## Fitur

- **Pemindaian multi-database** untuk kategori pemborosan ruang:
  - Cache & Temporary Files
  - Log Files Lama (≥90 hari)
  - Duplikat Identik (validasi SHA-256)
  - Installer Lama (≥90 hari)
  - File Besar Tidak Terpakai (≥100 MB & ≥365 hari)
  - Sisa Aplikasi Terhapus (via registry Windows + Program Files)
- **Karantina berbasis manifest SHA-256** - file diverifikasi sebelum dihapus,
  dan bisa dipulihkan selama masa retensi.
- **Audit trail** - seluruh proses tercatat dalam log.
- **Live progress** - tampilan progres pemindaian & pemindahan secara real-time.
- **Mode CLI** untuk skrip / cron (`--yes` non-interaktif).

## Persyaratan

- [Go 1.27+](https://go.dev/dl/)
- [Node.js 18+ dan npm](https://nodejs.org/)
- [Wails v2](https://wails.io/) - CLI untuk dev & build
- Windows (WebView2) - fitur deteksi drive/orphan spesifik Windows

## Menjalankan (Development)

```bash
wails dev
```

## Build (Production)

```bash
wails build
```

Hasil build berada di `build/bin/Bersihin.exe`.

Untuk membuat **installer Windows** (`.exe` instalasi + uninstaller otomatis):

```bash
wails build --nsis
```

Hasil installer berada di `build/bin/Bersihin-amd64-installer.exe`.

## Instalasi (Windows)

1. Unduh file installer `Bersihin-<arsitektur>-installer.exe` dari rilis
   (atau hasil `wails build --nsis`).
2. Jalankan installer dengan **klik dua kali** (butuh hak administrator).
3. Ikuti langkah pada wizard instalasi:
   - **Welcome** - klik *Next*.
   - **Choose Install Location** - pilih folder tujuan
     (default: `C:\Program Files\...`), lalu klik *Install*.
   - Tunggu sampai proses selesai, lalu klik *Finish*.
4. Setelah selesai:
   - Shortcut "Bersihin" dibuat otomatis di **Start Menu** dan **Desktop**.
   - Aplikasi siap dijalankan lewat shortcut tersebut.

Data aplikasi (pernah dipindai, laporan scan, karantina, log audit) disimpan
otomatis di `~/.storage-optimizer` pada profil pengguna.

## Uninstalasi (Windows)

**Catatan penting:** Uninstaller menghapus **seluruh file aplikasi termasuk
semua data pengguna** (laporan scan, file karantina, dan log audit) secara
otomatis dan permanen. Pastikan Anda sudah memindahkan/pulihkan apa pun yang
masih dibutuhkan dari karantina sebelum melakukan uninstalasi.

Cara uninstal:

1. **Lewat Start Menu** - cari folder *Bersihin* → klik **Uninstall Bersihin**.
   Atau **lewati Pengaturan Windows** - buka *Settings → Apps → Installed apps*,
   cari *Bersihin*, lalu klik *Uninstall*.
2. Konfirmasi pada jendela uninstaller; proses berjalan otomatis.
3. Setelah selesai, berikut ini ikut terhapus semuanya secara otomatis:
   - File program (folder instalasi)
   - Data pengguna `~/.storage-optimizer` (laporan scan, file karantina, log)
   - Data cache WebView2 aplikasi di `%AppData%`
   - Shortcut di Start Menu dan Desktop
   - Entri registri dan custom protocol yang didaftarkan installer

Tidak diperlukan langkah pembersihan manual tambahan.

## Mode CLI

> Untuk menjalankan versi CLI (tanpa GUI), langsung akses repo khusus:
> **[wujdan/BersihInCli](https://github.com/wujdan/BersihInCli.git)**.

Binary yang sama juga menyediakan antarmuka CLI:

```bash
Bersihin scan --mode=full                # scan seluruh drive
Bersihin scan --mode=full --dry-run      # simulasi tanpa hapus
Bersihin scan --mode=custom --path=D:/Project
Bersihin scan --mode=quick --yes         # non-interaktif (skrip/cron)
Bersihin review                          # tinjau hasil scan terakhir
Bersihin restore                         # pulihkan file dari karantina
Bersihin purge                           # hapus permanen file expired
Bersihin config                          # tampilkan/mengubah konfigurasi
```

## Konfigurasi

Konfigurasi default ada di `config/default.yaml`. Data aplikasi (laporan scan,
karantina, log) disimpan di `~/.storage-optimizer` kecuali diubah pada
`app.data_dir`.

## Struktur Proyek

```
app.go                    Binding Wails (Scan, MoveToQuarantine, Restore, dll.)
main.go                   Entry point Wails
config/default.yaml       Konfigurasi default
internal/
  app/                    Logika aplikasi bersama (scan, execute)
  classifier/             Klasifikasi kandidat file
  cli/                    Perintah Cobra (scan, restore, purge, ...)
  config/                 Pemuatan & normalisasi konfigurasi
  logging/                Audit trail
  models/                 Model domain (ScanReport, Candidate, ...)
  orphan/                 Deteksi sisa aplikasi terhapus
  quarantine/             Karantina ber-manifest SHA-256
  safety/                 Validator keamanan penghapusan
  scanner/                Scanner file & deteksi duplikat
  store/                  Persistensi laporan
  ui/                     Dashboard TUI (Bubble Tea)
frontend/                 GUI Wails (Vanilla JS + Tailwind CSS 4 + Vite)
```

## Keamanan

- Setiap file yang dikarantina diverifikasi SHA-256 sebelum dihapus permanen.
- File tidak langsung dihapus - dipindah ke direktori karantina terlebih dulu
  dan dapat dipulihkan selama masa retensi (default 30 hari).
- Folder sistem dan program secara default dikecualikan dari pemindaian.

## Lisensi

[MIT](LICENSE) © 2026 Danss