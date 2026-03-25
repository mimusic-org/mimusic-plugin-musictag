let currentTaskId = null;
let isScraping = false;
let loadedPlaylists = []; // 存储加载的歌单列表

/**
 * 从 localStorage 获取认证 Token
 */
function getAuthToken() {
    try {
        var authData = localStorage.getItem('mimusic-auth');
        if (authData) {
            var auth = JSON.parse(authData);
            return auth.accessToken || '';
        }
    } catch (e) {
        console.error('获取 Token 失败:', e);
    }
    return '';
}

/**
 * 获取认证头
 */
function getAuthHeaders() {
    var headers = { 'Content-Type': 'application/json' };
    var token = getAuthToken();
    if (token) {
        headers['Authorization'] = 'Bearer ' + token;
    }
    return headers;
}

// 页面加载时检查是否有未完成的任务
window.addEventListener('DOMContentLoaded', async () => {
    // 绑定加载歌单按钮事件
    document.getElementById('loadPlaylists').addEventListener('click', loadPlaylists);
    
    // 绑定全选/取消全选按钮事件
    document.getElementById('selectAll').addEventListener('click', selectAll);
    document.getElementById('deselectAll').addEventListener('click', deselectAll);
    
    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/status?task_id=current', {
            headers: getAuthHeaders()
        });
        const progress = await response.json();
        
        // 如果任务正在运行，恢复 UI 状态并继续轮询
        if (progress.status === 'running') {
            currentTaskId = 'current';
            isScraping = true;
            
            const startBtn = document.getElementById('startScrape');
            const stopBtn = document.getElementById('stopScrape');
            const progressContainer = document.getElementById('progressContainer');
            
            startBtn.disabled = true;
            startBtn.textContent = '刮削中...';
            stopBtn.style.display = 'inline-block';
            progressContainer.style.display = 'block';
            
            const progressFill = document.getElementById('progressFill');
            const progressText = document.getElementById('progressText');
            const progressDetail = document.getElementById('progressDetail');
            const resultsDiv = document.getElementById('results');
            
            pollProgress(currentTaskId, progressFill, progressText, progressDetail, resultsDiv, startBtn, stopBtn);
        } else if (progress.status === 'completed_with_errors' || (progress.status === 'completed' && progress.failed > 0)) {
            // 显示上次刮削的结果
            const resultsDiv = document.getElementById('results');
            showResultsFromProgress(progress, resultsDiv);
        }
    } catch (error) {
        console.error('获取任务状态失败:', error);
    }
});

document.getElementById('startScrape').addEventListener('click', startScrape);
document.getElementById('stopScrape').addEventListener('click', stopScrape);

// 加载歌单列表
async function loadPlaylists() {
    const loadBtn = document.getElementById('loadPlaylists');
    const playlistSelector = document.getElementById('playlistSelector');
    const selectAllBtn = document.getElementById('selectAll');
    const deselectAllBtn = document.getElementById('deselectAll');
    
    loadBtn.disabled = true;
    loadBtn.textContent = '加载中...';
    
    try {
        const response = await fetch('/api/v1/playlists?limit=100000', {
            headers: getAuthHeaders()
        });
        
        if (!response.ok) {
            throw new Error('加载歌单失败');
        }
        
        const result = await response.json();
        
        if (!result.playlists || result.playlists.length === 0) {
            alert('暂无歌单');
            loadBtn.disabled = false;
            loadBtn.textContent = '加载歌单列表';
            return;
        }
        
        loadedPlaylists = result.playlists;
        
        // 清空并填充 checkbox 列表
        const playlistCheckboxList = document.getElementById('playlistCheckboxList');
        playlistCheckboxList.innerHTML = '';
        result.playlists.forEach(playlist => {
            const checkboxItem = document.createElement('div');
            checkboxItem.className = 'playlist-checkbox-item';
            
            const checkbox = document.createElement('input');
            checkbox.type = 'checkbox';
            checkbox.id = 'playlist-' + playlist.id;
            checkbox.value = playlist.id;
            checkbox.className = 'playlist-checkbox';
            
            const label = document.createElement('label');
            label.htmlFor = 'playlist-' + playlist.id;
            label.className = 'playlist-checkbox-label';
            label.textContent = playlist.name + (playlist.type === 'radio' ? ' (电台)' : '');
            
            checkboxItem.appendChild(checkbox);
            checkboxItem.appendChild(label);
            playlistCheckboxList.appendChild(checkboxItem);
        });
        
        // 绑定 checkbox 变化事件
        const checkboxes = playlistCheckboxList.querySelectorAll('.playlist-checkbox');
        checkboxes.forEach(checkbox => {
            checkbox.addEventListener('change', updateSelectedCount);
        });
        
        // 显示选择器和全选/取消全选按钮
        playlistSelector.style.display = 'flex';
        selectAllBtn.style.display = 'inline-block';
        deselectAllBtn.style.display = 'inline-block';
        loadBtn.textContent = '重新加载';
        loadBtn.disabled = false;
        
    } catch (error) {
        alert('加载歌单失败：' + error.message);
        loadBtn.disabled = false;
        loadBtn.textContent = '加载歌单列表';
    }
}

