import './style.css';
import {
  GetDrives, Scan, GetLastReport, MoveToQuarantine,
  ListQuarantine, Restore, Purge, PurgeSelected,
  BrowseFolder, Stats
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

// ── Category metadata ──────────────────────────────────────────
const CAT_META = {
  'Cache & Temp': {
    title: 'Cache & Temporary Files', retention: '14 hari',
    icon: 'draft', badge: 'AMAN', badgeClass: 'bg-safety-safe-tint text-safety-safe',
    tone: 'safe'
  },
  'Log Files': {
    title: 'Log Files Lama (≥90 hari)', retention: '30 hari',
    icon: 'description', badge: 'AMAN', badgeClass: 'bg-safety-safe-tint text-safety-safe',
    tone: 'safe'
  },
  'Duplikat': {
    title: 'Duplikat Identik (SHA-256 Valid)', retention: '30 hari',
    icon: 'copy_all', badge: 'REVIEW', badgeClass: 'bg-safety-review-tint text-safety-review',
    tone: 'review'
  },
  'Installer Lama': {
    title: 'Installer Lama (≥90 hari)', retention: '90 hari',
    icon: 'inventory_2', badge: 'REVIEW', badgeClass: 'bg-tertiary-container/20 text-tertiary',
    tone: 'review'
  },
  'File Besar Tidak Terpakai': {
    title: 'File Besar Tidak Terpakai (≥100MB & ≥365 hari)', retention: '90 hari',
    icon: 'video_file', badge: 'REVIEW', badgeClass: 'bg-safety-review-tint text-safety-review-warn',
    tone: 'warn'
  },
  'Sisa Aplikasi Terhapus': {
    title: 'Data Sisa Aplikasi yang Sudah Dihapus', retention: '90 hari',
    icon: 'delete_sweep', badge: 'CEK', badgeClass: 'bg-tertiary-container/20 text-tertiary',
    tone: 'review'
  }
};

// ── State ──────────────────────────────────────────────────────
let allDrives = [];
let scanReport = null;
let selectedKeys = new Set();
let scanning = false;

// ── Init ───────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
  initNavigation();
  initScanModeButtons();
  initScanButton();
  initBrowseButton();
  initDriveSelect();
  initBottomBarButtons();
  initQuarantineButtons();
  await loadDrives();
  await tryLoadLastReport();
});

// ── Navigation ─────────────────────────────────────────────────
function initNavigation() {
  document.querySelectorAll('.nav-tab[data-page]').forEach(tab => {
    tab.addEventListener('click', () => switchPage(tab.dataset.page));
  });
}

function switchPage(pageId) {
  document.querySelectorAll('.nav-tab').forEach(t => t.classList.remove('bg-surface-container-high', 'text-on-surface', 'font-bold', 'shadow-[0_0_12px_rgba(76,215,246,0.15)]', 'rounded-lg'));
  const activeTab = document.querySelector(`.nav-tab[data-page="${pageId}"]`);
  if (activeTab) activeTab.classList.add('bg-surface-container-high', 'text-on-surface', 'font-bold', 'shadow-[0_0_12px_rgba(76,215,246,0.15)]', 'rounded-lg');
  document.querySelectorAll('.page').forEach(p => p.classList.add('hidden'));
  const page = document.getElementById(pageId);
  if (page) page.classList.remove('hidden');
  if (pageId === 'page-quarantine') loadQuarantine();
  if (pageId === 'page-audit') loadAuditPage();
}

// ── Scan mode buttons ──────────────────────────────────────────
function initScanModeButtons() {
  document.querySelectorAll('.scan-mode-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.scan-mode-btn').forEach(b => {
        b.classList.remove('bg-primary', 'text-on-primary', 'font-bold', 'shadow-sm');
        b.classList.add('text-on-surface-variant');
      });
      btn.classList.add('bg-primary', 'text-on-primary', 'font-bold', 'shadow-sm');
      btn.classList.remove('text-on-surface-variant');
    });
  });
}

function getScanMode() {
  const active = document.querySelector('.scan-mode-btn.bg-primary');
  return active ? active.dataset.mode : 'folder';
}

// ── Drives ─────────────────────────────────────────────────────
function initDriveSelect() {
  document.getElementById('driveSelect').addEventListener('change', function () {
    const path = this.value;
    if (path) {
      document.getElementById('customPathInput').value = path;
      setScanMode('full');
    }
  });
}

async function loadDrives() {
  try {
    allDrives = (await GetDrives()) || [];
    const sel = document.getElementById('driveSelect');
    sel.innerHTML = '';
    allDrives.forEach(d => {
      const opt = document.createElement('option');
      opt.value = d.path;
      opt.textContent = d.path + ' [' + d.type + ']';
      sel.appendChild(opt);
    });
    if (allDrives.length > 0 && !document.getElementById('customPathInput').value) {
      document.getElementById('customPathInput').value = allDrives[0].path;
    }
    updateDriveUsage(allDrives);
  } catch (e) {
    console.error('GetDrives failed', e);
  }
}

