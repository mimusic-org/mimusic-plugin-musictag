// ---- 全局状态 ----

let currentTaskId = null;
let isScraping = false;
let snackbarTimer = null;

// 歌曲数据
let allSongs = [];           // 所有歌曲（从 API 获取，含刮削状态）
let filteredSongs = [];      // 筛选后的歌曲
let selectedSongIds = new Set(); // 已勾选的歌曲 ID
let currentFilter = 'all';   // 当前过滤状态
let searchKeyword = '';       // 搜索关键词
let searchDebounceTimer = null;

// ---- 工具函数 ----

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text || '';
    return div.innerHTML;
}

function showSnackbar(message, type = 'info', duration = 3000) {
    const el = document.getElementById('snackbar');
    if (snackbarTimer) clearTimeout(snackbarTimer);
    el.textContent = message;
    el.className = `snackbar ${type} show`;
    snackbarTimer = setTimeout(() => { el.className = 'snackbar'; }, duration);
}

function getAuthToken() {
    try {
        const authData = localStorage.getItem('mimusic-auth');
        if (authData) {
            const auth = JSON.parse(authData);
            return auth.accessToken || '';
        }
    } catch (e) {
        console.error('获取 Token 失败:', e);
    }
    return '';
}

function getAuthHeaders() {
    const headers = { 'Content-Type': 'application/json' };
    const token = getAuthToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    return headers;
}

// ---- 歌单渲染 ----

function renderPlaylists(playlists) {
    const container = document.getElementById('playlistContainer');
    if (!playlists || playlists.length === 0) {
        container.innerHTML = `<div class="empty-state">
            <span class="material-symbols-outlined">queue_music</span>
            <p>暂无歌单</p>
        </div>`;
        return;
    }
    container.innerHTML = playlists.map(pl => `
        <label class="list-item playlist-item">
            <input type="checkbox" class="playlist-checkbox" value="${pl.id}"
                style="accent-color:var(--md-primary)" onchange="onPlaylistSelectionChange()">
            <div class="list-item-info">
                <div class="list-item-title">${escapeHtml(pl.name)}</div>
                <div class="list-item-subtitle">${pl.song_count || 0} 首歌曲${pl.type === 'radio' ? ' · 电台' : ''}</div>
            </div>
        </label>
    `).join('');
}

// ---- 歌单选择 ----

function plSelectAll() {
    document.querySelectorAll('.playlist-checkbox').forEach(cb => { cb.checked = true; });
    onPlaylistSelectionChange();
}

function plDeselectAll() {
    document.querySelectorAll('.playlist-checkbox').forEach(cb => { cb.checked = false; });
    onPlaylistSelectionChange();
}

function updatePlSelectedCount() {
    const checked = document.querySelectorAll('.playlist-checkbox:checked');
    const count = checked.length;
    const chip = document.getElementById('plSelectedCount');
    chip.textContent = `已选 ${count} 个`;
    chip.style.display = count > 0 ? '' : 'none';
}

function getSelectedPlaylistIds() {
    return Array.from(document.querySelectorAll('.playlist-checkbox:checked')).map(cb => parseInt(cb.value));
}

// 歌单勾选变化时，加载歌曲列表
async function onPlaylistSelectionChange() {
    updatePlSelectedCount();
    const playlistIds = getSelectedPlaylistIds();

    if (playlistIds.length === 0) {
        // 隐藏歌曲列表和控制卡片
        document.getElementById('songsCard').style.display = 'none';
        document.getElementById('controlsCard').style.display = 'none';
        allSongs = [];
        filteredSongs = [];
        selectedSongIds.clear();
        return;
    }

    await loadSongs(playlistIds);
}

// ---- 加载歌单 ----