// 全选
function selectAll() {
    const checkboxes = document.querySelectorAll('.playlist-checkbox');
    checkboxes.forEach(checkbox => {
        checkbox.checked = true;
    });
    updateSelectedCount();
}

// 取消全选
function deselectAll() {
    const checkboxes = document.querySelectorAll('.playlist-checkbox');
    checkboxes.forEach(checkbox => {
        checkbox.checked = false;
    });
    updateSelectedCount();
}

// 更新已选择歌单数量显示
function updateSelectedCount() {
    const checkboxes = document.querySelectorAll('.playlist-checkbox:checked');
    const selectedCount = document.getElementById('selectedCount');
    const startBtn = document.getElementById('startScrape');
    
    const count = checkboxes.length;
    
    selectedCount.textContent = '已选择 ' + count + ' 个歌单';
    
    // 只有选择了歌单才能开始刮削
    startBtn.disabled = count === 0;
}

async function startScrape() {
    if (isScraping) {
        alert('已有任务正在运行，请先停止当前任务');
        return;
    }
    
    const checkboxes = document.querySelectorAll('.playlist-checkbox:checked');
    
    if (checkboxes.length === 0) {
        alert('请至少选择一个歌单');
        return;
    }
    
    const selectedPlaylistIds = Array.from(checkboxes).map(cb => parseInt(cb.value));
    
    const startBtn = document.getElementById('startScrape');
    const stopBtn = document.getElementById('stopScrape');
    const progressContainer = document.getElementById('progressContainer');
    const progressFill = document.getElementById('progressFill');
    const progressText = document.getElementById('progressText');
    const progressDetail = document.getElementById('progressDetail');
    const resultsDiv = document.getElementById('results');
    
    // 更新按钮状态
    isScraping = true;
    startBtn.disabled = true;
    startBtn.textContent = '刮削中...';
    stopBtn.style.display = 'inline-block';
    progressContainer.style.display = 'block';
    resultsDiv.style.display = 'none';
    resultsDiv.className = 'results';
    
    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/batch', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ playlist_ids: selectedPlaylistIds })
        });
        
        const result = await response.json();
        
        if (result.success) {
            // 轮询进度
            currentTaskId = result.task_id;
            await pollProgress(currentTaskId, progressFill, progressText, progressDetail, resultsDiv, startBtn, stopBtn);
        } else {
            showResults('error', '批量刮削失败：' + (result.message || '未知错误'), resultsDiv);
            resetButtons(startBtn, stopBtn);
        }
    } catch (error) {
        showResults('error', '请求失败：' + error.message, resultsDiv);
        resetButtons(startBtn, stopBtn);
    }
}

async function stopScrape() {
    if (!currentTaskId) {
        return;
    }
    
    const startBtn = document.getElementById('startScrape');
    const stopBtn = document.getElementById('stopScrape');
    
    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/stop', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ task_id: currentTaskId })
        });
        
        const result = await response.json();
        
        if (result.success) {
            showResults('error', '已停止刮削', document.getElementById('results'));
            resetButtons(startBtn, stopBtn);
            currentTaskId = null;
            isScraping = false;
        } else {
            alert('停止失败：' + (result.message || '未知错误'));
        }
    } catch (error) {
        alert('停止请求失败：' + error.message);
    }
}

function resetButtons(startBtn, stopBtn) {
    startBtn.disabled = false;
    startBtn.textContent = '开始刮削';
    stopBtn.style.display = 'none';
    isScraping = false;
}

async function pollProgress(taskId, progressFill, progressText, progressDetail, resultsDiv, startBtn, stopBtn) {
    const maxAttempts = 200;
    let attempts = 0;
    
    while (attempts < maxAttempts) {
        try {
            const response = await fetch(`/api/v1/plugin/musictag/api/scrape/status?task_id=${taskId}`, {
                headers: getAuthHeaders()
            });
            const progress = await response.json();
            
            // 检查任务是否结束（完成、失败、已停止或有错误的完成）
            if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'stopped' || progress.status === 'completed_with_errors') {
                updateProgress(progress, progressFill, progressText, progressDetail);
                showResultsFromProgress(progress, resultsDiv);
                resetButtons(startBtn, stopBtn);
                currentTaskId = null;
                break;
            }
            
            updateProgress(progress, progressFill, progressText, progressDetail);
            await new Promise(resolve => setTimeout(resolve, 500));
            attempts++;
        } catch (error) {
            console.error('轮询失败:', error);
            break;
        }
    }
}