// Renders up to 2 drives with the most used space (mirrors real system drives).
function updateDriveUsage(drives) {
  const box = document.getElementById('driveUsageBox');
  const usable = (drives || []).filter(d => d.total_bytes > 0);
  if (!box || usable.length === 0) return;
  const top = usable
    .map(d => ({ d, used: d.total_bytes - d.free_bytes }))
    .sort((a, b) => b.used - a.used)
    .slice(0, 2)
    .map(x => x.d);

  box.innerHTML = '';
  top.forEach(drive => {
    const usedPct = Math.round(((drive.total_bytes - drive.free_bytes) / drive.total_bytes) * 100);
    const row = document.createElement('div');
    row.className = 'flex flex-col items-end gap-0.5 px-space-md py-space-xs rounded-lg bg-surface-container-lowest border border-border-subtle';
    row.innerHTML = `
      <div class="flex items-center gap-space-xs">
        <span class="font-label-sm text-label-sm text-text-muted">Drive ${drive.path}</span>
        <span class="font-code-sm text-code-sm text-info-sky font-semibold">${usedPct}%</span>
      </div>
      <div class="w-20 h-1 bg-surface-container-highest rounded-full overflow-hidden">
        <div class="h-full bg-info-sky rounded-full" style="width:${usedPct}%"></div>
      </div>
    `;
    box.appendChild(row);
  });
}

// ── Browse folder ──────────────────────────────────────────────
function initBrowseButton() {
  document.getElementById('browseBtn').addEventListener('click', async () => {
    try {
      const dir = await BrowseFolder();
      if (dir) {
        document.getElementById('customPathInput').value = dir;
        setScanMode('folder');
      }
    } catch (e) { console.error('BrowseFolder failed', e); }
  });
}

function setScanMode(mode) {
  document.querySelectorAll('.scan-mode-btn').forEach(b => {
    const isActive = b.dataset.mode === mode;
    b.classList.toggle('bg-primary', isActive);
    b.classList.toggle('text-on-primary', isActive);
    b.classList.toggle('font-bold', isActive);
    b.classList.toggle('shadow-sm', isActive);
    b.classList.toggle('text-on-surface-variant', !isActive);
  });
}

// ── Scan ───────────────────────────────────────────────────────
function initScanButton() {
  document.getElementById('scanActionBtn').addEventListener('click', runScan);
}

async function runScan() {
  const root = document.getElementById('customPathInput').value.trim();
  if (!root) { showToast('Peringatan', 'Pilih atau ketik jalur folder terlebih dahulu', 'warning'); return; }
  if (scanning) return;
  scanning = true;

  const btn = document.getElementById('scanActionBtn');
  const label = document.getElementById('scanActionLabel');
  const progressWrap = document.getElementById('scanProgressWrap');
  const progress = document.getElementById('scanProgressBar');
  const statusText = document.getElementById('scanStatusText');
  const phaseLabel = document.getElementById('scanPhaseLabel');
  const spinner = document.getElementById('scanSpinner');
  const idleDot = document.getElementById('idleDot');
  const liveWrap = document.getElementById('scanLiveWrap');
  const liveFiles = document.getElementById('scanLiveFiles');
  const livePath = document.getElementById('scanLivePath');

  const ov = {
    el: document.getElementById('scanOverlay'),
    title: document.getElementById('scanOverlayTitle'),
    sub: document.getElementById('scanOverlaySub'),
    phase: document.getElementById('scanOverlayPhase'),
    path: document.getElementById('scanOverlayPath'),
    bar: document.getElementById('scanOverlayBar'),
    file: document.getElementById('scanOverlayFile')
  };

  const modeText = getScanMode() === 'full' ? 'Full Scan' : 'Folder ini saja';
  ov.title.textContent = 'Memindai ' + modeText + '...';
  ov.sub.textContent = root;
  ov.phase.textContent = 'Memulai pemindaian...';
  ov.path.textContent = '-';
  ov.bar.style.width = '0%';
  ov.file.textContent = 'menyiapkan...';
  ov.el.classList.remove('hidden');
  ov.el.classList.add('flex');

  label.innerHTML = '<span class="material-symbols-outlined text-[18px] spin">refresh</span> Memindai...';
  btn.classList.add('pointer-events-none');
  progressWrap.classList.remove('hidden');
  progress.style.width = '10%';
  statusText.textContent = '';
  phaseLabel.textContent = 'Memulai pemindaian...';
  spinner.classList.remove('hidden');
  idleDot.classList.add('hidden');
  liveWrap.classList.remove('hidden');
  liveFiles.textContent = '0';
  livePath.textContent = 'menyiapkan...';

  selectedKeys.clear();

  const off = EventsOn('scan:progress', (p) => {
    if (p.done) {
      progress.style.width = '100%';
      phaseLabel.textContent = 'Pemindaian selesai';
      statusText.textContent = '';
      ov.phase.textContent = 'Selesai';
      ov.bar.style.width = '100%';
      ov.file.textContent = 'Menyelesaikan...';
    } else if (p.phase === 'dedup') {
      progress.style.width = '70%';
      phaseLabel.textContent = 'Mendeteksi duplikat...';
      livePath.textContent = 'menghitung hash file...';
      ov.phase.textContent = 'Mendeteksi duplikat...';
      ov.bar.style.width = '70%';
      ov.file.textContent = 'menghitung hash file...';
    } else if (p.phase === 'scan') {
      const pct = Math.min(60, 5 + (p.files / 200));
      progress.style.width = pct + '%';
      liveFiles.textContent = (p.files || 0).toLocaleString('id-ID');
      const cur = p.current_path || '';
      if (cur) {
        const pathParts = cur.replace(/\\/g, '/').split('/');
        livePath.textContent = pathParts.length > 3 ? '.../' + pathParts.slice(-3).join('/') : cur.replace(/\\/g, '/');
      }
      phaseLabel.textContent = 'Memindai folder...';
      ov.bar.style.width = pct + '%';
      ov.phase.textContent = 'Memindai folder...';
      ov.file.textContent = (p.files || 0).toLocaleString('id-ID') + ' file • ' + livePath.textContent;
    }
  });

  try {
    const backendMode = (getScanMode() === 'full') ? 'full' : 'single';
    const result = await Scan(root, backendMode);
    if (!result.success) {
      showToast('Gagal', result.message, 'error');
      phaseLabel.textContent = 'Pemindaian gagal';
      statusText.textContent = result.message;
      return;
    }
    scanReport = result.report;
    renderResults(scanReport);
    const cands = result.report.candidates || [];
    showToast('Pemindaian Selesai', `${result.report.files_scanned.toLocaleString()} file diperiksa dalam ${(result.report.duration_ms/1000).toFixed(1)}s — ${cands.reduce((s,c)=>s+c.count,0)} kandidat`, 'task_alt');
  } catch (e) {
    showToast('Error', String(e), 'error');
    phaseLabel.textContent = 'Terjadi error';
    statusText.textContent = String(e);
  } finally {
    off();
    scanning = false;
    label.textContent = 'Pindai Ulang';
    btn.classList.remove('pointer-events-none');
    spinner.classList.add('hidden');
    idleDot.classList.remove('hidden');
    ov.el.classList.add('hidden');
    ov.el.classList.remove('flex');
    setTimeout(() => {
      progressWrap.classList.add('hidden');
      progress.style.width = '0';
      liveWrap.classList.add('hidden');
      phaseLabel.textContent = 'Menunggu pemindaian...';
    }, 1200);
  }
}