async function loadPlaylists() {
    const loadBtn = document.getElementById('loadPlaylists');
    loadBtn.disabled = true;
    loadBtn.innerHTML = '<span class="spinner"></span>加载中...';

    try {
        const response = await fetch('/api/v1/playlists?limit=100000', {
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('HTTP ' + response.status);
        const result = await response.json();

        renderPlaylists(result.playlists || []);

        if (result.playlists && result.playlists.length > 0) {
            document.getElementById('plSelectAllBtn').style.display = '';
            document.getElementById('plDeselectAllBtn').style.display = '';
            loadBtn.innerHTML = '<span class="material-symbols-outlined">refresh</span>重新加载';
            showSnackbar(`已加载 ${result.playlists.length} 个歌单`, 'success');
        } else {
            loadBtn.innerHTML = '<span class="material-symbols-outlined">refresh</span>加载歌单';
            showSnackbar('暂无歌单', 'info');
        }
    } catch (error) {
        showSnackbar('加载歌单失败：' + error.message, 'error');
        loadBtn.innerHTML = '<span class="material-symbols-outlined">refresh</span>加载歌单';
    } finally {
        loadBtn.disabled = false;
    }
}

// ---- 加载歌曲列表 ----

async function loadSongs(playlistIds) {
    const songsCard = document.getElementById('songsCard');
    const container = document.getElementById('songListContainer');
    songsCard.style.display = '';
    container.innerHTML = `<div class="empty-state"><span class="spinner" style="border-color:rgba(0,0,0,.15);border-top-color:var(--md-primary);width:24px;height:24px"></span><p>加载歌曲中...</p></div>`;

    try {
        const response = await fetch('/api/v1/plugin/musictag/api/songs', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ playlist_ids: playlistIds })
        });
        if (!response.ok) throw new Error('HTTP ' + response.status);
        const result = await response.json();

        allSongs = result.songs || [];
        // 保留之前选中的歌曲（如果还在列表中）
        const validIds = new Set(allSongs.map(s => s.id));
        selectedSongIds = new Set([...selectedSongIds].filter(id => validIds.has(id)));

        applyFilters();
        document.getElementById('controlsCard').style.display = '';
        updateClearRecordsBtnVisibility();

        const totalChip = document.getElementById('songTotalCount');
        totalChip.textContent = `${allSongs.length} 首`;
        totalChip.style.display = '';

    } catch (error) {
        container.innerHTML = `<div class="empty-state"><span class="material-symbols-outlined">error</span><p>加载失败：${escapeHtml(error.message)}</p></div>`;
        showSnackbar('加载歌曲失败：' + error.message, 'error');
    }
}

// ---- 搜索与过滤 ----

function applyFilters() {
    let songs = allSongs;

    // 状态过滤
    if (currentFilter === 'pending') {
        songs = songs.filter(s => !s.scraped && !s.scrape_failed);
    } else if (currentFilter === 'scraped') {
        songs = songs.filter(s => s.scraped);
    } else if (currentFilter === 'failed') {
        songs = songs.filter(s => s.scrape_failed);
    }

    // 搜索过滤
    if (searchKeyword) {
        const kw = searchKeyword.toLowerCase();
        songs = songs.filter(s =>
            (s.title && s.title.toLowerCase().includes(kw)) ||
            (s.artist && s.artist.toLowerCase().includes(kw)) ||
            (s.album && s.album.toLowerCase().includes(kw))
        );
    }

    filteredSongs = songs;
    renderSongList();
    updateSongSelectedCount();
    updateFilterBadges();
}

function updateFilterBadges() {
    const counts = { all: allSongs.length, pending: 0, scraped: 0, failed: 0 };
    allSongs.forEach(s => {
        if (s.scraped) counts.scraped++;
        else if (s.scrape_failed) counts.failed++;
        else counts.pending++;
    });

    document.querySelectorAll('.filter-tab').forEach(tab => {
        const filter = tab.dataset.filter;
        const count = counts[filter] || 0;
        const badge = tab.querySelector('.badge');
        if (badge) badge.textContent = count;
        else {
            const b = document.createElement('span');
            b.className = 'badge';
            b.textContent = count;
            tab.appendChild(b);
        }
    });
}

// ---- 歌曲列表渲染 ----