function updateProgress(progress, progressFill, progressText, progressDetail) {
    const percent = progress.total > 0 ? Math.round((progress.current / progress.total) * 100) : 0;
    progressFill.style.width = percent + '%';
    
    // 主进度文本
    progressText.textContent = `进度：${progress.current}/${progress.total} (${percent}%) | 成功：${progress.success} | 失败：${progress.failed}`;
    
    // 详细信息：当前正在处理的歌曲
    if (progress.current_song) {
        progressDetail.textContent = `当前处理：${progress.current_song}`;
        progressDetail.style.display = 'block';
    } else {
        progressDetail.style.display = 'none';
    }
}

function showResultsFromProgress(progress, resultsDiv) {
    resultsDiv.style.display = 'block';
    
    let html = '';
    
    if (progress.success > 0) {
        html += `<div class="result-item success">✓ 成功刮削 ${progress.success} 首歌曲</div>`;
    }
    
    if (progress.failed > 0) {
        html += `<div class="result-item error">✗ 失败：${progress.failed} 首</div>`;
    }
    
    // 显示失败歌曲列表
    if (progress.failed_songs && progress.failed_songs.length > 0) {
        html += '<div class="failed-songs-container"><h3>失败歌曲列表</h3><ul class="failed-songs-list">';
        progress.failed_songs.forEach(song => {
            html += `<li class="failed-song-item">${escapeHtml(song.title)} - ${escapeHtml(song.artist)}</li>`;
        });
        html += '</ul></div>';
        
        // 添加重新刮削按钮
        html += `<button id="retryFailedBtn" class="btn btn-warning" style="margin-top: 10px;">重新刮削失败歌曲</button>`;
    }
    
    if (progress.error) {
        html += `<div class="result-detail">${escapeHtml(progress.error)}</div>`;
    }
    
    if (progress.success === 0 && progress.failed === 0) {
        html += `<div class="result-item">ℹ️ 没有找到需要刮削的歌曲</div>`;
    }
    
    resultsDiv.innerHTML = html;
    resultsDiv.className = 'results ' + (progress.failed > 0 ? 'error' : 'success');
    
    // 绑定重新刮削按钮事件
    const retryBtn = document.getElementById('retryFailedBtn');
    if (retryBtn) {
        retryBtn.addEventListener('click', () => retryFailedSongs(progress.task_id));
    }
}

function showResults(type, message, resultsDiv) {
    resultsDiv.className = 'results ' + type;
    resultsDiv.innerHTML = `<div class="result-item">${type === 'success' ? '✓' : '✗'} ${escapeHtml(message)}</div>`;
    resultsDiv.style.display = 'block';
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

async function retryFailedSongs(taskId) {
    const retryBtn = document.getElementById('retryFailedBtn');
    if (retryBtn) {
        retryBtn.disabled = true;
        retryBtn.textContent = '重新刮削中...';
    }
    
    try {
        const response = await fetch('/api/v1/plugin/musictag/api/scrape/retry-failed', {
            method: 'POST',
            headers: getAuthHeaders(),
            body: JSON.stringify({ task_id: taskId })
        });
        
        const result = await response.json();
        
        if (result.success) {
            // 开始轮询新任务的进度
            currentTaskId = result.task_id;
            
            const progressContainer = document.getElementById('progressContainer');
            const progressFill = document.getElementById('progressFill');
            const progressText = document.getElementById('progressText');
            const progressDetail = document.getElementById('progressDetail');
            const resultsDiv = document.getElementById('results');
            const startBtn = document.getElementById('startScrape');
            const stopBtn = document.getElementById('stopScrape');
            
            isScraping = true;
            startBtn.disabled = true;
            startBtn.textContent = '刮削中...';
            stopBtn.style.display = 'inline-block';
            progressContainer.style.display = 'block';
            resultsDiv.style.display = 'none';
            
            await pollProgress(currentTaskId, progressFill, progressText, progressDetail, resultsDiv, startBtn, stopBtn);
        } else {
            alert('重新刮削失败：' + (result.message || '未知错误'));
            if (retryBtn) {
                retryBtn.disabled = false;
                retryBtn.textContent = '重新刮削失败歌曲';
            }
        }
    } catch (error) {
        alert('重新刮削请求失败：' + error.message);
        if (retryBtn) {
            retryBtn.disabled = false;
            retryBtn.textContent = '重新刮削失败歌曲';
        }
    }
}