// ── Render results ─────────────────────────────────────────────
function renderResults(report) {
  const catList = document.getElementById('catList');
  let empty = document.getElementById('emptyState');
  if (!empty) {
    empty = document.createElement('div');
    empty.id = 'emptyState';
    empty.className = 'bg-surface-container-low rounded-lg p-12 flex flex-col items-center justify-center text-center shadow-sm';
    empty.innerHTML = `
      <span class="material-symbols-outlined text-text-muted text-[56px] mb-4">search</span>
      <span class="font-headline-lg text-headline-lg text-text-primary mb-2">Belum ada hasil pemindaian</span>
      <span class="font-body-sm text-body-sm text-text-secondary">Pilih drive atau folder, lalu klik <strong class="text-primary">Pindai Ulang</strong> untuk memulai.</span>
    `;
  }
  if (!report.candidates || report.candidates.length === 0) {
    empty.querySelector('span:nth-child(2)').textContent = 'Tidak ditemukan kandidat untuk dibersihkan';
    empty.querySelector('span:nth-child(3)').textContent = `${report.files_scanned.toLocaleString()} file dipindai — semuanya aman.`;
    empty.classList.remove('hidden');
    catList.innerHTML = '';
    catList.appendChild(empty);
    updateSummary();
    updateDonut({});
    document.getElementById('scanLegend').style.display = '';
    document.getElementById('scanStatusText').textContent = `${report.files_scanned.toLocaleString()} file • ${report.total_human} • 0 kandidat`;
    return;
  }
  empty.classList.add('hidden');
  document.getElementById('scanLegend').style.display = '';
  document.getElementById('scanStatusText').textContent = `${report.files_scanned.toLocaleString()} file • ${report.total_human} • ${report.candidates.length} Kategori terdeteksi`;

  catList.innerHTML = '';
  report.candidates.forEach((cat, i) => {
    catList.appendChild(buildCategoryCard(cat, i));
  });
  updateSummary();
  updateDonut(report);
}