function renderSongList() {
    const container = document.getElementById('songListContainer');
    const countInfo = document.getElementById('songCountInfo');

    if (filteredSongs.length === 0) {
        const msg = allSongs.length === 0 ? '请先选择歌单' : '没有匹配的歌曲';
        container.innerHTML = `<div class="empty-state">
            <span class="material-symbols-outlined">music_off</span>
            <p>${msg}</p>
        </div>`;
        countInfo.textContent = '';
        return;
    }

    container.innerHTML = filteredSongs.map(song => {
        const checked = selectedSongIds.has(song.id) ? 'checked' : '';
        const statusChip = getSongStatusChip(song);
        const meta = [song.artist, song.album].filter(Boolean).join(' · ');

        return `
        <label class="song-item" data-song-id="${song.id}">
            <input type="checkbox" class="song-checkbox" value="${song.id}" ${checked}
                onchange="onSongCheckChange(${song.id}, this.checked)">
            <div class="song-item-info">
                <div class="song-item-title">${escapeHtml(song.title || '未知标题')}</div>
                ${meta ? `<div class="song-item-meta">${escapeHtml(meta)}</div>` : ''}
            </div>
            ${statusChip}
        </label>`;
    }).join('');

    countInfo.textContent = `显示 ${filteredSongs.length} / ${allSongs.length} 首歌曲`;
}

function getSongStatusChip(song) {
    if (song.scraped) {
        return `<span class="status-chip success">
            <span class="material-symbols-outlined">check_circle</span>已刮削
        </span>`;
    }
    if (song.scrape_failed) {
        return `<span class="status-chip error">
            <span class="material-symbols-outlined">error</span>失败
        </span>`;
    }
    return `<span class="status-chip pending">未刮削</span>`;
}

// ---- 歌曲勾选 ----

function onSongCheckChange(songId, checked) {
    if (checked) {
        selectedSongIds.add(songId);
    } else {
        selectedSongIds.delete(songId);
    }
    updateSongSelectedCount();
}

function songSelectAll() {
    filteredSongs.forEach(s => selectedSongIds.add(s.id));
    document.querySelectorAll('.song-checkbox').forEach(cb => { cb.checked = true; });
    updateSongSelectedCount();
}

function songDeselectAll() {
    filteredSongs.forEach(s => selectedSongIds.delete(s.id));
    document.querySelectorAll('.song-checkbox').forEach(cb => { cb.checked = false; });
    updateSongSelectedCount();
}

function updateSongSelectedCount() {
    const count = selectedSongIds.size;
    const chip = document.getElementById('songSelectedCount');
    chip.textContent = `已选 ${count} 首`;
    chip.style.display = count > 0 ? '' : 'none';

    const startBtn = document.getElementById('startBtn');
    if (startBtn && !isScraping) {
        startBtn.disabled = count === 0;
    }
}

// ---- 清除刮削记录按钮可见性 ----

function updateClearRecordsBtnVisibility() {
    const btn = document.getElementById('clearRecordsBtn');
    const hasRecords = allSongs.some(s => s.scraped || s.scrape_failed);
    btn.style.display = hasRecords ? '' : 'none';
}

// ---- 失败歌曲渲染 ----

function renderFailedSongs(failedSongs) {
    const container = document.getElementById('failedList');
    if (!failedSongs || failedSongs.length === 0) {
        container.innerHTML = '';
        return;
    }
    container.innerHTML = failedSongs.map(song => `
        <div class="failed-item">
            <span class="material-symbols-outlined" style="color:var(--md-error)">error</span>
            <div>
                <div style="font-weight:500;font-size:14px">${escapeHtml(song.title)}</div>
                <div style="font-size:13px;color:var(--md-on-surface-variant)">${escapeHtml(song.artist || '')}</div>
            </div>
        </div>
    `).join('');
}

// ---- 进度更新 ----

function updateProgress(data) {
    const pct = data.total > 0 ? Math.round(data.current / data.total * 100) : 0;
    document.getElementById('progressFill').style.width = pct + '%';
    document.getElementById('progressText').textContent =
        `${data.current} / ${data.total} (${pct}%) — 成功: ${data.success}，失败: ${data.failed}`;
    const detailEl = document.getElementById('progressDetail');
    if (data.current_song) {
        detailEl.textContent = '正在处理: ' + data.current_song;
    } else {
        detailEl.textContent = '';
    }
}

// ---- 显示结果 ----

