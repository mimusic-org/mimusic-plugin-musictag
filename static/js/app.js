let currentTaskId = null;
let isScraping = false;
let snackbarTimer = null;

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
                style="accent-color:var(--md-primary)" onchange="updateSelectedCount()">
            <div class="list-item-info">
                <div class="list-item-title">${escapeHtml(pl.name)}</div>
                <div class="list-item-subtitle">${pl.song_count || 0} 首歌曲${pl.type === 'radio' ? ' · 电台' : ''}</div>
            </div>
        </label>
    `).join('');
}

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

// ---- 歌单选择 ----

function selectAll() {
    document.querySelectorAll('.playlist-checkbox').forEach(cb => { cb.checked = true; });
    updateSelectedCount();
}

function deselectAll() {
    document.querySelectorAll('.playlist-checkbox').forEach(cb => { cb.checked = false; });
    updateSelectedCount();
}

function updateSelectedCount() {
    const checked = document.querySelectorAll('.playlist-checkbox:checked');
    const count = checked.length;
    const chip = document.getElementById('selectedCount');
    chip.textContent = `已选 ${count} 个`;
    chip.style.display = count > 0 ? '' : 'none';
    const startBtn = document.getElementById('startBtn');
    if (startBtn) startBtn.disabled = count === 0;
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
            document.getElementById('selectAllBtn').style.display = '';
            document.getElementById('deselectAllBtn').style.display = '';
            document.getElementById('controlsCard').style.display = '';
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
        // 绑定重试按钮
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

// ---- 开始刮削 ----

async function startScrape() {
    if (isScraping) {
        showSnackbar('已有任务正在运行，请先停止', 'warning');
        return;
    }

    const checkboxes = document.querySelectorAll('.playlist-checkbox:checked');
    if (checkboxes.length === 0) {
        showSnackbar('请至少选择一个歌单', 'warning');
        return;
    }

    const selectedPlaylistIds = Array.from(checkboxes).map(cb => parseInt(cb.value));
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
            body: JSON.stringify({ playlist_ids: selectedPlaylistIds })
        });
        const result = await response.json();

        if (result.success) {
            currentTaskId = result.task_id;
            startBtn.innerHTML = '<span class="material-symbols-outlined">auto_fix_high</span>刮削中...';
            await pollProgress(currentTaskId);
        } else {
            showSnackbar('批量刮削失败：' + (result.message || '未知错误'), 'error');
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
    startBtn.disabled = document.querySelectorAll('.playlist-checkbox:checked').length === 0;
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

// ---- 页面初始化 ----

window.addEventListener('DOMContentLoaded', async () => {
    document.getElementById('loadPlaylists').addEventListener('click', loadPlaylists);
    document.getElementById('selectAllBtn').addEventListener('click', selectAll);
    document.getElementById('deselectAllBtn').addEventListener('click', deselectAll);
    document.getElementById('startBtn').addEventListener('click', startScrape);
    document.getElementById('stopBtn').addEventListener('click', stopScrape);

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