function buildCategoryCard(cat, idx) {
  const meta = CAT_META[cat.category] || { title: cat.category, retention: '30 hari', icon: 'folder', badge: '?', badgeClass: 'bg-surface-container-high text-text-muted', tone: 'review' };
  const id = 'cat-' + idx;

  const card = document.createElement('div');
  card.className = 'bg-surface-container-low rounded-lg overflow-hidden shadow-sm transition-all';
  card.id = id;

  // Header
  const hdr = document.createElement('div');
  hdr.className = 'flex items-center justify-between p-space-md bg-surface-container-high cursor-pointer select-none';
  hdr.innerHTML = `
    <div class="flex items-center gap-space-md">
      <input type="checkbox" checked class="cat-checkbox w-4 h-4 rounded bg-surface-container-lowest text-primary focus:ring-0 cursor-pointer" data-idx="${idx}" data-cat="${cat.category}">
      <div class="flex flex-col">
        <div class="flex items-center gap-space-xs">
          <span class="font-headline-md text-headline-md text-text-primary">${meta.title}</span>
          <span class="font-code-sm text-code-sm text-text-muted px-1.5 py-0.5 rounded bg-surface-container-lowest">Retensi ${meta.retention}</span>
        </div>
        <span class="font-code-sm text-code-sm text-text-secondary">${cat.count} file terdeteksi &mdash; ${cat.total_human} pemborosan</span>
      </div>
    </div>
    <div class="flex items-center gap-space-md">
      <span class="px-space-sm py-1 rounded-lg ${meta.badgeClass} font-code-sm text-code-sm font-semibold flex items-center gap-1">
        <span class="material-symbols-outlined text-[14px]">${meta.tone === 'safe' ? 'verified' : meta.tone === 'warn' ? 'warning' : 'shield'}</span>
        ${(cat.confidence * 100).toFixed(0)}% ${meta.badge}
      </span>
      <span class="material-symbols-outlined text-text-muted transition-transform duration-200 cat-chevron">expand_less</span>
    </div>
  `;
  card.appendChild(hdr);

  // Body (open by default for first 2, closed for rest)
  const body = document.createElement('div');
  body.className = 'cat-body' + (idx < 2 ? ' open' : '');

  // Header row
  body.innerHTML = `<div class="bg-surface-container-lowest px-space-md py-1.5 flex items-center justify-between font-code-sm text-code-sm text-text-muted border-b border-border-subtle">
    <span>Path &amp; Nama Berkas</span>
    <div class="flex items-center gap-space-lg"><span>Alasan / Flag</span><span class="w-20 text-right">Ukuran</span></div>
  </div>`;

  const showCount = Math.min((cat.files || []).length, 8);
  for (let i = 0; i < showCount; i++) {
    body.appendChild(buildFileRow(cat.files[i], i % 2 === 0));
  }
  if ((cat.files || []).length > showCount) {
    const more = document.createElement('div');
    more.className = 'px-space-md py-1 bg-surface-container-lowest/50 flex justify-between items-center text-text-muted font-code-sm text-code-sm';
    more.innerHTML = `<span>... dan ${(cat.files || []).length - showCount} berkas lainnya</span><span class="text-primary hover:underline cursor-pointer">Lihat Rincian Lengkap</span>`;
    more.querySelector('.text-primary').addEventListener('click', (e) => {
      e.stopPropagation();
      const currentShow = body.querySelectorAll('.file-row').length;
      for (let j = currentShow; j < (cat.files || []).length; j++) {
        body.appendChild(buildFileRow(cat.files[j], j % 2 === 0));
      }
      more.remove();
    });
    body.appendChild(more);
  }
  card.appendChild(body);

  // Toggle accordion
  hdr.addEventListener('click', () => {
    const isOpen = body.classList.toggle('open');
    const chevron = hdr.querySelector('.cat-chevron');
    chevron.textContent = isOpen ? 'expand_less' : 'expand_more';
  });

  // Category checkbox
  const catCb = hdr.querySelector('.cat-checkbox');
  catCb.addEventListener('change', function () {
    const checked = this.checked;
    (cat.files || []).forEach(f => {
      if (checked) selectedKeys.add(f.key); else selectedKeys.delete(f.key);
    });
    body.querySelectorAll('.file-check').forEach(fc => { fc.checked = checked; });
    updateSummary();
  });

  return card;
}

function buildFileRow(file, even) {
  const div = document.createElement('div');
  div.className = `file-row flex items-center justify-between px-space-md py-space-xs transition-colors ${even ? 'bg-surface-container/60 hover:bg-surface-container' : 'bg-surface-container-low/40 hover:bg-surface-container'}`;
  const typeIcon = getTypeIcon(file.type);
  const labelClass = file.confidence >= 0.9 ? 'text-safety-safe' : 'text-text-muted';
  const checkedClass = file.confidence >= 0.9 ? ' checked' : '';
  if (file.confidence >= 0.9) selectedKeys.add(file.key);
  div.innerHTML = `
    <div class="flex items-center gap-space-sm truncate max-w-md">
      <span class="material-symbols-outlined text-primary text-[16px] shrink-0">${typeIcon}</span>
      <label class="flex items-center gap-1 cursor-pointer">
        <input type="checkbox" class="file-check${checkedClass} w-3.5 h-3.5 rounded bg-surface-container-lowest text-primary focus:ring-0 cursor-pointer" data-key="${file.key}" ${file.confidence >= 0.9 ? 'checked' : ''}>
        <span class="font-code-sm text-code-sm text-on-surface truncate" title="${file.path}">${file.path}</span>
      </label>
    </div>
    <div class="flex items-center gap-space-lg shrink-0">
      <span class="font-code-sm text-code-sm ${labelClass}">${file.reason}</span>
      <span class="font-code-sm text-code-sm text-text-primary font-semibold w-20 text-right">${file.size_human}</span>
    </div>
  `;
  const cb = div.querySelector('.file-check');
  cb.addEventListener('change', function () {
    if (this.checked) selectedKeys.add(file.key); else selectedKeys.delete(file.key);
    updateSummary();
  });
  return div;
}

function getTypeIcon(type) {
  if (!type) return 'draft';
  const t = type.toLowerCase();
  if (['.mp4', '.mkv', '.avi', '.mov'].includes(t)) return 'video_file';
  if (['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'].includes(t)) return 'image';
  if (['.log', '.logs'].includes(t)) return 'description';
  if (['.msi', '.msix', '.pkg', '.deb'].includes(t)) return 'inventory_2';
  if (t.includes('cache') || t.includes('tmp')) return 'web';
  return 'draft';
}