function showResults(data) {
    document.getElementById('statTotal').textContent = data.total || 0;
    document.getElementById('statSuccess').textContent = data.success || 0;
    document.getElementById('statFailed').textContent = data.failed || 0;
    document.getElementById('resultsCard').style.display = '';

    const failedSection = document.getElementById('failedSection');
    if (data.failed > 0 && data.failed_songs && data.failed_songs.length > 0) {
        failedSection.style.display = '';
        renderFailedSongs(data.failed_songs);
        const retryBtn = document.getElementById('retryBtn');
        retryBtn.onclick = () => retryFailedSongs(data.task_id);
    } else {
        failedSection.style.display = 'none';
    }
}

// ---- 轮询进度 ----

async function pollProgress(taskId) {
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');
    const maxAttempts = 600;
    let attempts = 0;

    while (attempts < maxAttempts) {
        try {
            const response = await fetch(`/api/v1/plugin/musictag/api/scrape/status?task_id=${taskId}`, {
                headers: getAuthHeaders()
            });
            const data = await response.json();

            updateProgress(data);

            if (data.status === 'completed' || data.status === 'failed' ||
                data.status === 'stopped' || data.status === 'completed_with_errors') {
                showResults(data);
                resetButtons(startBtn, stopBtn);
                currentTaskId = null;
                isScraping = false;

                if (data.status === 'stopped') {
                    showSnackbar('已停止刮削', 'warning');
                } else {
                    const msg = data.failed > 0
                        ? `刮削完成：成功 ${data.success}，失败 ${data.failed}`
                        : `刮削完成：成功 ${data.success} 首`;
                    showSnackbar(msg, data.failed > 0 ? 'warning' : 'success');
                }

                // 刮削完成后刷新歌曲列表状态
                await refreshSongsAfterScrape();
                break;
            }

            await new Promise(r => setTimeout(r, 500));
            attempts++;
        } catch (error) {
            console.error('轮询失败:', error);
            break;
        }
    }
}

// 刮削完成后刷新歌曲列表
async function refreshSongsAfterScrape() {
    const playlistIds = getSelectedPlaylistIds();
    if (playlistIds.length > 0) {
        await loadSongs(playlistIds);
    }
}

// ---- 开始刮削 ----

async function startScrape() {
    if (isScraping) {
        showSnackbar('已有任务正在运行，请先停止', 'warning');
        return;
    }

    if (selectedSongIds.size === 0) {
        showSnackbar('请至少选择一首歌曲', 'warning');
        return;
    }

    const songIds = Array.from(selectedSongIds);
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');

    isScraping = true;
    startBtn.innerHTML = '<span class="spinner"></span>启动中...';
    startBtn.disabled = true;
    stopBtn.style.display = '';
    document.getElementById('progressCard').style.display = '';
    document.getElementById('resultsCard').style.display = 'none';

    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/batch', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ song_ids: songIds })
        });
        const result = await response.json();

        if (result.success) {
            currentTaskId = result.task_id || result.data?.task_id;
            startBtn.innerHTML = '<span class="material-symbols-outlined">auto_fix_high</span>刮削中...';
            await pollProgress(currentTaskId);
        } else {
            showSnackbar('刮削失败：' + (result.message || '未知错误'), 'error');
            resetButtons(startBtn, stopBtn);
            isScraping = false;
        }
    } catch (error) {
        showSnackbar('请求失败：' + error.message, 'error');
        resetButtons(startBtn, stopBtn);
        isScraping = false;
    }
}

// ---- 停止刮削 ----

async function stopScrape() {
    if (!currentTaskId) return;

    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/stop', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ task_id: currentTaskId })
        });
        const result = await response.json();
        if (!result.success) {
            showSnackbar('停止失败：' + (result.message || '未知错误'), 'error');
        }
    } catch (error) {
        showSnackbar('停止请求失败：' + error.message, 'error');
    }
}

// ---- 重置按钮 ----

function resetButtons(startBtn, stopBtn) {
    startBtn.innerHTML = '<span class="material-symbols-outlined">auto_fix_high</span>开始刮削';
    startBtn.disabled = selectedSongIds.size === 0;
    stopBtn.style.display = 'none';
    isScraping = false;
}

// ---- 重新刮削失败歌曲 ----