// ── Summary bar & donut ────────────────────────────────────────
function updateSummary() {
  const bar = document.getElementById('summaryBar');
  const countEl = document.getElementById('sumSelectedFiles');
  const sizeEl = document.getElementById('sumSelectedSize');
  const btn = document.getElementById('executePurgeBtn');
  let count = 0, freed = 0;
  if (scanReport && scanReport.candidates) {
    scanReport.candidates.forEach(cat => (cat.files || []).forEach(f => {
      if (selectedKeys.has(f.key)) { count++; freed += f.size; }
    }));
  }
  countEl.textContent = count.toLocaleString('id-ID');
  sizeEl.textContent = formatBytes(freed);
  if (count > 0) { bar.classList.remove('hidden'); btn.disabled = false; }
  else { bar.classList.add('hidden'); btn.disabled = true; }
  // donut gauge text
  if (scanReport && scanReport.candidates && scanReport.candidates.length > 0) {
    const totalCand = scanReport.candidates.reduce((s,c)=>s+c.total_bytes,0);
    const ratio = totalCand > 0 ? Math.round((freed / totalCand) * 100) : 0;
    document.getElementById('gaugeRatioText').textContent = ratio + '%';
  }
}

function updateDonut(report) {
  const safeEl = document.getElementById('donutSafe');
  const reviewEl = document.getElementById('donutReview');
  const warnEl = document.getElementById('donutWarn');
  const distSafe = document.getElementById('distSafe');
  const distLog = document.getElementById('distLog');
  const distDup = document.getElementById('distDup');
  const distLarge = document.getElementById('distLarge');
  const distOrphan = document.getElementById('distOrphan');
  const totalLabel = document.getElementById('distTotalLabel');

  if (!report || !report.candidates || report.candidates.length === 0) {
    [safeEl,reviewEl,warnEl].forEach(e => { e.setAttribute('stroke-dasharray','0, 100'); e.setAttribute('stroke-dashoffset','0'); });
    distSafe.textContent='0 B'; distLog.textContent='0 B'; distDup.textContent='0 B'; distLarge.textContent='0 B'; totalLabel.textContent='0 B';
    if (distOrphan) distOrphan.textContent='0 B';
    return;
  }
  const catBytes = {};
  report.candidates.forEach(c => { catBytes[c.category] = c.total_bytes; });
  const safe = (catBytes['Cache & Temp']||0);
  const log = (catBytes['Log Files']||0);
  const dup = (catBytes['Duplikat']||0);
  const installer = (catBytes['Installer Lama']||0);
  const large = (catBytes['File Besar Tidak Terpakai']||0);
  const orphan = (catBytes['Sisa Aplikasi Terhapus']||0);
  const total = safe+log+dup+installer+large+orphan;

  distSafe.textContent = formatBytes(safe);
  distLog.textContent = formatBytes(log);
  distDup.textContent = formatBytes(dup);
  distLarge.textContent = formatBytes(installer+large);
  if (distOrphan) distOrphan.textContent = formatBytes(orphan);
  totalLabel.textContent = formatBytes(total);

  if (total === 0) { [safeEl,reviewEl,warnEl].forEach(e => { e.setAttribute('stroke-dasharray','0, 100'); e.setAttribute('stroke-dashoffset','0'); }); return; }
  const safePct = (safe/total)*100;
  const secondaryPct = (log/total)*100;
  const reviewPct = (dup/total)*100;
  const warnPct = (installer+large+orphan)/total*100;
  safeEl.setAttribute('stroke-dasharray', `${safePct}, 100`);
  safeEl.setAttribute('stroke-dashoffset', '0');
  reviewEl.setAttribute('stroke-dasharray', `${secondaryPct+reviewPct}, 100`);
  reviewEl.setAttribute('stroke-dashoffset', `-${safePct}`);
  warnEl.setAttribute('stroke-dasharray', `${warnPct}, 100`);
  warnEl.setAttribute('stroke-dashoffset', `-${safePct+secondaryPct+reviewPct}`);
}

function formatBytes(b) {
  if (!b || b === 0) return '0 B';
  const u = ['B','KB','MB','GB','TB'];
  let i = 0, n = Math.abs(b);
  while (n >= 1024 && i < u.length-1) { n /= 1024; i++; }
  return n.toFixed(1) + ' ' + u[i];
}

// ── Bottom bar buttons ─────────────────────────────────────────
function initBottomBarButtons() {
  document.getElementById('selectSafeBtn').addEventListener('click', () => {
    selectedKeys.clear();
    if (scanReport && scanReport.candidates) scanReport.candidates.forEach(cat => {
      if ((cat.confidence||0) >= 0.9) (cat.files || []).forEach(f => selectedKeys.add(f.key));
    });
    document.querySelectorAll('.file-check').forEach(fc => {
      fc.checked = selectedKeys.has(fc.dataset.key);
    });
    document.querySelectorAll('.cat-checkbox').forEach(cb => {
      const catName = cb.dataset.cat;
      const catData = scanReport?.candidates.find(c => c.category === catName);
      cb.checked = catData && catData.confidence >= 0.9;
    });
    updateSummary();
    showToast('Opsi Diterapkan', 'Hanya kategori berstatus AMAN (≥90% confidence) yang dicentang.', 'verified');
  });

  document.getElementById('clearSelBtn').addEventListener('click', () => {
    selectedKeys.clear();
    document.querySelectorAll('.file-check').forEach(fc => fc.checked = false);
    document.querySelectorAll('.cat-checkbox').forEach(cb => cb.checked = false);
    updateSummary();
    showToast('Pilihan Dikosongkan', 'Semua kategori telah dinonaktifkan.', 'info');
  });

  document.getElementById('executePurgeBtn').addEventListener('click', async () => {
    if (selectedKeys.size === 0) return;
    const btn = document.getElementById('executePurgeBtn');
    btn.disabled = true;

    const overlay = document.getElementById('moveOverlay');
    const listEl = document.getElementById('moveFileList');
    const progBar = document.getElementById('moveProgressBar');
    const progText = document.getElementById('moveProgressText');
    const progPct = document.getElementById('moveProgressPct');
    const curFile = document.getElementById('moveCurrentFile');
    const doneCount = document.getElementById('moveDoneCount');
    const titleEl = document.getElementById('moveOverlayTitle');
    const subEl = document.getElementById('moveOverlaySub');

    // collect selected files (key + path + size)
    const queue = [];
    if (scanReport && scanReport.candidates) {
      scanReport.candidates.forEach(cat => (cat.files || []).forEach(f => {
        if (selectedKeys.has(f.key)) queue.push(f);
      }));
    }
    const total = queue.length;
    let processed = 0;
    const rows = new Map(); // key -> row el

    overlay.classList.remove('hidden');
    overlay.classList.add('flex');
    listEl.innerHTML = '';
    progBar.style.width = '0%';
    progText.textContent = `0 / ${total}`;
    progPct.textContent = '0%';
    curFile.textContent = 'menyiapkan...';
    doneCount.textContent = '0 selesai';
    titleEl.textContent = 'Memindahkan ke Karantina...';
    subEl.textContent = 'Menyalin file ke penyimpanan aman dengan validasi SHA-256.';

    queue.forEach((f, i) => {
      const row = document.createElement('div');
      row.className = 'flex items-center justify-between px-space-sm py-1.5 rounded-md bg-surface-container-low/50 transition-colors';
      row.dataset.status = 'pending';
      row.innerHTML = `
        <div class="flex items-center gap-2 min-w-0">
          <span class="material-symbols-outlined text-[16px] text-text-muted shrink-0 row-icon">schedule</span>
          <span class="font-code-sm text-code-sm text-on-surface truncate" title="${f.path}">${f.path.split(/[\\/]/).pop()}</span>
        </div>
        <span class="font-code-sm text-code-sm text-text-muted shrink-0 row-status shrink-0">menunggu</span>`;
      rows.set(f.key, row);
      listEl.appendChild(row);
    });

    const rowStatus = (key, status, text) => {
      const row = rows.get(key);
      if (!row) return;
      row.dataset.status = status;
      const icon = row.querySelector('.row-icon');
      const st = row.querySelector('.row-status');
      const map = {
        moving: ['sync', 'text-primary spin', 'memindahkan'],
        done: ['check_circle', 'text-secondary', 'selesai'],
        skip: ['skip_next', 'text-text-muted', 'di-skip'],
        fail: ['error', 'text-safety-danger', 'gagal'],
      };
      const m = map[status] || map.pending;
      icon.textContent = m[0];
      icon.className = 'material-symbols-outlined text-[16px] shrink-0 row-icon ' + m[1];
      st.textContent = text || m[2];
      st.className = 'font-code-sm text-code-sm shrink-0 row-status ' + (status === 'fail' ? 'text-safety-danger' : status === 'done' ? 'text-secondary' : status === 'skip' ? 'text-text-muted' : 'text-primary');
      listEl.appendChild(row);
      listEl.scrollTop = listEl.scrollHeight;
    };

    const off = EventsOn('quarantine:progress', (p) => {
      if (!p || !p.path) return;
      rowStatus(p.path, p.status || 'moving', p.status === 'fail' || p.status === 'skip' ? (p.message || p.status) : undefined);
      if (p.status === 'done' || p.status === 'fail' || p.status === 'skip') processed++;
      const pct = total > 0 ? Math.round((Math.min(processed + (p.status === 'moving' ? 0.5 : 0), total) / total) * 100) : 100;
      progBar.style.width = pct + '%';
      progText.textContent = `${Math.min(processed + (p.status === 'moving' ? 1 : 0), total)} / ${total}`;
      progPct.textContent = pct + '%';
      doneCount.textContent = `${processed} selesai`;
      if (p.status === 'moving') {
        curFile.textContent = (p.name || '').replace(/[\\/]/g, '/') || p.path.replace(/[\\/]/g, '/');
      }
      if (p.done) {
        progBar.style.width = '100%';
        progPct.textContent = '100%';
        progText.textContent = `${total} / ${total}`;
        doneCount.textContent = `${processed} selesai`;
        titleEl.textContent = 'Pemindahan Selesai';
        subEl.textContent = 'File berhasil dipindahkan ke karantina.';
      }
    });

    try {
      const result = await MoveToQuarantine([...selectedKeys]);
      off();
      // mark any left-over rows (not reached by events) as done if in movedKeys
      const movedSet = new Set(result.moved_keys || []);
      rows.forEach((row, key) => {
        if (row.dataset.status === 'pending') {
          rowStatus(key, movedSet.has(key) ? 'done' : 'skip', movedSet.has(key) ? undefined : 'tidak dipindahkan');
        }
      });
      processed = total;
      progBar.style.width = '100%';
      progPct.textContent = '100%';
      progText.textContent = `${total} / ${total}`;
      doneCount.textContent = `${processed} selesai`;
      titleEl.textContent = 'Pemindahan Selesai';
      subEl.textContent = result.message || 'File berhasil dipindahkan ke karantina.';

      setTimeout(() => {
        overlay.classList.add('hidden');
        overlay.classList.remove('flex');
      }, 1400);

      if (scanReport && scanReport.candidates) {
        scanReport.candidates.forEach(cat => {
          const files = cat.files || [];
          cat.files = files.filter(f => !movedSet.has(f.key));
          cat.count = cat.files.length;
          cat.total_bytes = cat.files.reduce((s, f) => s + f.size, 0);
          cat.total_human = formatBytes(cat.total_bytes);
        });
        scanReport.candidates = scanReport.candidates.filter(c => c.count > 0);
        movedSet.forEach(k => selectedKeys.delete(k));
        renderResults(scanReport);
      }
      const failed = result.failed || 0;
      const moved = result.succeeded || 0;
      if (moved > 0 && failed > 0) {
        showToast('Sebagian Berhasil', `${moved} file dipindahkan (${result.freed_human}). ${failed} gagal / di-skip. Periksa Detail di bawah.`, 'warning');
      } else if (moved > 0) {
        showToast('Karantina Berhasil', `${result.succeeded} file (${result.freed_human}) berhasil diisolasi ke karantina ber-manifest SHA-256.`, 'verified_user');
      } else {
        showToast('Gagal', result.message || 'Tidak ada file yang berhasil dipindahkan.', 'error');
      }
    } catch (e) {
      off();
      overlay.classList.add('hidden');
      overlay.classList.remove('flex');
      showToast('Error', String(e), 'error');
    }
    finally { btn.disabled = false; }
  });
}