async function retryFailedSongs(taskId) {
    const retryBtn = document.getElementById('retryBtn');
    retryBtn.disabled = true;
    retryBtn.innerHTML = '<span class="spinner" style="border-top-color:var(--md-primary)"></span>刮削中...';

    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');

    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/retry-failed', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ task_id: taskId })
        });
        const result = await response.json();

        if (result.success) {
            currentTaskId = result.task_id;
            isScraping = true;
            startBtn.innerHTML = '<span class="material-symbols-outlined">auto_fix_high</span>刮削中...';
            startBtn.disabled = true;
            stopBtn.style.display = '';
            document.getElementById('progressCard').style.display = '';
            document.getElementById('resultsCard').style.display = 'none';
            await pollProgress(currentTaskId);
        } else {
            showSnackbar('重新刮削失败：' + (result.message || '未知错误'), 'error');
            retryBtn.disabled = false;
            retryBtn.innerHTML = '<span class="material-symbols-outlined">refresh</span>重新刮削失败歌曲';
        }
    } catch (error) {
        showSnackbar('重新刮削请求失败：' + error.message, 'error');
        retryBtn.disabled = false;
        retryBtn.innerHTML = '<span class="material-symbols-outlined">refresh</span>重新刮削失败歌曲';
    }
}

// ---- 清除刮削记录 ----

async function clearRecords() {
    if (!confirm('确定要清除所有刮削记录吗？这不会影响已更新的歌曲元数据。')) {
        return;
    }

    const btn = document.getElementById('clearRecordsBtn');
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner" style="border-top-color:var(--md-primary)"></span>清除中...';

    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/records', {
            method: 'DELETE',
            headers: getAuthHeaders()
        });
        const result = await response.json();

        if (result.success) {
            showSnackbar('刮削记录已清除', 'success');
            // 刷新歌曲列表
            await refreshSongsAfterScrape();
        } else {
            showSnackbar('清除失败：' + (result.message || '未知错误'), 'error');
        }
    } catch (error) {
        showSnackbar('清除请求失败：' + error.message, 'error');
    } finally {
        btn.disabled = false;
        btn.innerHTML = '<span class="material-symbols-outlined">delete_sweep</span>清除刮削记录';
    }
}

// ---- 页面初始化 ----

window.addEventListener('DOMContentLoaded', async () => {
    // 歌单操作
    document.getElementById('loadPlaylists').addEventListener('click', loadPlaylists);
    document.getElementById('plSelectAllBtn').addEventListener('click', plSelectAll);
    document.getElementById('plDeselectAllBtn').addEventListener('click', plDeselectAll);

    // 歌曲操作
    document.getElementById('songSelectAllBtn').addEventListener('click', songSelectAll);
    document.getElementById('songDeselectAllBtn').addEventListener('click', songDeselectAll);

    // 搜索
    document.getElementById('searchInput').addEventListener('input', (e) => {
        if (searchDebounceTimer) clearTimeout(searchDebounceTimer);
        searchDebounceTimer = setTimeout(() => {
            searchKeyword = e.target.value.trim();
            applyFilters();
        }, 300);
    });

    // 过滤标签
    document.querySelectorAll('.filter-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.filter-tab').forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            currentFilter = tab.dataset.filter;
            applyFilters();
        });
    });

    // 刮削操作
    document.getElementById('startBtn').addEventListener('click', startScrape);
    document.getElementById('stopBtn').addEventListener('click', stopScrape);
    document.getElementById('clearRecordsBtn').addEventListener('click', clearRecords);

    // 恢复运行中的任务
    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/status?task_id=current', {
            headers: getAuthHeaders()
        });
        const data = await response.json();

        if (data.status === 'running') {
            currentTaskId = 'current';
            isScraping = true;
            document.getElementById('controlsCard').style.display = '';
            document.getElementById('progressCard').style.display = '';
            const startBtn = document.getElementById('startBtn');
            const stopBtn = document.getElementById('stopBtn');
            startBtn.innerHTML = '<span class="material-symbols-outlined">auto_fix_high</span>刮削中...';
            startBtn.disabled = true;
            stopBtn.style.display = '';
            await pollProgress(currentTaskId);
        } else if (data.status === 'completed_with_errors' ||
            (data.status === 'completed' && (data.success > 0 || data.failed > 0))) {
            document.getElementById('controlsCard').style.display = '';
            showResults(data);
        }
    } catch (error) {
        console.error('获取任务状态失败:', error);
    }
});