// ── Quarantine ─────────────────────────────────────────────────
let qSelected = new Set();
let qAllIds = [];

function initQuarantineButtons() {
  document.getElementById('btnRestoreSel').addEventListener('click', async () => {
    if (qSelected.size === 0) return;
    try {
      const r = await Restore([...qSelected]);
      showToast(r.succeeded ? 'Pemulihan' : 'Error', r.message, r.succeeded ? 'restore' : 'error');
      qSelected.clear();
      loadQuarantine();
    } catch(e) { showToast('Error', String(e), 'error'); }
  });

  document.getElementById('btnPurgeSel').addEventListener('click', async () => {
    if (qSelected.size === 0) return;
    if (!confirm(`Hapus permanen ${qSelected.size} file terpilih? Tidak bisa dibatalkan!`)) return;
    try {
      const r = await PurgeSelected([...qSelected]);
      showToast(r.success ? 'Dihapus' : 'Error', r.message, r.success ? 'delete_forever' : 'error');
      qSelected.clear();
      loadQuarantine();
    } catch(e) { showToast('Error', String(e), 'error'); }
  });

  document.getElementById('btnPurgeExpired').addEventListener('click', async () => {
    if (!confirm('Hapus permanen semua file yang sudah expired?')) return;
    try {
      const r = await Purge(false);
      showToast('Selesai', r.message, 'task_alt');
      loadQuarantine();
    } catch(e) { showToast('Error', String(e), 'error'); }
  });

  document.getElementById('qCheckAll').addEventListener('change', function () {
    qSelected.clear();
    if (this.checked) qAllIds.forEach(id => qSelected.add(id));
    document.querySelectorAll('.q-row-check').forEach(cb => cb.checked = this.checked);
    updateQBtns();
  });

  document.getElementById('btnRefreshQ').addEventListener('click', loadQuarantine);
}

function updateQBtns() {
  document.getElementById('btnRestoreSel').disabled = qSelected.size === 0;
  document.getElementById('btnPurgeSel').disabled = qSelected.size === 0;
}

async function loadQuarantine() {
  qSelected.clear();
  document.getElementById('qCheckAll').checked = false;
  updateQBtns();
  try {
    const items = await ListQuarantine();
    const tbody = document.getElementById('qTbody');
    const empty = document.getElementById('qEmpty');
    if (!items || items.length === 0) {
      tbody.innerHTML = '';
      empty.classList.remove('hidden');
      document.getElementById('qTotalCount').textContent = '0';
      document.getElementById('qExpiredCount').textContent = '0';
      document.getElementById('qTotalSize').textContent = '0 B';
      qAllIds = [];
      return;
    }
    empty.classList.add('hidden');
    qAllIds = items.map(q => q.id);
    let totalBytes = 0, expired = 0;
    items.forEach(q => { totalBytes += q.size; if (q.expired) expired++; });
    document.getElementById('qTotalCount').textContent = items.length;
    document.getElementById('qExpiredCount').textContent = expired;
    document.getElementById('qTotalSize').textContent = formatBytes(totalBytes);

    tbody.innerHTML = '';
    items.forEach(q => {
      const tr = document.createElement('tr');
      tr.className = 'border-b border-border-subtle hover:bg-surface-container-high/50 transition-colors';
      tr.innerHTML = `
        <td class="px-space-md py-space-sm"><input type="checkbox" class="q-row-check w-3.5 h-3.5 rounded bg-surface-container-lowest text-primary focus:ring-0 cursor-pointer" data-id="${q.id}"></td>
        <td class="px-space-md py-space-sm max-w-[300px] truncate font-code-sm text-code-sm" title="${q.original_path}">${q.original_path.split('\\').pop()}</td>
        <td class="px-space-md py-space-sm font-code-sm text-code-sm">${q.category}</td>
        <td class="px-space-md py-space-sm font-code-sm text-code-sm">${q.size_human}</td>
        <td class="px-space-md py-space-sm font-code-sm text-code-sm ${q.expired ? 'text-safety-danger' : ''}">${q.expired ? 'Expired' : q.retention_until}</td>
        <td class="px-space-md py-space-sm font-code-sm text-code-sm text-text-muted">${q.moved_at}</td>
        <td class="px-space-md py-space-sm flex gap-1">
          <button class="px-2 py-0.5 rounded text-secondary hover:bg-secondary/20 text-xs font-semibold q-act-btn" data-action="restore" data-id="${q.id}">Pulihkan</button>
          <button class="px-2 py-0.5 rounded text-safety-danger hover:bg-safety-danger/20 text-xs font-semibold q-act-btn" data-action="purge" data-id="${q.id}">Hapus</button>
        </td>
      `;
      const cb = tr.querySelector('.q-row-check');
      cb.addEventListener('change', function () {
        if (this.checked) qSelected.add(q.id); else qSelected.delete(q.id);
        updateQBtns();
      });
      tr.querySelectorAll('.q-act-btn').forEach(b => {
        b.addEventListener('click', async (e) => {
          e.stopPropagation();
          const action = b.dataset.action;
          if (action === 'restore') {
            try { const r = await Restore([q.id]); showToast('Pulihkan', r.message, 'restore'); loadQuarantine(); } catch(e2) { showToast('Error', String(e2), 'error'); }
          } else {
            if (!confirm(`Hapus "${q.original_path.split('\\').pop()}" secara permanen?`)) return;
            try { const r = await PurgeSelected([q.id]); showToast('Dihapus', r.message, 'delete_forever'); loadQuarantine(); } catch(e2) { showToast('Error', String(e2), 'error'); }
          }
        });
      });
      tbody.appendChild(tr);
    });
  } catch (e) { console.error('ListQuarantine failed', e); }
}

// ── Audit page ─────────────────────────────────────────────────
async function loadAuditPage() {
  const reportEl = document.getElementById('auditReportContent');
  const quarantineEl = document.getElementById('auditQuarantineContent');
  try {
    const report = await GetLastReport();
    reportEl.innerHTML = `
      <div class="flex justify-between"><span class="text-text-muted">Waktu scan</span><span class="text-text-primary">${report.generated_at}</span></div>
      <div class="flex justify-between"><span class="text-text-muted">Durasi</span><span class="text-text-primary">${(report.duration_ms/1000).toFixed(1)} detik</span></div>
      <div class="flex justify-between"><span class="text-text-muted">File dipindai</span><span class="text-text-primary">${report.files_scanned.toLocaleString()}</span></div>
      <div class="flex justify-between"><span class="text-text-muted">Direktori</span><span class="text-text-primary">${report.dirs_scanned.toLocaleString()}</span></div>
      <div class="flex justify-between"><span class="text-text-muted">Total ukuran</span><span class="text-text-primary">${report.total_human}</span></div>
      <div class="flex justify-between"><span class="text-text-muted">Kandidat</span><span class="text-text-primary">${(report.candidates||[]).reduce((s,c)=>s+c.count,0)}</span></div>
    `;
  } catch(e) { reportEl.innerHTML = '<span class="text-text-muted">Belum ada data pemindaian.</span>'; }
  try {
    const st = await Stats();
    quarantineEl.innerHTML = `
      <div class="flex justify-between"><span class="text-text-muted">File dikarantina</span><span class="text-text-primary">${st.quarantine_count}</span></div>
      <div class="flex justify-between"><span class="text-text-muted">Total ukuran</span><span class="text-text-primary">${formatBytes(st.quarantine_bytes)}</span></div>
    `;
  } catch(e) { quarantineEl.innerHTML = '<span class="text-text-muted">Tidak ada data.</span>'; }
}

// ── Toast ──────────────────────────────────────────────────────
function showToast(title, message, icon) {
  const toast = document.getElementById('toastNotification');
  document.getElementById('toastTitle').textContent = title;
  document.getElementById('toastMessage').textContent = message;
  document.getElementById('toastIcon').textContent = icon || 'check_circle';
  toast.classList.remove('translate-y-24','opacity-0','pointer-events-none');
  toast.classList.add('translate-y-0','opacity-100');
  setTimeout(() => {
    toast.classList.remove('translate-y-0','opacity-100');
    toast.classList.add('translate-y-24','opacity-0','pointer-events-none');
  }, 3500);
}

// ── Try load last report on startup ────────────────────────────
async function tryLoadLastReport() {
  try {
    const report = await GetLastReport();
    if (report && report.candidates && report.candidates.length > 0) {
      scanReport = report;
      renderResults(scanReport);
    }
  } catch(e) { /* no prior report */ }
  try { document.getElementById('settingsDataDir').textContent = '...'; } catch(e) {}
}
