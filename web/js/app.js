// RoamBench — Client Application
(function() {
    'use strict';

    const MIN_EDITOR_HEIGHT = 140;
    const MIN_TERMINAL_HEIGHT = 140;
    const EMBEDDED_FILE_PANEL_DEFAULT_WIDTH_RATIO = 0.38;
    const EMBEDDED_FILE_PANEL_DEFAULT_MAX_WIDTH = 520;
    const EMBEDDED_FILE_PANEL_MAX_WIDTH = 920;
    const EMBEDDED_FILE_PANEL_MIN_TERMINAL_WIDTH = 320;
    const TERMINAL_SETTINGS_STORAGE_KEY = 'roambench.terminal-settings.v2';
    const TERMINAL_SETTINGS_FALLBACK_KEYS = ['roambench.terminal-settings'];
    const LANGUAGE_STORAGE_KEY = 'roambench.language.v1';
    const TERMINAL_WORKSPACES_STORAGE_KEY = 'roambench.terminal-workspaces.v1';
    const EDITOR_DRAFTS_STORAGE_KEY = 'roambench.editor-drafts.v1';
    const EDITOR_UI_STORAGE_KEY = 'roambench.editor-ui.v1';
    const UPLOAD_RESPONSE_GRACE_MS = 6000;
    const DEFAULT_LANGUAGE = 'en';
    const WORKSPACE_LAYOUT_SLOT_COUNTS = { '1': 1, '2': 2, '4': 4, '4w': 4 };
    const BASE_PATH = normalizeBasePath(typeof window.__BASE_PATH__ === 'string' ? window.__BASE_PATH__ : '');
    const DEFAULT_TERMINAL_NAME_PREFIXES = {
        en: 'Term ',
        'zh-CN': '终端 ',
        ja: '端末 '
    };

    function normalizeBasePath(value) {
        let normalized = typeof value === 'string' ? value.trim() : '';

        if (!normalized || normalized === '/') {
            return '';
        }
        if (!normalized.startsWith('/')) {
            normalized = '/' + normalized;
        }
        normalized = normalized.replace(/\/{2,}/g, '/').replace(/\/+$/, '');
        return normalized === '/' ? '' : normalized;
    }

    function withBasePath(url) {
        if (typeof url !== 'string' || !url) {
            return url;
        }
        if (/^(?:[a-z][a-z0-9+.-]*:)?\/\//i.test(url) || url.startsWith('data:') || url.startsWith('blob:')) {
            return url;
        }
        if (!url.startsWith('/')) {
            return url;
        }
        if (!BASE_PATH || url === BASE_PATH || url.indexOf(BASE_PATH + '/') === 0) {
            return url;
        }
        return BASE_PATH + url;
    }
    const TRANSLATIONS = {
        en: {
            'login.subtitle': 'Web Terminal',
            'login.language': 'Language',
            'login.usernamePlaceholder': 'Username',
            'login.passwordPlaceholder': 'Password',
            'login.showPassword': 'Show password',
            'login.connect': 'Connect',
            'auth.enterCredentials': 'Please enter username and password',
            'auth.relogin': 'Please sign in again.',
            'auth.sessionExpiredRelogin': 'Session expired. Please sign in again.',
            'auth.sessionExpired': 'Session expired',
            'auth.disconnectConfirm': 'Disconnect from server?',
            'header.logout': 'Logout',
            'header.newTerminal': 'New Terminal',
            'header.settings': 'Settings',
            'header.files': 'Files',
            'header.maximizeTerminal': 'Maximize Terminal',
            'workspace.newView': 'New View',
            'workspace.defaultName': 'View {number}',
            'workspace.renameView': 'Rename View',
            'workspace.renamePrompt': 'Rename view:',
            'workspace.layoutSwitcher': 'Terminal Layout',
            'workspace.layout.single': 'Single Terminal',
            'workspace.layout.double': 'Two Terminals',
            'workspace.layout.quad': 'Four Terminals',
            'workspace.layout.quadWide': 'Four Columns',
            'workspace.emptySlot': 'Empty pane',
            'workspace.emptyHint': 'Choose a terminal from the list or create a new one for this pane.',
            'workspace.selectTerminal': 'Select terminal',
            'workspace.createHere': 'Create new terminal',
            'workspace.renameTerminal': 'Rename Terminal',
            'workspace.closeTerminal': 'Close Terminal',
            'workspace.closeView': 'Close View',
            'workspace.duplicateHint': 'This terminal is already visible in another pane.',
            'viewer.tab': 'Viewer',
            'viewer.emptyTitle': 'Paste from clipboard',
            'viewer.emptyHint': 'Paste text or an image here, or open an image, PDF, PPTX, DOCX, XLSX, or text file from Files.',
            'viewer.copyText': 'Copy Text',
            'viewer.edit': 'Edit',
            'viewer.pptxLoading': 'Loading PPTX viewer and rendering slides...',
            'viewer.pptxUnavailable': 'PPTX preview is experimental and could not load in this browser. Use Download for now.',
            'viewer.pptxPrev': 'Previous slide',
            'viewer.pptxNext': 'Next slide',
            'viewer.pptxSlideStatus': 'Slide {current} / {total}',
            'viewer.docxLoading': 'Loading DOCX preview...',
            'viewer.docxUnavailable': 'DOCX preview could not load in this browser. Use Download for now.',
            'viewer.xlsxLoading': 'Loading XLSX workbook preview...',
            'viewer.xlsxUnavailable': 'XLSX preview could not load in this browser. Use Download for now.',
            'viewer.new': 'New',
            'viewer.newDraft.filename': 'File name',
            'viewer.newDraft.hint': 'Paste text or an image here, then Save.',
            'viewer.newDraft.textPlaceholder': 'Paste or type text...',
            'viewer.newDraft.clearImage': 'Clear image',
            'viewer.newDraft.cancel': 'Cancel',
            'viewer.newDraft.save': 'Save',
            'viewer.newDraft.switchToImage': 'Replace the text you typed with the pasted image?',
            'viewer.newDraft.switchToText': 'Discard the pasted image and switch to text?',
            'viewer.newDraft.filenameRequired': 'File name is required.',
            'viewer.newDraft.filenameInvalid': 'File name cannot contain slashes.',
            'viewer.newDraft.noContent': 'Paste text or an image before saving.',
            'viewer.newDraft.saving': 'Saving...',
            'viewer.newDraft.saved': 'Saved as {name}',
            'viewer.newDraft.saveFailed': 'Save failed: {message}',
            'viewer.newDraft.imagePasted': 'Image pasted ({size})',
            'viewer.newDraft.discardConfirm': 'Discard this draft?',
            'memory.title': 'Memory',
            'memory.tooltip': 'RoamBench RSS {app} / System Used {used} / System RAM {total}',
            'memory.unavailableTooltip': 'Memory status unavailable',
            'editor.noFileOpen': 'No file open',
            'editor.save': 'Save',
            'editor.saveAs': 'Save As',
            'editor.saveAsShort': 'As',
            'editor.maximize': 'Maximize Editor',
            'editor.close': 'Close Editor',
            'editor.saving': 'Saving...',
            'editor.modified': 'Unsaved changes',
            'editor.saved': 'Saved',
            'editor.closeWithoutSaving': 'Close "{name}" without saving?',
            'editor.saveFailed': 'Save failed: {message}',
            'editor.saveAsPrompt': 'Save as path:',
            'editor.pathAlreadyOpen': 'An editor tab is already open for "{path}".',
            'editor.find': 'Find / Replace',
            'editor.findShort': 'Find',
            'editor.findPlaceholder': 'Find',
            'editor.replacePlaceholder': 'Replace',
            'editor.replaceShort': 'Replace',
            'editor.replaceAllShort': 'All',
            'editor.previousShort': 'Prev',
            'editor.nextShort': 'Next',
            'editor.goToLine': 'Go to Line',
            'editor.goToLineShort': 'Go',
            'editor.goToLinePlaceholder': 'Line:Col',
            'editor.lineNumbers': 'Line Numbers',
            'editor.lineNumbersShort': '123',
            'editor.cursorPosition': 'Ln {line}, Col {column}',
            'editor.searchHint': 'Type to search',
            'editor.searchNoResults': 'No matches',
            'editor.searchCount': '{current} / {total}',
            'settings.title': 'Settings',
            'settings.subtitle': 'Fonts, colors, and language are stored in this browser and apply live.',
            'settings.resetTerminal': 'Reset Terminal Style',
            'settings.close': 'Close',
            'settings.language': 'Interface Language',
            'settings.fontFamily': 'Font Family',
            'settings.fontSize': 'Font Size',
            'settings.themePreset': 'Theme Preset',
            'settings.theme.classic': 'Classic Dark',
            'settings.theme.midnight': 'Midnight Blue',
            'settings.theme.forest': 'Forest',
            'settings.theme.amber': 'Amber',
            'settings.theme.custom': 'Custom',
            'settings.cursorBlink': 'Cursor Blink',
            'settings.ansiPalette': 'ANSI Palette',
            'settings.ansiNote': 'Used by Codex, git diff, ls, and other ANSI-colored tools.',
            'settings.preview': 'Preview',
            'settings.previewNote': 'Changes apply to open terminals immediately',
            'settings.previewCaption': 'term preview',
            'settings.preview.src': 'src',
            'settings.preview.warning': 'warning:',
            'settings.preview.pending': 'session check pending',
            'settings.preview.codex': 'codex',
            'settings.preview.review': 'review',
            'settings.preview.selection': 'Term 1  logs  notes.txt',
            'settings.preview.ready': 'ready',
            'language.en': 'English',
            'language.zh-CN': '简体中文',
            'language.ja': '日本語',
            'color.background': 'Background',
            'color.foreground': 'Foreground',
            'color.cursor': 'Cursor',
            'color.selection': 'Selection',
            'color.black': 'Black',
            'color.red': 'Red',
            'color.green': 'Green',
            'color.yellow': 'Yellow',
            'color.blue': 'Blue',
            'color.magenta': 'Magenta',
            'color.cyan': 'Cyan',
            'color.white': 'White',
            'color.brightBlack': 'Bright Black',
            'color.brightRed': 'Bright Red',
            'color.brightGreen': 'Bright Green',
            'color.brightYellow': 'Bright Yellow',
            'color.brightBlue': 'Bright Blue',
            'color.brightMagenta': 'Bright Magenta',
            'color.brightCyan': 'Bright Cyan',
            'color.brightWhite': 'Bright White',
            'terminal.defaultName': 'Term {number}',
            'terminal.rename': 'Rename',
            'terminal.renamePrompt': 'Rename terminal:',
            'terminal.renameFailed': 'Rename failed: {message}',
            'terminal.loadFailed': 'Failed to load terminals: {message}',
            'terminal.createFailed': 'Error: {message}',
            'terminal.reconnected': 'reconnected',
            'terminal.connectionError': 'connection error',
            'terminal.disconnected': 'disconnected',
            'terminal.copySelection': 'Copy Selection',
            'files.showHidden': 'Show Hidden',
            'files.hideHidden': 'Hide Hidden',
            'files.upload': 'Upload',
            'files.refresh': 'Refresh',
            'files.home': 'Home',
            'files.up': 'Up',
            'files.selectMode': 'Select',
            'files.newFile': 'New File',
            'files.newFolder': 'New Folder',
            'files.noVisibleFiles': 'No visible files',
            'files.noMatches': 'No files match the current filter',
            'files.filterPlaceholder': 'Filter current directory',
            'files.columnName': 'Name',
            'files.columnSize': 'Size',
            'files.columnModified': 'Modified',
            'files.sortAscending': 'Ascending',
            'files.sortDescending': 'Descending',
            'files.sortBy': 'Sort by {column}: {direction}',
            'files.view': 'View',
            'files.copy': 'Copy',
            'files.move': 'Move',
            'files.rename': 'Rename',
            'files.download': 'Download',
            'files.delete': 'Delete',
            'files.downloadInstead': 'Cannot open "{name}" in editor: {message}\n\nDownload it instead?',
            'files.deleteConfirm': 'Delete "{name}"?',
            'files.newFilePrompt': 'New file path:',
            'files.newFolderPrompt': 'New folder path:',
            'files.copyPrompt': 'Copy to:',
            'files.copyFailed': 'Copy failed: {message}',
            'files.movePrompt': 'Move selected items to directory:',
            'files.moveFailed': 'Move failed: {message}',
            'files.renamePrompt': 'Rename or move to:',
            'files.renameFailed': 'Rename failed: {message}',
            'files.dropToUpload': 'Drop files to upload',
            'files.cannotOverwriteDirectory': '"{path}" is an existing directory.',
            'files.selectionReady': 'Select files',
            'files.selectionSummary': '{count} selected',
            'files.selectAll': 'Select All',
            'files.selectionCancel': 'Cancel',
            'files.bulkDeleteConfirm': 'Delete {count} selected items?',
            'upload.uploading': 'Uploading',
            'upload.progress': 'Uploading {current} of {total}',
            'upload.complete': 'Upload complete',
            'upload.failed': 'Upload failed',
            'upload.completeSingle': '{name} uploaded',
            'upload.completeMultiple': '{count} files uploaded • {size}',
            'upload.failedFor': 'Upload failed for {name}',
            'upload.canceled': 'Upload canceled',
            'upload.cancel': 'Cancel',
            'common.requestFailed': 'Request failed',
            'common.error': 'Error: {message}',
            'image.preview': 'Image Preview'
        },
        'zh-CN': {
            'login.subtitle': '网页终端',
            'login.language': '语言',
            'login.usernamePlaceholder': '用户名',
            'login.passwordPlaceholder': '密码',
            'login.showPassword': '显示密码',
            'login.connect': '连接',
            'auth.enterCredentials': '请输入用户名和密码',
            'auth.relogin': '请重新登录。',
            'auth.sessionExpiredRelogin': '会话已过期，请重新登录。',
            'auth.sessionExpired': '会话已过期',
            'auth.disconnectConfirm': '要断开与服务器的连接吗？',
            'header.logout': '退出登录',
            'header.newTerminal': '新建终端',
            'header.settings': '设置',
            'header.files': '文件',
            'header.maximizeTerminal': '最大化终端',
            'workspace.newView': '新建视图',
            'workspace.defaultName': '视图 {number}',
            'workspace.renameView': '重命名视图',
            'workspace.renamePrompt': '重命名视图：',
            'workspace.layoutSwitcher': '终端布局',
            'workspace.layout.single': '单终端',
            'workspace.layout.double': '双终端',
            'workspace.layout.quad': '四终端',
            'workspace.layout.quadWide': '四列布局',
            'workspace.emptySlot': '空白窗格',
            'workspace.emptyHint': '可从列表选择终端，或直接为这个窗格新建一个终端。',
            'workspace.selectTerminal': '选择终端',
            'workspace.createHere': '在此新建终端',
            'workspace.renameTerminal': '重命名终端',
            'workspace.closeTerminal': '关闭终端',
            'workspace.closeView': '关闭视图',
            'workspace.duplicateHint': '这个终端已经显示在另一个窗格中。',
            'viewer.tab': '查看器',
            'viewer.emptyTitle': '从剪贴板粘贴',
            'viewer.emptyHint': '在这里粘贴文字或截图，或在文件标签中打开图片、PDF、PPTX、DOCX、XLSX 或文本文件。',
            'viewer.copyText': '复制全文',
            'viewer.edit': '编辑',
            'viewer.pptxLoading': '正在加载 PPTX 预览器并渲染幻灯片……',
            'viewer.pptxUnavailable': 'PPTX 预览仍是实验功能，当前浏览器中加载失败，请先使用下载。',
            'viewer.pptxPrev': '上一页',
            'viewer.pptxNext': '下一页',
            'viewer.pptxSlideStatus': '第 {current} / {total} 页',
            'viewer.docxLoading': '正在加载 DOCX 预览……',
            'viewer.docxUnavailable': 'DOCX 预览在当前浏览器中加载失败，请先使用下载。',
            'viewer.xlsxLoading': '正在加载 XLSX 工作簿预览……',
            'viewer.xlsxUnavailable': 'XLSX 预览在当前浏览器中加载失败，请先使用下载。',
            'viewer.new': '新建',
            'viewer.newDraft.filename': '文件名',
            'viewer.newDraft.hint': '在此粘贴文本或图片，然后保存。',
            'viewer.newDraft.textPlaceholder': '粘贴或输入文本……',
            'viewer.newDraft.clearImage': '清除图片',
            'viewer.newDraft.cancel': '取消',
            'viewer.newDraft.save': '保存',
            'viewer.newDraft.switchToImage': '要用粘贴的图片替换已输入的文本吗？',
            'viewer.newDraft.switchToText': '丢弃已粘贴的图片并切换为文本？',
            'viewer.newDraft.filenameRequired': '请输入文件名。',
            'viewer.newDraft.filenameInvalid': '文件名不能包含斜杠。',
            'viewer.newDraft.noContent': '保存前请先粘贴文本或图片。',
            'viewer.newDraft.saving': '保存中……',
            'viewer.newDraft.saved': '已保存为 {name}',
            'viewer.newDraft.saveFailed': '保存失败：{message}',
            'viewer.newDraft.imagePasted': '已粘贴图片（{size}）',
            'viewer.newDraft.discardConfirm': '放弃这份草稿吗？',
            'memory.title': '内存',
            'memory.tooltip': 'RoamBench 占用 {app} / 系统已用 {used} / 系统总内存 {total}',
            'memory.unavailableTooltip': '内存状态暂不可用',
            'editor.noFileOpen': '未打开文件',
            'editor.save': '保存',
            'editor.saveAs': '另存为',
            'editor.saveAsShort': '另存',
            'editor.maximize': '最大化编辑器',
            'editor.close': '关闭编辑器',
            'editor.saving': '保存中...',
            'editor.modified': '未保存更改',
            'editor.saved': '已保存',
            'editor.closeWithoutSaving': '要在不保存的情况下关闭“{name}”吗？',
            'editor.saveFailed': '保存失败：{message}',
            'editor.saveAsPrompt': '另存为路径：',
            'editor.pathAlreadyOpen': '路径“{path}”已经有打开的编辑器标签。',
            'editor.find': '查找 / 替换',
            'editor.findShort': '查找',
            'editor.findPlaceholder': '查找',
            'editor.replacePlaceholder': '替换为',
            'editor.replaceShort': '替换',
            'editor.replaceAllShort': '全部',
            'editor.previousShort': '上一个',
            'editor.nextShort': '下一个',
            'editor.goToLine': '跳转到行',
            'editor.goToLineShort': '跳转',
            'editor.goToLinePlaceholder': '行:列',
            'editor.lineNumbers': '行号',
            'editor.lineNumbersShort': '123',
            'editor.cursorPosition': '第 {line} 行，第 {column} 列',
            'editor.searchHint': '输入后开始查找',
            'editor.searchNoResults': '没有匹配项',
            'editor.searchCount': '{current} / {total}',
            'settings.title': '设置',
            'settings.subtitle': '字体、颜色和语言会保存在当前浏览器中，并立即生效。',
            'settings.resetTerminal': '重置终端样式',
            'settings.close': '关闭',
            'settings.language': '界面语言',
            'settings.fontFamily': '字体',
            'settings.fontSize': '字号',
            'settings.themePreset': '主题预设',
            'settings.theme.classic': '经典深色',
            'settings.theme.midnight': '午夜蓝',
            'settings.theme.forest': '森林',
            'settings.theme.amber': '琥珀',
            'settings.theme.custom': '自定义',
            'settings.cursorBlink': '光标闪烁',
            'settings.ansiPalette': 'ANSI 调色板',
            'settings.ansiNote': '用于 Codex、git diff、ls 等 ANSI 彩色工具。',
            'settings.preview': '预览',
            'settings.previewNote': '修改会立即应用到已打开的终端',
            'settings.previewCaption': '终端预览',
            'settings.preview.src': 'src',
            'settings.preview.warning': '警告：',
            'settings.preview.pending': '会话检查待处理',
            'settings.preview.codex': 'codex',
            'settings.preview.review': '审阅',
            'settings.preview.selection': '终端 1  日志  笔记.txt',
            'settings.preview.ready': '就绪',
            'language.en': 'English',
            'language.zh-CN': '简体中文',
            'language.ja': '日本語',
            'color.background': '背景',
            'color.foreground': '前景',
            'color.cursor': '光标',
            'color.selection': '选区',
            'color.black': '黑色',
            'color.red': '红色',
            'color.green': '绿色',
            'color.yellow': '黄色',
            'color.blue': '蓝色',
            'color.magenta': '洋红',
            'color.cyan': '青色',
            'color.white': '白色',
            'color.brightBlack': '亮黑',
            'color.brightRed': '亮红',
            'color.brightGreen': '亮绿',
            'color.brightYellow': '亮黄',
            'color.brightBlue': '亮蓝',
            'color.brightMagenta': '亮洋红',
            'color.brightCyan': '亮青',
            'color.brightWhite': '亮白',
            'terminal.defaultName': '终端 {number}',
            'terminal.rename': '重命名',
            'terminal.renamePrompt': '重命名终端：',
            'terminal.renameFailed': '重命名失败：{message}',
            'terminal.loadFailed': '加载终端失败：{message}',
            'terminal.createFailed': '错误：{message}',
            'terminal.reconnected': '已重新连接',
            'terminal.connectionError': '连接错误',
            'terminal.disconnected': '已断开连接',
            'terminal.copySelection': '复制选中内容',
            'files.showHidden': '显示隐藏文件',
            'files.hideHidden': '不显示隐藏文件',
            'files.upload': '上传',
            'files.refresh': '刷新',
            'files.home': '主目录',
            'files.up': '上一级',
            'files.selectMode': '选择',
            'files.newFile': '新建文件',
            'files.newFolder': '新建文件夹',
            'files.noVisibleFiles': '没有可见文件',
            'files.noMatches': '没有文件匹配当前筛选',
            'files.filterPlaceholder': '筛选当前目录',
            'files.columnName': '名称',
            'files.columnSize': '大小',
            'files.columnModified': '修改时间',
            'files.sortAscending': '升序',
            'files.sortDescending': '降序',
            'files.sortBy': '按{column}排序：{direction}',
            'files.view': '查看',
            'files.copy': '复制',
            'files.move': '移动',
            'files.rename': '重命名',
            'files.download': '下载',
            'files.delete': '删除',
            'files.downloadInstead': '无法在编辑器中打开“{name}”：{message}\n\n要改为下载吗？',
            'files.deleteConfirm': '要删除“{name}”吗？',
            'files.newFilePrompt': '新文件路径：',
            'files.newFolderPrompt': '新建文件夹路径：',
            'files.copyPrompt': '复制到：',
            'files.copyFailed': '复制失败：{message}',
            'files.movePrompt': '将已选择项移动到目录：',
            'files.moveFailed': '移动失败：{message}',
            'files.renamePrompt': '重命名或移动到：',
            'files.renameFailed': '重命名失败：{message}',
            'files.dropToUpload': '拖拽文件到这里上传',
            'files.cannotOverwriteDirectory': '“{path}”是已存在的目录。',
            'files.selectionReady': '选择文件',
            'files.selectionSummary': '已选择 {count} 项',
            'files.selectAll': '全选',
            'files.selectionCancel': '取消',
            'files.bulkDeleteConfirm': '要删除已选择的 {count} 项吗？',
            'upload.uploading': '上传中',
            'upload.progress': '正在上传第 {current} / {total} 个',
            'upload.complete': '上传完成',
            'upload.failed': '上传失败',
            'upload.completeSingle': '已上传 {name}',
            'upload.completeMultiple': '已上传 {count} 个文件 • {size}',
            'upload.failedFor': '{name} 上传失败',
            'upload.canceled': '上传已取消',
            'upload.cancel': '取消',
            'common.requestFailed': '请求失败',
            'common.error': '错误：{message}',
            'image.preview': '图片预览'
        },
        ja: {
            'login.subtitle': 'Web ターミナル',
            'login.language': '言語',
            'login.usernamePlaceholder': 'ユーザー名',
            'login.passwordPlaceholder': 'パスワード',
            'login.showPassword': 'パスワードを表示',
            'login.connect': '接続',
            'auth.enterCredentials': 'ユーザー名とパスワードを入力してください',
            'auth.relogin': '再度サインインしてください。',
            'auth.sessionExpiredRelogin': 'セッションの有効期限が切れました。再度サインインしてください。',
            'auth.sessionExpired': 'セッションの有効期限が切れました',
            'auth.disconnectConfirm': 'サーバーから切断しますか？',
            'header.logout': 'ログアウト',
            'header.newTerminal': '新しい端末',
            'header.settings': '設定',
            'header.files': 'ファイル',
            'header.maximizeTerminal': '端末を最大化',
            'workspace.newView': '新しい表示',
            'workspace.defaultName': 'View {number}',
            'workspace.renameView': 'View 名を変更',
            'workspace.renamePrompt': 'View 名を変更:',
            'workspace.layoutSwitcher': '端末レイアウト',
            'workspace.layout.single': '単一端末',
            'workspace.layout.double': '2 分割端末',
            'workspace.layout.quad': '4 分割端末',
            'workspace.layout.quadWide': '4 列端末',
            'workspace.emptySlot': '空のペイン',
            'workspace.emptyHint': '一覧から端末を選ぶか、このペイン用に新しい端末を作成してください。',
            'workspace.selectTerminal': '端末を選択',
            'workspace.createHere': 'ここに新しい端末を作成',
            'workspace.renameTerminal': '端末名を変更',
            'workspace.closeTerminal': '端末を閉じる',
            'workspace.closeView': 'View を閉じる',
            'workspace.duplicateHint': 'この端末は別のペインですでに表示されています。',
            'viewer.tab': 'Viewer',
            'viewer.emptyTitle': 'クリップボードから貼り付け',
            'viewer.emptyHint': 'ここにテキストや画像を貼り付けるか、Files から画像、PDF、PPTX、DOCX、XLSX、テキストを開いてください。',
            'viewer.copyText': 'テキストをコピー',
            'viewer.edit': '編集',
            'viewer.pptxLoading': 'PPTX ビューアを読み込み、スライドを描画しています...',
            'viewer.pptxUnavailable': 'PPTX プレビューは実験機能のため、このブラウザでは読み込めませんでした。いったんダウンロードを使ってください。',
            'viewer.pptxPrev': '前のスライド',
            'viewer.pptxNext': '次のスライド',
            'viewer.pptxSlideStatus': '{current} / {total} 枚目',
            'viewer.docxLoading': 'DOCX プレビューを読み込んでいます...',
            'viewer.docxUnavailable': 'このブラウザでは DOCX プレビューを読み込めませんでした。いったんダウンロードを使ってください。',
            'viewer.xlsxLoading': 'XLSX ブックのプレビューを読み込んでいます...',
            'viewer.xlsxUnavailable': 'このブラウザでは XLSX プレビューを読み込めませんでした。いったんダウンロードを使ってください。',
            'viewer.new': '新規',
            'viewer.newDraft.filename': 'ファイル名',
            'viewer.newDraft.hint': 'ここにテキストまたは画像を貼り付けてから保存してください。',
            'viewer.newDraft.textPlaceholder': 'テキストを貼り付けまたは入力…',
            'viewer.newDraft.clearImage': '画像をクリア',
            'viewer.newDraft.cancel': 'キャンセル',
            'viewer.newDraft.save': '保存',
            'viewer.newDraft.switchToImage': '入力したテキストを貼り付けた画像で置き換えますか？',
            'viewer.newDraft.switchToText': '貼り付けた画像を破棄してテキストに切り替えますか？',
            'viewer.newDraft.filenameRequired': 'ファイル名を入力してください。',
            'viewer.newDraft.filenameInvalid': 'ファイル名にスラッシュは使用できません。',
            'viewer.newDraft.noContent': '保存前にテキストまたは画像を貼り付けてください。',
            'viewer.newDraft.saving': '保存中…',
            'viewer.newDraft.saved': '{name} として保存しました',
            'viewer.newDraft.saveFailed': '保存に失敗しました：{message}',
            'viewer.newDraft.imagePasted': '画像を貼り付けました（{size}）',
            'viewer.newDraft.discardConfirm': 'この下書きを破棄しますか？',
            'memory.title': 'メモリ',
            'memory.tooltip': 'RoamBench 使用量 {app} / システム使用中 {used} / システム総メモリ {total}',
            'memory.unavailableTooltip': 'メモリ状態を取得できません',
            'editor.noFileOpen': 'ファイルは開かれていません',
            'editor.save': '保存',
            'editor.saveAs': '別名で保存',
            'editor.saveAsShort': '別名',
            'editor.maximize': 'エディタを最大化',
            'editor.close': 'エディタを閉じる',
            'editor.saving': '保存中...',
            'editor.modified': '未保存の変更',
            'editor.saved': '保存済み',
            'editor.closeWithoutSaving': '「{name}」を保存せずに閉じますか？',
            'editor.saveFailed': '保存に失敗しました: {message}',
            'editor.saveAsPrompt': '保存先のパス:',
            'editor.pathAlreadyOpen': '「{path}」はすでに別のエディタタブで開かれています。',
            'editor.find': '検索 / 置換',
            'editor.findShort': '検索',
            'editor.findPlaceholder': '検索',
            'editor.replacePlaceholder': '置換',
            'editor.replaceShort': '置換',
            'editor.replaceAllShort': 'すべて',
            'editor.previousShort': '前へ',
            'editor.nextShort': '次へ',
            'editor.goToLine': '行へ移動',
            'editor.goToLineShort': '移動',
            'editor.goToLinePlaceholder': '行:列',
            'editor.lineNumbers': '行番号',
            'editor.lineNumbersShort': '123',
            'editor.cursorPosition': '行 {line}, 列 {column}',
            'editor.searchHint': '検索文字列を入力',
            'editor.searchNoResults': '一致なし',
            'editor.searchCount': '{current} / {total}',
            'settings.title': '設定',
            'settings.subtitle': 'フォント、色、言語はこのブラウザに保存され、すぐに反映されます。',
            'settings.resetTerminal': '端末の表示設定をリセット',
            'settings.close': '閉じる',
            'settings.language': '表示言語',
            'settings.fontFamily': 'フォント',
            'settings.fontSize': 'フォントサイズ',
            'settings.themePreset': 'テーマ',
            'settings.theme.classic': 'クラシックダーク',
            'settings.theme.midnight': 'ミッドナイトブルー',
            'settings.theme.forest': 'フォレスト',
            'settings.theme.amber': 'アンバー',
            'settings.theme.custom': 'カスタム',
            'settings.cursorBlink': 'カーソル点滅',
            'settings.ansiPalette': 'ANSI パレット',
            'settings.ansiNote': 'Codex、git diff、ls などの ANSI カラー表示に使われます。',
            'settings.preview': 'プレビュー',
            'settings.previewNote': '変更は開いている端末にすぐ反映されます',
            'settings.previewCaption': '端末プレビュー',
            'settings.preview.src': 'src',
            'settings.preview.warning': '警告:',
            'settings.preview.pending': 'セッション確認待ち',
            'settings.preview.codex': 'codex',
            'settings.preview.review': 'レビュー',
            'settings.preview.selection': '端末 1  ログ  メモ.txt',
            'settings.preview.ready': '準備完了',
            'language.en': 'English',
            'language.zh-CN': '简体中文',
            'language.ja': '日本語',
            'color.background': '背景',
            'color.foreground': '前景',
            'color.cursor': 'カーソル',
            'color.selection': '選択範囲',
            'color.black': '黒',
            'color.red': '赤',
            'color.green': '緑',
            'color.yellow': '黄',
            'color.blue': '青',
            'color.magenta': 'マゼンタ',
            'color.cyan': 'シアン',
            'color.white': '白',
            'color.brightBlack': '明るい黒',
            'color.brightRed': '明るい赤',
            'color.brightGreen': '明るい緑',
            'color.brightYellow': '明るい黄',
            'color.brightBlue': '明るい青',
            'color.brightMagenta': '明るいマゼンタ',
            'color.brightCyan': '明るいシアン',
            'color.brightWhite': '明るい白',
            'terminal.defaultName': '端末 {number}',
            'terminal.rename': '名前変更',
            'terminal.renamePrompt': '端末名を変更:',
            'terminal.renameFailed': '名前変更に失敗しました: {message}',
            'terminal.loadFailed': '端末の読み込みに失敗しました: {message}',
            'terminal.createFailed': 'エラー: {message}',
            'terminal.reconnected': '再接続しました',
            'terminal.connectionError': '接続エラー',
            'terminal.disconnected': '切断されました',
            'terminal.copySelection': '選択範囲をコピー',
            'files.showHidden': '隠しファイルを表示',
            'files.hideHidden': '隠しファイルを隠す',
            'files.upload': 'アップロード',
            'files.refresh': '更新',
            'files.home': 'ホーム',
            'files.up': '上へ',
            'files.selectMode': '選択',
            'files.newFile': '新しいファイル',
            'files.newFolder': '新しいフォルダ',
            'files.noVisibleFiles': '表示できるファイルがありません',
            'files.noMatches': '現在の絞り込みに一致するファイルがありません',
            'files.filterPlaceholder': '現在のディレクトリを絞り込み',
            'files.columnName': '名前',
            'files.columnSize': 'サイズ',
            'files.columnModified': '更新日時',
            'files.sortAscending': '昇順',
            'files.sortDescending': '降順',
            'files.sortBy': '{column}で並び替え: {direction}',
            'files.view': '表示',
            'files.copy': 'コピー',
            'files.move': '移動',
            'files.rename': '名前変更',
            'files.download': 'ダウンロード',
            'files.delete': '削除',
            'files.downloadInstead': '「{name}」をエディタで開けません: {message}\n\n代わりにダウンロードしますか？',
            'files.deleteConfirm': '「{name}」を削除しますか？',
            'files.newFilePrompt': '新しいファイルのパス:',
            'files.newFolderPrompt': '新しいフォルダのパス:',
            'files.copyPrompt': 'コピー先:',
            'files.copyFailed': 'コピーに失敗しました: {message}',
            'files.movePrompt': '選択した項目の移動先ディレクトリ:',
            'files.moveFailed': '移動に失敗しました: {message}',
            'files.renamePrompt': '名前変更または移動先:',
            'files.renameFailed': '名前変更に失敗しました: {message}',
            'files.dropToUpload': 'ここにドロップしてアップロード',
            'files.cannotOverwriteDirectory': '「{path}」は既存のディレクトリです。',
            'files.selectionReady': 'ファイルを選択',
            'files.selectionSummary': '{count} 件を選択中',
            'files.selectAll': 'すべて選択',
            'files.selectionCancel': 'キャンセル',
            'files.bulkDeleteConfirm': '選択した {count} 件を削除しますか？',
            'upload.uploading': 'アップロード中',
            'upload.progress': '{current} / {total} をアップロード中',
            'upload.complete': 'アップロード完了',
            'upload.failed': 'アップロード失敗',
            'upload.completeSingle': '{name} をアップロードしました',
            'upload.completeMultiple': '{count} 個のファイルをアップロードしました • {size}',
            'upload.failedFor': '{name} のアップロードに失敗しました',
            'upload.canceled': 'アップロードを中止しました',
            'upload.cancel': '中止',
            'common.requestFailed': 'リクエストに失敗しました',
            'common.error': 'エラー: {message}',
            'image.preview': '画像プレビュー'
        }
    };
    const TERMINAL_FONT_FAMILIES = [
        "'Menlo', 'Consolas', 'Courier New', monospace",
        "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
        "'SFMono-Regular', 'Consolas', 'Liberation Mono', monospace",
        "'IBM Plex Mono', 'Source Code Pro', monospace",
        "'Monaco', 'Courier New', monospace"
    ];
    const TERMINAL_ANSI_FIELDS = [
        { key: 'black', idSuffix: 'black' },
        { key: 'red', idSuffix: 'red' },
        { key: 'green', idSuffix: 'green' },
        { key: 'yellow', idSuffix: 'yellow' },
        { key: 'blue', idSuffix: 'blue' },
        { key: 'magenta', idSuffix: 'magenta' },
        { key: 'cyan', idSuffix: 'cyan' },
        { key: 'white', idSuffix: 'white' },
        { key: 'brightBlack', idSuffix: 'bright-black' },
        { key: 'brightRed', idSuffix: 'bright-red' },
        { key: 'brightGreen', idSuffix: 'bright-green' },
        { key: 'brightYellow', idSuffix: 'bright-yellow' },
        { key: 'brightBlue', idSuffix: 'bright-blue' },
        { key: 'brightMagenta', idSuffix: 'bright-magenta' },
        { key: 'brightCyan', idSuffix: 'bright-cyan' },
        { key: 'brightWhite', idSuffix: 'bright-white' }
    ];
    const TERMINAL_THEME_PRESETS = {
        classic: {
            background: '#1e1e1e',
            foreground: '#d4d4d4',
            cursor: '#569cd6',
            selection: '#569cd6',
            ansi: {
                black: '#1e1e1e',
                red: '#d16969',
                green: '#608b4e',
                yellow: '#d7ba7d',
                blue: '#569cd6',
                magenta: '#c586c0',
                cyan: '#4ec9b0',
                white: '#d4d4d4',
                brightBlack: '#808080',
                brightRed: '#f44747',
                brightGreen: '#b5cea8',
                brightYellow: '#ffd78c',
                brightBlue: '#9cdcfe',
                brightMagenta: '#d670d6',
                brightCyan: '#61d6d6',
                brightWhite: '#f3f3f3'
            }
        },
        midnight: {
            background: '#0f1720',
            foreground: '#d8e2f0',
            cursor: '#58a6ff',
            selection: '#58a6ff',
            ansi: {
                black: '#0f1720',
                red: '#ff7b72',
                green: '#7ee787',
                yellow: '#e3b341',
                blue: '#79c0ff',
                magenta: '#d2a8ff',
                cyan: '#56d4dd',
                white: '#d8e2f0',
                brightBlack: '#6e7681',
                brightRed: '#ffa198',
                brightGreen: '#56d364',
                brightYellow: '#f2cc60',
                brightBlue: '#a5d6ff',
                brightMagenta: '#e2c5ff',
                brightCyan: '#7ee7ff',
                brightWhite: '#f0f6fc'
            }
        },
        forest: {
            background: '#111915',
            foreground: '#d5e6d4',
            cursor: '#7bd88f',
            selection: '#7bd88f',
            ansi: {
                black: '#111915',
                red: '#ff7a90',
                green: '#7bd88f',
                yellow: '#f2c572',
                blue: '#69c0ff',
                magenta: '#d3a3ff',
                cyan: '#63e6d4',
                white: '#d5e6d4',
                brightBlack: '#6c7f72',
                brightRed: '#ff98aa',
                brightGreen: '#b8f397',
                brightYellow: '#ffd98a',
                brightBlue: '#8fd3ff',
                brightMagenta: '#e1c0ff',
                brightCyan: '#8ef3e4',
                brightWhite: '#f4fff2'
            }
        },
        amber: {
            background: '#1d1404',
            foreground: '#f4d59a',
            cursor: '#ffb347',
            selection: '#ffb347',
            ansi: {
                black: '#1d1404',
                red: '#ff8f6b',
                green: '#c8e37b',
                yellow: '#ffb347',
                blue: '#78c4ff',
                magenta: '#f4a9ff',
                cyan: '#72e3d2',
                white: '#f4d59a',
                brightBlack: '#8d6f3f',
                brightRed: '#ffb199',
                brightGreen: '#e4f39f',
                brightYellow: '#ffd27a',
                brightBlue: '#9ad7ff',
                brightMagenta: '#ffc6ff',
                brightCyan: '#9bf5ea',
                brightWhite: '#fff3d4'
            }
        }
    };
    const DEFAULT_TERMINAL_SETTINGS = {
        fontFamily: "'Menlo', 'Consolas', 'Courier New', monospace",
        fontSize: 14,
        cursorBlink: true,
        themePreset: 'classic',
        background: TERMINAL_THEME_PRESETS.classic.background,
        foreground: TERMINAL_THEME_PRESETS.classic.foreground,
        cursor: TERMINAL_THEME_PRESETS.classic.cursor,
        selection: TERMINAL_THEME_PRESETS.classic.selection,
        ansi: Object.assign({}, TERMINAL_THEME_PRESETS.classic.ansi)
    };
    const DEFAULT_UI_CONFIG = {
        title: 'RoamBench',
        motd: '',
        scrollback: 10000,
        tmux: false
    };
    const terminalEncoder = new TextEncoder();

    const state = {
        username: '',
        terminals: {},       // id -> { term, ws, fitAddon, wrapper, name }
        workspaces: [],      // [{ id, layout, terminalIds[4] }]
        activeWorkspaceId: null,
        activeId: null,
        loadingTerminals: false,
        ctrlActive: false,
        fileBrowserOpen: false,
        fileBrowserSingleDesktopDismissed: false,
        embeddedFileBrowserWidth: null,
        embeddedFileBrowserDrag: null,
        fileBrowserTab: 'files',
        fileSelectionMode: false,
        selectedFilePaths: [],
        showHidden: false,
        fileFilterQuery: '',
        fileSort: { key: 'name', direction: 'asc' },
        previewImagePath: '',
        previewImageName: '',
        previewImageVersion: '',
        previewImageSize: 0,
        previewContentType: '',
        previewTextContent: '',
        previewEditMode: false,
        viewerDraft: null,
        currentPath: '',
        files: [],
        editors: [],         // [{ path, name, content, savedContent, dirty, saving, scrollTop }]
        activeEditorPath: null,
        editorPaneSuspended: false,
        editorSearchVisible: false,
        editorSearchMatches: [],
        editorSearchCurrentIndex: -1,
        showEditorLineNumbers: true,
        editorHeight: 320,
        editorDrag: null,
        terminalScrollbarDrag: null,
        fileDropActive: false,
        maximized: 'terminal', // 'terminal', 'editor', or null
        uiConfig: Object.assign({}, DEFAULT_UI_CONFIG),
        language: DEFAULT_LANGUAGE,
        memoryStatus: {
            processRSSBytes: 0,
            systemUsedBytes: 0,
            totalMemoryBytes: 0,
            available: false
        },
        terminalSettings: Object.assign({}, DEFAULT_TERMINAL_SETTINGS),
        terminalSettingsOpen: false,
        upload: {
            active: false,
            totalFiles: 0,
            totalBytes: 0,
            completedBytes: 0,
            currentFileIndex: 0,
            currentFileName: '',
            currentFileLoaded: 0,
            currentFileSize: 0,
            currentXHR: null,
            cancelRequested: false
        }
    };

    let fitTimer = null;
    let authRedirectActive = false;
    let memoryStatusTimer = null;
    let uploadHideTimer = null;
    let viewerLoadSequence = 0;
    let pptxViewerLibraryPromise = null;
    let pptxViewerInstance = null;
    let pptxViewerResizeObserver = null;
    let pptxViewerResizeFrame = 0;
    let docxViewerLibraryPromise = null;
    let xlsxViewerLibraryPromise = null;
    let editorHighlightFrame = 0;
    let fileDragDepth = 0;
    let workspaceTabDrag = null;
    let workspaceTabDragIgnoreClickId = null;

    const loginTitle = document.querySelector('.login-title');
    const loginSubtitle = document.getElementById('login-subtitle');
    const memoryIndicator = document.getElementById('memory-indicator');
    const memoryIndicatorText = document.getElementById('memory-indicator-text');
    const filePanel = document.getElementById('file-panel');
    const filePanelResizer = document.getElementById('file-panel-resizer');
    const fileOverlay = document.getElementById('file-overlay');
    const filePathLabel = document.getElementById('file-path');
    const fileFilterWrap = document.getElementById('file-filter-wrap');
    const fileFilterInput = document.getElementById('file-filter-input');
    const fileFilterSummary = document.getElementById('file-filter-summary');
    const fileToolbar = document.getElementById('file-toolbar');
    const fileSelectModeBtn = document.getElementById('file-select-mode-btn');
    const fileSelectionBar = document.getElementById('file-selection-bar');
    const fileSelectionSummary = document.getElementById('file-selection-summary');
    const fileSelectionAllBtn = document.getElementById('file-selection-all-btn');
    const fileSelectionCopyBtn = document.getElementById('file-selection-copy-btn');
    const fileSelectionMoveBtn = document.getElementById('file-selection-move-btn');
    const fileSelectionRenameBtn = document.getElementById('file-selection-rename-btn');
    const fileSelectionDownloadBtn = document.getElementById('file-selection-download-btn');
    const fileSelectionDeleteBtn = document.getElementById('file-selection-delete-btn');
    const fileBrowserView = document.getElementById('file-browser-view');
    const fileSortNameBtn = document.getElementById('file-sort-name');
    const fileSortSizeBtn = document.getElementById('file-sort-size');
    const fileSortModifiedBtn = document.getElementById('file-sort-modified');
    const fileSortNameIndicator = document.getElementById('file-sort-name-indicator');
    const fileSortSizeIndicator = document.getElementById('file-sort-size-indicator');
    const fileSortModifiedIndicator = document.getElementById('file-sort-modified-indicator');
    const fileViewerView = document.getElementById('file-viewer-view');
    const fileViewerEmpty = document.getElementById('viewer-empty');
    const fileViewerCanvas = document.getElementById('viewer-canvas');
    const fileViewerPreviewImage = document.getElementById('image-preview-blur');
    const fileViewerImage = document.getElementById('image-preview-img');
    const fileViewerImageStage = document.getElementById('viewer-image-stage');
    const fileViewerPdfFrame = document.getElementById('viewer-pdf-frame');
    const fileViewerPptxStage = document.getElementById('viewer-pptx-stage');
    const fileViewerPptxHost = document.getElementById('viewer-pptx-host');
    const fileViewerPptxStatus = document.getElementById('viewer-pptx-status');
    const fileViewerDocxStage = document.getElementById('viewer-docx-stage');
    const fileViewerDocxHost = document.getElementById('viewer-docx-host');
    const fileViewerDocxStatus = document.getElementById('viewer-docx-status');
    const fileViewerXlsxStage = document.getElementById('viewer-xlsx-stage');
    const fileViewerXlsxHost = document.getElementById('viewer-xlsx-host');
    const fileViewerXlsxStatus = document.getElementById('viewer-xlsx-status');
    const fileViewerTextContent = document.getElementById('viewer-text-content');
    const fileViewerEditorHost = document.getElementById('viewer-editor-host');
    const fileViewerLoadingIndicator = document.getElementById('viewer-loading-indicator');
    const fileViewerTitle = document.getElementById('image-preview-title');
    const fileViewerPath = document.getElementById('viewer-file-path');
    const fileViewerCopyBtn = document.getElementById('viewer-copy-btn');
    const fileViewerEditBtn = document.getElementById('viewer-edit-btn');
    const fileViewerDownloadBtn = document.getElementById('viewer-download-btn');
    const fileViewerNewBtn = document.getElementById('viewer-new-btn');
    const fileViewerDraft = document.getElementById('viewer-draft');
    const fileViewerDraftFilename = document.getElementById('viewer-draft-filename');
    const fileViewerDraftHint = document.getElementById('viewer-draft-hint');
    const fileViewerDraftTextarea = document.getElementById('viewer-draft-textarea');
    const fileViewerDraftImage = document.getElementById('viewer-draft-image');
    const fileViewerDraftImagePreview = document.getElementById('viewer-draft-image-preview');
    const fileViewerDraftImageSize = document.getElementById('viewer-draft-image-size');
    const fileViewerDraftStatus = document.getElementById('viewer-draft-status');
    const fileViewerDraftSaveBtn = document.getElementById('viewer-draft-save-btn');
    const fileDropzone = document.getElementById('file-dropzone');
    const editorPane = document.getElementById('editor-pane');
    const editorTabs = document.getElementById('editor-tab-bar');
    const editorPath = document.getElementById('editor-path');
    const editorCaret = document.getElementById('editor-caret');
    const editorStatus = document.getElementById('editor-status');
    const PPTX_VIEWER_JSZIP_URL = 'https://cdn.jsdelivr.net/npm/jszip@3.10.1/dist/jszip.min.js';
    const PPTX_VIEWER_CHART_URL = 'https://cdn.jsdelivr.net/npm/chart.js@4.4.6/dist/chart.umd.min.js';
    const PPTX_VIEWER_SCRIPT_URL = 'https://cdn.jsdelivr.net/npm/pptxviewjs/dist/PptxViewJS.min.js';
    const DOCX_VIEWER_SCRIPT_URL = 'https://cdn.jsdelivr.net/npm/docx-preview/dist/docx-preview.min.js';
    const XLSX_VIEWER_SCRIPT_URL = 'https://cdn.sheetjs.com/xlsx-0.20.3/package/dist/xlsx.full.min.js';
    const editorSaveBtn = document.getElementById('editor-save-btn');
    const editorLineNumbersBtn = document.getElementById('editor-line-numbers-btn');
    const editorFindBtn = document.getElementById('editor-find-btn');
    const editorGotoBtn = document.getElementById('editor-goto-btn');
    const editorSearchBar = document.getElementById('editor-search-bar');
    const editorFindInput = document.getElementById('editor-find-input');
    const editorReplaceInput = document.getElementById('editor-replace-input');
    const editorGoToLineInput = document.getElementById('editor-goto-line-input');
    const editorSearchCount = document.getElementById('editor-search-count');
    const editorLineNumbers = document.getElementById('editor-line-numbers');
    const editorHighlight = document.getElementById('editor-highlight');
    const editorTextarea = document.getElementById('editor-textarea');
    const splitter = document.getElementById('workspace-splitter');
    const workspace = document.getElementById('workspace');
    const terminalContainer = document.getElementById('terminal-container');
    const terminalDock = document.getElementById('terminal-dock');
    const terminalGrid = document.getElementById('terminal-grid');
    const terminalSettingsModal = document.getElementById('terminal-settings');
    const terminalSettingsPreview = document.getElementById('terminal-settings-preview');
    const terminalSettingsPreviewBody = document.getElementById('terminal-settings-preview-body');
    const terminalSettingsPreviewCursor = document.getElementById('terminal-settings-preview-cursor');
    const loginLanguageSelect = document.getElementById('login-language');
    const interfaceLanguageSelect = document.getElementById('interface-language');
    const terminalFontSizeInput = document.getElementById('terminal-font-size');
    const terminalFontSizeValue = document.getElementById('terminal-font-size-value');

    if (filePanel && terminalContainer && filePanel.parentNode !== terminalContainer) {
        terminalContainer.appendChild(filePanel);
    }

    window.addEventListener('load', init);
    window.addEventListener('resize', handleWindowResize);
    if (window.visualViewport) {
        window.visualViewport.addEventListener('resize', handleWindowResize);
        window.visualViewport.addEventListener('scroll', handleWindowResize);
    }
    window.addEventListener('pointermove', handleWorkspaceTabDrag);
    window.addEventListener('pointermove', handleEditorDrag);
    window.addEventListener('pointerup', endEditorDrag);
    window.addEventListener('pointercancel', endEditorDrag);
    window.addEventListener('pointermove', handleEmbeddedFileBrowserDrag);
    window.addEventListener('pointerup', endEmbeddedFileBrowserDrag);
    window.addEventListener('pointercancel', endEmbeddedFileBrowserDrag);
    window.addEventListener('pointerup', endWorkspaceTabDrag);
    window.addEventListener('pointercancel', endWorkspaceTabDrag);
    window.addEventListener('resize', updateWorkspaceTabScrollButtons);
    window.addEventListener('pointermove', handleTerminalScrollbarDrag);
    window.addEventListener('pointerup', endTerminalScrollbarDrag);
    window.addEventListener('pointercancel', endTerminalScrollbarDrag);

    function syncViewportMetrics() {
        var viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0;
        var viewportOffsetTop = 0;
        var visualViewport = window.visualViewport;

        if (visualViewport) {
            if (typeof visualViewport.height === 'number' && visualViewport.height > 0) {
                viewportHeight = visualViewport.height;
            }
            if (typeof visualViewport.offsetTop === 'number' && visualViewport.offsetTop > 0) {
                viewportOffsetTop = visualViewport.offsetTop;
            }
        }

        document.documentElement.style.setProperty('--app-height', Math.round(viewportHeight) + 'px');
        document.documentElement.style.setProperty('--viewport-offset-top', Math.round(viewportOffsetTop) + 'px');
    }

    function resolveLanguage(value) {
        var candidate = (value || '').trim();
        var normalized;

        if (!candidate) {
            return DEFAULT_LANGUAGE;
        }
        if (TRANSLATIONS[candidate]) {
            return candidate;
        }

        normalized = candidate.toLowerCase();
        if (normalized.indexOf('zh') === 0) {
            return 'zh-CN';
        }
        if (normalized.indexOf('ja') === 0) {
            return 'ja';
        }
        if (normalized.indexOf('en') === 0) {
            return 'en';
        }
        return DEFAULT_LANGUAGE;
    }

    function getTranslations() {
        return TRANSLATIONS[state.language] || TRANSLATIONS[DEFAULT_LANGUAGE];
    }

    function t(key, vars) {
        var dict = getTranslations();
        var template = dict[key] || TRANSLATIONS[DEFAULT_LANGUAGE][key] || key;

        if (!vars) {
            return template;
        }

        return template.replace(/\{(\w+)\}/g, function(_, name) {
            return Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : '';
        });
    }

    function applyStaticTranslations() {
        document.documentElement.lang = state.language;

        document.querySelectorAll('[data-i18n]').forEach(function(node) {
            node.textContent = t(node.dataset.i18n);
        });

        document.querySelectorAll('[data-i18n-placeholder]').forEach(function(node) {
            node.placeholder = t(node.dataset.i18nPlaceholder);
        });

        document.querySelectorAll('[data-i18n-title]').forEach(function(node) {
            node.title = t(node.dataset.i18nTitle);
        });

        document.querySelectorAll('[data-i18n-aria-label]').forEach(function(node) {
            node.setAttribute('aria-label', t(node.dataset.i18nAriaLabel));
        });
    }

    function syncLanguageControl() {
        if (loginLanguageSelect) {
            loginLanguageSelect.value = resolveLanguage(state.language);
        }
        if (interfaceLanguageSelect) {
            interfaceLanguageSelect.value = resolveLanguage(state.language);
        }
    }

    function saveLanguageSetting() {
        try {
            window.localStorage.setItem(LANGUAGE_STORAGE_KEY, state.language);
        } catch (_) {
            // Ignore storage failures and continue with in-memory settings.
        }
    }

    function loadLanguageSetting() {
        var stored = null;

        try {
            stored = window.localStorage.getItem(LANGUAGE_STORAGE_KEY);
        } catch (_) {
            stored = null;
        }

        state.language = resolveLanguage(stored || navigator.language || navigator.userLanguage || DEFAULT_LANGUAGE);
    }

    function getDefaultTerminalName(number) {
        return t('terminal.defaultName', { number: number });
    }

    function extractDefaultTerminalNumber(name) {
        var prefixes = Object.keys(DEFAULT_TERMINAL_NAME_PREFIXES).map(function(language) {
            return DEFAULT_TERMINAL_NAME_PREFIXES[language];
        });
        var matched = 0;

        prefixes.forEach(function(prefix) {
            var suffix;
            var number;

            if (matched || typeof name !== 'string' || name.indexOf(prefix) !== 0) {
                return;
            }

            suffix = name.slice(prefix.length).trim();
            number = parseInt(suffix, 10);
            if (!Number.isNaN(number)) {
                matched = number;
            }
        });

        return matched;
    }

    function normalizeWorkspaceLayout(value) {
        var key = String(value || '1');
        return Object.prototype.hasOwnProperty.call(WORKSPACE_LAYOUT_SLOT_COUNTS, key) ? key : '1';
    }

    function getWorkspaceSlotCount(layout) {
        return WORKSPACE_LAYOUT_SLOT_COUNTS[normalizeWorkspaceLayout(layout)];
    }

    function generateWorkspaceId() {
        return 'view-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8);
    }

    function normalizeWorkspaceLabelNumber(value) {
        var labelNumber = Number(value);

        if (Number.isInteger(labelNumber) && labelNumber > 0) {
            return labelNumber;
        }

        return 0;
    }

    function getNextWorkspaceLabelNumber(workspaces) {
        var items = Array.isArray(workspaces) ? workspaces : state.workspaces;

        return items.reduce(function(max, workspace) {
            return Math.max(max, normalizeWorkspaceLabelNumber(workspace && workspace.labelNumber));
        }, 0) + 1;
    }

    function createWorkspace(layout, terminalIds, name, labelNumber) {
        var slots = ['', '', '', ''];

        if (Array.isArray(terminalIds)) {
            terminalIds.slice(0, 4).forEach(function(id, index) {
                slots[index] = typeof id === 'string' ? id : '';
            });
        }

        return {
            id: generateWorkspaceId(),
            layout: normalizeWorkspaceLayout(layout),
            terminalIds: slots,
            name: typeof name === 'string' ? name.trim() : '',
            labelNumber: normalizeWorkspaceLabelNumber(labelNumber) || getNextWorkspaceLabelNumber()
        };
    }

    function normalizeWorkspaceRecord(candidate, index) {
        var workspace = createWorkspace(
            candidate && candidate.layout,
            candidate && candidate.terminalIds,
            candidate && candidate.name,
            normalizeWorkspaceLabelNumber(candidate && candidate.labelNumber) || (index + 1)
        );

        if (candidate && typeof candidate.id === 'string' && candidate.id.trim()) {
            workspace.id = candidate.id.trim();
        }

        return workspace;
    }

    function getWorkspaceById(id) {
        return state.workspaces.find(function(workspace) {
            return workspace.id === id;
        }) || null;
    }

    function getActiveWorkspace() {
        return getWorkspaceById(state.activeWorkspaceId);
    }

    function getDefaultWorkspaceName(number) {
        return t('workspace.defaultName', { number: number });
    }

    function getWorkspaceLabel(workspace, index) {
        if (workspace && workspace.name) {
            return workspace.name;
        }

        return getDefaultWorkspaceName(normalizeWorkspaceLabelNumber(workspace && workspace.labelNumber) || (index + 1));
    }

    function normalizeWorkspaceStatePayload(payload) {
        var workspaces = [];
        var activeWorkspaceId = null;
        var updatedAt = '';

        if (Array.isArray(payload)) {
            workspaces = payload;
        } else if (payload && typeof payload === 'object') {
            workspaces = Array.isArray(payload.workspaces) ? payload.workspaces : [];
            activeWorkspaceId = typeof payload.activeWorkspaceId === 'string' ? payload.activeWorkspaceId : null;
            updatedAt = typeof payload.updatedAt === 'string' ? payload.updatedAt : '';
        }

        return {
            activeWorkspaceId: activeWorkspaceId,
            updatedAt: updatedAt,
            workspaces: workspaces.map(function(workspace, index) {
                return normalizeWorkspaceRecord(workspace, index);
            })
        };
    }

    function applyWorkspaceStatePayload(payload) {
        var normalized = normalizeWorkspaceStatePayload(payload);

        state.workspaces = normalized.workspaces;
        state.activeWorkspaceId = normalized.activeWorkspaceId;
    }

    function readLocalWorkspaceStatePayload() {
        var stored = null;
        var payload = null;

        try {
            stored = window.localStorage.getItem(TERMINAL_WORKSPACES_STORAGE_KEY);
        } catch (_) {
            stored = null;
        }

        if (!stored) {
            return null;
        }

        try {
            payload = JSON.parse(stored);
        } catch (_) {
            payload = null;
        }

        return normalizeWorkspaceStatePayload(payload);
    }

    function loadWorkspaceState() {
        var payload = readLocalWorkspaceStatePayload();

        if (!payload) {
            state.workspaces = [];
            state.activeWorkspaceId = null;
            return;
        }

        applyWorkspaceStatePayload(payload);
    }

    function readEditorDraftPayload() {
        let stored = null;
        let payload = null;

        try {
            stored = window.localStorage.getItem(EDITOR_DRAFTS_STORAGE_KEY);
        } catch (_) {
            stored = null;
        }

        if (!stored) {
            return null;
        }

        try {
            payload = JSON.parse(stored);
        } catch (_) {
            payload = null;
        }

        return payload && typeof payload === 'object' ? payload : null;
    }

    function loadEditorUIPreferences() {
        let stored = null;
        let payload = null;

        try {
            stored = window.localStorage.getItem(EDITOR_UI_STORAGE_KEY);
        } catch (_) {
            stored = null;
        }

        if (!stored) {
            state.showEditorLineNumbers = true;
            state.embeddedFileBrowserWidth = null;
            return;
        }

        try {
            payload = JSON.parse(stored);
        } catch (_) {
            payload = null;
        }

        state.showEditorLineNumbers = !payload || payload.showLineNumbers !== false;
        state.embeddedFileBrowserWidth = payload && Number.isFinite(payload.embeddedFileBrowserWidth) && payload.embeddedFileBrowserWidth > 0
            ? Number(payload.embeddedFileBrowserWidth)
            : null;
    }

    function persistEditorUIPreferences() {
        try {
            window.localStorage.setItem(EDITOR_UI_STORAGE_KEY, JSON.stringify({
                showLineNumbers: state.showEditorLineNumbers,
                embeddedFileBrowserWidth: state.embeddedFileBrowserWidth
            }));
        } catch (_) {
            // Ignore storage failures.
        }
    }

    function clearEditorDraftStorage() {
        try {
            window.localStorage.removeItem(EDITOR_DRAFTS_STORAGE_KEY);
        } catch (_) {
            // Ignore storage failures.
        }
    }

    function hasDirtyEditors() {
        return state.editors.some(function(editor) {
            return Boolean(editor && editor.dirty);
        });
    }

    function persistEditorDrafts() {
        const drafts = state.editors.filter(function(editor) {
            return editor && editor.dirty;
        }).map(function(editor) {
            return {
                path: editor.path,
                name: editor.name,
                content: editor.content,
                savedContent: editor.savedContent,
                scrollTop: editor.scrollTop || 0,
                scrollLeft: editor.scrollLeft || 0,
                isNew: Boolean(editor.isNew)
            };
        });

        if (!drafts.length) {
            clearEditorDraftStorage();
            return;
        }

        try {
            window.localStorage.setItem(EDITOR_DRAFTS_STORAGE_KEY, JSON.stringify({
                activePath: state.activeEditorPath,
                editors: drafts
            }));
        } catch (_) {
            // Ignore storage failures.
        }
    }

    function buildWorkspaceStatePayload() {
        return {
            activeWorkspaceId: state.activeWorkspaceId,
            updatedAt: new Date().toISOString(),
            workspaces: state.workspaces.map(function(workspace) {
                return {
                    id: workspace.id,
                    layout: normalizeWorkspaceLayout(workspace.layout),
                    terminalIds: workspace.terminalIds.slice(0, 4),
                    name: workspace.name || '',
                    labelNumber: normalizeWorkspaceLabelNumber(workspace.labelNumber) || 1
                };
            })
        };
    }

    function persistWorkspaceStateLocally(payload) {
        if (!payload) {
            return;
        }

        try {
            window.localStorage.setItem(TERMINAL_WORKSPACES_STORAGE_KEY, JSON.stringify(payload));
        } catch (_) {
            // Ignore storage failures and continue with in-memory state.
        }
    }

    function getWorkspaceStateTimestamp(payload) {
        var value = Date.parse(payload && payload.updatedAt ? payload.updatedAt : '');
        return Number.isFinite(value) ? value : 0;
    }

    function shouldPreferServerWorkspaceState(localPayload, serverPayload) {
        if (!serverPayload || !serverPayload.workspaces.length) {
            return false;
        }
        if (!localPayload || !localPayload.workspaces.length) {
            return true;
        }
        return getWorkspaceStateTimestamp(serverPayload) >= getWorkspaceStateTimestamp(localPayload);
    }

    function persistWorkspaceStateToServer(payload) {
        if (!payload || !state.username) {
            return Promise.resolve();
        }

        return fetchJSON('/api/workspace-state', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        }, { authRequired: true })
            .catch(function(err) {
                if (!isHandledAuthError(err) && window.console && typeof window.console.warn === 'function') {
                    window.console.warn('workspace sync failed', err);
                }
                return null;
            });
    }

    function saveWorkspaceState() {
        var payload = buildWorkspaceStatePayload();

        persistWorkspaceStateLocally(payload);
        persistWorkspaceStateToServer(payload);
    }

    function loadWorkspaceStateFromServer() {
        return fetchJSON('/api/workspace-state', undefined, { authRequired: true })
            .then(function(payload) {
                return normalizeWorkspaceStatePayload(payload);
            })
            .catch(function(err) {
                if (isHandledAuthError(err)) {
                    throw err;
                }
                return null;
            });
    }

    function hydrateWorkspaceState() {
        var localPayload = readLocalWorkspaceStatePayload();

        return loadWorkspaceStateFromServer()
            .then(function(serverPayload) {
                var preferred = shouldPreferServerWorkspaceState(localPayload, serverPayload)
                    ? serverPayload
                    : localPayload;

                if (!preferred) {
                    state.workspaces = [];
                    state.activeWorkspaceId = null;
                    return null;
                }

                applyWorkspaceStatePayload(preferred);
                persistWorkspaceStateLocally(preferred);

                if (preferred === localPayload && (!serverPayload || !serverPayload.workspaces.length || getWorkspaceStateTimestamp(localPayload) > getWorkspaceStateTimestamp(serverPayload))) {
                    persistWorkspaceStateToServer(buildWorkspaceStatePayload());
                }

                return preferred;
            });
    }

    function getVisibleTerminalIdsForWorkspace(workspace) {
        var visible = [];
        var seen = {};
        var slotCount;

        if (!workspace) {
            return visible;
        }

        slotCount = getWorkspaceSlotCount(workspace.layout);
        workspace.terminalIds.slice(0, slotCount).forEach(function(id) {
            if (!id || !state.terminals[id] || seen[id]) {
                return;
            }
            seen[id] = true;
            visible.push(id);
        });

        return visible;
    }

    function getVisibleTerminalIds() {
        return getVisibleTerminalIdsForWorkspace(getActiveWorkspace());
    }

    function isTerminalViewVisible() {
        return document.getElementById('terminal-view').style.display !== 'none';
    }

    function isTerminalVisibleInActiveWorkspace(id) {
        return getVisibleTerminalIds().indexOf(id) !== -1;
    }

    function ensureActiveTerminalVisible(preferredId) {
        var visibleIds = getVisibleTerminalIds();

        if (!visibleIds.length) {
            state.activeId = null;
            return;
        }

        if (preferredId && visibleIds.indexOf(preferredId) !== -1) {
            state.activeId = preferredId;
            return;
        }

        if (!state.activeId || visibleIds.indexOf(state.activeId) === -1) {
            state.activeId = visibleIds[0];
        }
    }

    function syncActiveTerminalPane() {
        if (!terminalGrid) {
            return;
        }

        terminalGrid.querySelectorAll('.terminal-pane').forEach(function(node) {
            node.classList.toggle('active', Boolean(state.activeId) && node.dataset.terminalId === state.activeId);
        });
    }

    function detachTerminalSocket(id, terminalEntry) {
        if (!terminalEntry) {
            return;
        }

        terminalEntry.desiredConnected = false;
        terminalEntry.reconnectAttempts = 0;

        if (terminalEntry.ws) {
            terminalEntry.detachRequested = true;
            try {
                terminalEntry.ws.close();
            } catch (_) {
                terminalEntry.ws = null;
                terminalEntry.connecting = false;
                terminalEntry.detachRequested = false;
            }
        } else {
            terminalEntry.connecting = false;
            terminalEntry.detachRequested = false;
        }
    }

    function syncVisibleTerminalConnections() {
        var desired = {};

        if (isTerminalViewVisible()) {
            getVisibleTerminalIds().forEach(function(id) {
                desired[id] = true;
            });
        }

        Object.keys(state.terminals).forEach(function(id) {
            var terminalEntry = state.terminals[id];

            terminalEntry.desiredConnected = Boolean(desired[id]);
            if (terminalEntry.desiredConnected) {
                if (!terminalEntry.ws && !terminalEntry.connecting) {
                    openTerminalSocket(id, terminalEntry, false);
                }
            } else {
                detachTerminalSocket(id, terminalEntry);
            }
        });
    }

    function clearHiddenWorkspaceSlots(workspace) {
        var slotCount;

        if (!workspace) {
            return;
        }

        slotCount = getWorkspaceSlotCount(workspace.layout);
        for (var index = slotCount; index < 4; index += 1) {
            workspace.terminalIds[index] = '';
        }
    }

    function getAssignedTerminalIdsExcludingWorkspace(workspaceId) {
        var assigned = {};

        state.workspaces.forEach(function(workspace) {
            var slotCount;

            if (!workspace || workspace.id === workspaceId) {
                return;
            }

            slotCount = getWorkspaceSlotCount(workspace.layout);
            workspace.terminalIds.slice(0, slotCount).forEach(function(id) {
                if (id && state.terminals[id]) {
                    assigned[id] = true;
                }
            });
        });

        return assigned;
    }

    function clearTerminalAssignment(terminalId, exceptWorkspaceId, exceptSlotIndex) {
        if (!terminalId) {
            return;
        }

        state.workspaces.forEach(function(workspace) {
            if (!workspace) {
                return;
            }

            workspace.terminalIds = workspace.terminalIds.map(function(id, index) {
                if (workspace.id === exceptWorkspaceId && index === exceptSlotIndex) {
                    return id;
                }
                return id === terminalId ? '' : id;
            });
        });
    }

    function autofillWorkspaceSlots(workspace) {
        var slotCount;
        var used = {};
        var assignedElsewhere;
        var available;

        if (!workspace) {
            return;
        }

        clearHiddenWorkspaceSlots(workspace);
        slotCount = getWorkspaceSlotCount(workspace.layout);

        workspace.terminalIds = workspace.terminalIds.map(function(id, index) {
            if (index >= slotCount) {
                return '';
            }
            if (!id || !state.terminals[id] || used[id]) {
                return '';
            }
            used[id] = true;
            return id;
        });

        assignedElsewhere = getAssignedTerminalIdsExcludingWorkspace(workspace.id);
        available = Object.keys(state.terminals).filter(function(id) {
            return !assignedElsewhere[id] && !used[id];
        });

        for (var index = 0; index < slotCount; index += 1) {
            if (!workspace.terminalIds[index] && available.length > 0) {
                workspace.terminalIds[index] = available.shift();
                used[workspace.terminalIds[index]] = true;
            }
        }
    }

    function rebalanceWorkspaceAssignments(preferredWorkspaceId) {
        var ordered = state.workspaces.slice();
        var seen = {};

        if (preferredWorkspaceId) {
            ordered.sort(function(left, right) {
                if (left.id === preferredWorkspaceId) {
                    return -1;
                }
                if (right.id === preferredWorkspaceId) {
                    return 1;
                }
                return 0;
            });
        }

        ordered.forEach(function(workspace) {
            var slotCount = getWorkspaceSlotCount(workspace.layout);
            var localSeen = {};

            clearHiddenWorkspaceSlots(workspace);

            for (var index = 0; index < slotCount; index += 1) {
                var terminalId = workspace.terminalIds[index];

                if (!terminalId || !state.terminals[terminalId] || localSeen[terminalId] || seen[terminalId]) {
                    workspace.terminalIds[index] = '';
                    continue;
                }

                localSeen[terminalId] = true;
                seen[terminalId] = true;
            }
        });

        ordered.forEach(function(workspace) {
            autofillWorkspaceSlots(workspace);
        });
    }

    function reconcileWorkspacesAfterTerminalLoad(preferredTerminalId) {
        var terminalIds = Object.keys(state.terminals);
        var hadStoredWorkspaces = state.workspaces.length > 0;
        var storedTerminalIds = {};
        var storedTerminalCount = 0;
        var hasMissingStoredTerminal = false;

        state.workspaces.forEach(function(workspace) {
            if (!workspace || !Array.isArray(workspace.terminalIds)) {
                return;
            }
            workspace.terminalIds.slice(0, 4).forEach(function(id) {
                if (!id || storedTerminalIds[id]) {
                    return;
                }
                storedTerminalIds[id] = true;
                storedTerminalCount += 1;
                if (!state.terminals[id]) {
                    hasMissingStoredTerminal = true;
                }
            });
        });

        if (hadStoredWorkspaces && terminalIds.length > 0 && storedTerminalCount > terminalIds.length && hasMissingStoredTerminal) {
            if (!getWorkspaceById(state.activeWorkspaceId) && state.workspaces.length) {
                state.activeWorkspaceId = state.workspaces[0].id;
            }
            ensureActiveTerminalVisible(preferredTerminalId);
            return;
        }

        state.workspaces = state.workspaces.map(function(workspace) {
            var next = normalizeWorkspaceRecord(workspace);

            next.terminalIds = next.terminalIds.map(function(id) {
                return id && state.terminals[id] ? id : '';
            });
            autofillWorkspaceSlots(next);
            return next;
        });

        if (!state.workspaces.length) {
            if (terminalIds.length === 0) {
                state.workspaces = [createWorkspace('1', [''], '', 1)];
            } else {
                state.workspaces = terminalIds.map(function(id, index) {
                    return createWorkspace('1', [id], '', index + 1);
                });
                state.activeWorkspaceId = state.workspaces[state.workspaces.length - 1].id;
            }
        }

        if (!getWorkspaceById(state.activeWorkspaceId)) {
            state.activeWorkspaceId = state.workspaces[0].id;
        }

        rebalanceWorkspaceAssignments(state.activeWorkspaceId);

        if (!hadStoredWorkspaces && preferredTerminalId) {
            state.activeWorkspaceId = state.workspaces[state.workspaces.length - 1].id;
            state.activeId = preferredTerminalId;
        }

        ensureActiveTerminalVisible(preferredTerminalId);
        saveWorkspaceState();
    }

    function updateLocalizedRuntimeText() {
        applyUIConfig();
        renderMemoryStatus();
        syncLanguageControl();
        syncTerminalSettingsForm();
        syncTerminalSettingsPreview();
        renderTabBar();
        renderActiveWorkspace();
        renderEditorTabs();
        updateEditorChrome();
        applyEditorViewOptions();
        syncEditorSearchBar();
        syncFileBrowserLayout();
        updateHiddenToggle();
        updateFileSortControls();

        if (state.currentPath || state.files.length) {
            renderFileList(state.files);
        }

        if (state.upload.active) {
            updateUploadProgress();
        } else {
            document.getElementById('file-upload-label').textContent = t('upload.uploading');
            document.getElementById('file-upload-detail').textContent = '';
            document.getElementById('file-upload-percent').textContent = '0%';
        }
    }

    function applyLocalization() {
        applyStaticTranslations();
        updateLocalizedRuntimeText();
    }

    function setLanguage(nextLanguage) {
        var resolved = resolveLanguage(nextLanguage);

        if (state.language === resolved) {
            syncLanguageControl();
            return;
        }

        state.language = resolved;
        saveLanguageSetting();
        applyLocalization();
    }

    function formatMemoryCompact(bytes) {
        var units = ['B', 'K', 'M', 'G', 'T'];
        var value = Number(bytes) || 0;
        var unitIndex = 0;
        var decimals = 0;

        while (value >= 1024 && unitIndex < units.length - 1) {
            value /= 1024;
            unitIndex += 1;
        }

        if (unitIndex === 0 || value >= 100) {
            decimals = 0;
        } else {
            decimals = 1;
        }

        return value.toFixed(decimals).replace(/\.0$/, '') + units[unitIndex];
    }

    function handleBeforeUnload(event) {
        if (!hasDirtyEditors()) {
            return undefined;
        }

        persistEditorDrafts();
        event.preventDefault();
        event.returnValue = '';
        return '';
    }

    function renderMemoryStatus() {
        var status = state.memoryStatus;
        var appText;
        var usedText;
        var totalText;

        if (!memoryIndicator || !memoryIndicatorText) {
            return;
        }

        if (!status.available) {
            memoryIndicator.dataset.state = 'unavailable';
            memoryIndicatorText.textContent = '-- / -- / --';
            memoryIndicator.title = t('memory.unavailableTooltip');
            return;
        }

        appText = formatMemoryCompact(status.processRSSBytes);
        usedText = formatMemoryCompact(status.systemUsedBytes);
        totalText = formatMemoryCompact(status.totalMemoryBytes);

        memoryIndicator.dataset.state = 'ready';
        memoryIndicatorText.textContent = appText + ' / ' + usedText + ' / ' + totalText;
        memoryIndicator.title = t('memory.tooltip', { app: appText, used: usedText, total: totalText });
    }

    function resetMemoryStatus() {
        state.memoryStatus.processRSSBytes = 0;
        state.memoryStatus.systemUsedBytes = 0;
        state.memoryStatus.totalMemoryBytes = 0;
        state.memoryStatus.available = false;
        renderMemoryStatus();
    }

    function stopMemoryStatusPolling() {
        if (memoryStatusTimer) {
            clearInterval(memoryStatusTimer);
            memoryStatusTimer = null;
        }
    }

    function refreshMemoryStatus() {
        return fetchJSON('/api/system/memory', undefined, { authRequired: true })
            .then(function(data) {
                state.memoryStatus.processRSSBytes = Number(data.processRSSBytes) || 0;
                state.memoryStatus.systemUsedBytes = Number(data.systemUsedBytes) || 0;
                state.memoryStatus.totalMemoryBytes = Number(data.totalMemoryBytes) || 0;
                state.memoryStatus.available = state.memoryStatus.processRSSBytes > 0 && state.memoryStatus.systemUsedBytes > 0 && state.memoryStatus.totalMemoryBytes > 0;
                renderMemoryStatus();
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    resetMemoryStatus();
                }
            });
    }

    function startMemoryStatusPolling() {
        stopMemoryStatusPolling();
        refreshMemoryStatus();
        memoryStatusTimer = setInterval(refreshMemoryStatus, 10000);
    }

    function setLoginPasswordVisible(visible) {
        var passwordInput = document.getElementById('login-password');
        var toggle = document.getElementById('login-show-password');

        if (!passwordInput || !toggle) {
            return;
        }

        toggle.checked = Boolean(visible);
        passwordInput.type = toggle.checked ? 'text' : 'password';
    }

    function init() {
        syncViewportMetrics();
        syncBodyViewState('login');
        loadLanguageSetting();
        loadTerminalSettings();
        loadWorkspaceState();
        loadEditorUIPreferences();
        applyEditorViewOptions();
        applyLocalization();
        bindTerminalSettingsControls();
        loadUIConfig().finally(checkAuth);

        document.getElementById('login-password').addEventListener('keydown', function(e) {
            if (e.key === 'Enter') {
                window.doLogin();
            }
        });

        document.getElementById('login-show-password').addEventListener('change', function(e) {
            setLoginPasswordVisible(e.target.checked);
        });
        setLoginPasswordVisible(false);

        if (loginLanguageSelect) {
            loginLanguageSelect.addEventListener('change', function(event) {
                setLanguage(event.target.value);
            });
        }

        document.getElementById('login-username').addEventListener('keydown', function(e) {
            if (e.key === 'Enter') {
                document.getElementById('login-password').focus();
            }
        });

        document.getElementById('file-upload-input').addEventListener('change', handleFileUploadSelection);
        document.addEventListener('keydown', handleGlobalKeyDown);
        document.addEventListener('copy', handleDocumentCopy);
        document.addEventListener('paste', handleDocumentPaste);
        fileFilterInput.addEventListener('input', handleFileFilterInput);
        fileFilterInput.addEventListener('keydown', handleFileFilterKeyDown);
        editorTextarea.addEventListener('input', handleEditorInput);
        editorTextarea.addEventListener('scroll', rememberActiveEditorScroll);
        editorTextarea.addEventListener('keydown', handleEditorTextareaKeyDown);
        editorTextarea.addEventListener('click', handleEditorSelectionChange);
        editorTextarea.addEventListener('keyup', handleEditorSelectionChange);
        editorTextarea.addEventListener('select', handleEditorSelectionChange);
        editorFindInput.addEventListener('input', handleEditorSearchInput);
        editorFindInput.addEventListener('keydown', handleEditorFindKeyDown);
        editorReplaceInput.addEventListener('keydown', handleEditorReplaceKeyDown);
        editorGoToLineInput.addEventListener('keydown', handleEditorGoToLineKeyDown);
        splitter.addEventListener('pointerdown', beginEditorDrag);
        if (filePanelResizer) {
            filePanelResizer.addEventListener('pointerdown', beginEmbeddedFileBrowserDrag);
        }
        window.addEventListener('beforeunload', handleBeforeUnload);
        filePanel.addEventListener('dragenter', handleFilePanelDragEnter);
        filePanel.addEventListener('dragover', handleFilePanelDragOver);
        filePanel.addEventListener('dragleave', handleFilePanelDragLeave);
        filePanel.addEventListener('drop', handleFilePanelDrop);
    }

    function handleWindowResize() {
        syncViewportMetrics();
        applyEditorLayout();
        syncWorkspaceFileBrowserDefaults('resize');
        scheduleFitActiveTerminal();
    }

    window.toggleMaximize = function(pane) {
        if (state.maximized === pane) {
            state.maximized = null;
        } else {
            state.maximized = pane;
        }
        applyMaximizeState();
        scheduleFitActiveTerminal();
    };

    function applyMaximizeState() {
        workspace.classList.remove('editor-maximized', 'terminal-maximized');
        var editorBtn = document.getElementById('editor-max-btn');
        var terminalBtn = document.getElementById('terminal-max-btn');

        if (editorBtn) editorBtn.classList.remove('maximize-active');
        if (terminalBtn) terminalBtn.classList.remove('maximize-active');

        if (state.maximized === 'editor' && state.editors.length > 0) {
            workspace.classList.add('editor-maximized');
            if (editorBtn) editorBtn.classList.add('maximize-active');
        } else if (state.maximized === 'terminal') {
            workspace.classList.add('terminal-maximized');
            if (terminalBtn) terminalBtn.classList.add('maximize-active');
        }
    }

    function isEditorShortcutTarget(node) {
        return Boolean(state.activeEditorPath && node && node.closest && node.closest('#editor-pane'));
    }

    function handleGlobalKeyDown(e) {
        if (handleGlobalTerminalClipboardShortcut(e)) {
            return;
        }

        if (e.key === 'Escape' && state.editorSearchVisible && isEditorShortcutTarget(document.activeElement)) {
            e.preventDefault();
            window.closeEditorSearch();
            return;
        }

        if ((e.ctrlKey || e.metaKey) && !e.altKey && isEditorShortcutTarget(document.activeElement)) {
            if (e.key.toLowerCase() === 'f') {
                e.preventDefault();
                window.openEditorFind();
                return;
            }
            if (e.key.toLowerCase() === 'h') {
                e.preventDefault();
                window.openEditorReplace();
                return;
            }
            if (e.key.toLowerCase() === 'g') {
                e.preventDefault();
                window.openEditorGoToLine();
                return;
            }
        }

        if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === 's' && state.activeEditorPath) {
            e.preventDefault();
            window.saveActiveEditorAs();
            return;
        }

        if (e.key === 'Escape' && state.terminalSettingsOpen) {
            window.closeTerminalSettings();
            return;
        }

        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && state.activeEditorPath) {
            e.preventDefault();
            window.saveActiveEditor();
            return;
        }

        if (shouldHandleViewerImageNavigationKey(e)) {
            e.preventDefault();
            navigateViewerImageByOffset(e.key === 'ArrowLeft' ? -1 : 1);
            return;
        }

        if (e.key === 'Escape' && state.fileBrowserOpen && state.fileBrowserTab === 'viewer' && state.previewImagePath) {
            if (shouldEmbedViewerEditor()) {
                closeEditor(state.previewImagePath);
            } else {
                window.closeImagePreview();
            }
            return;
        }

        if (e.key === 'Escape' && state.fileBrowserOpen && state.fileBrowserTab === 'files' && state.fileSelectionMode) {
            window.clearFileSelectionMode();
            return;
        }

        if (e.key === 'Escape' && state.fileBrowserOpen) {
            setFileBrowserOpen(false, null, { userInitiated: true });
            return;
        }

        if (shouldRouteTabToTerminal(e)) {
            e.preventDefault();
            sendActiveTerminalSequence(e.shiftKey ? '\x1b[Z' : '\t');
        }
    }

    function isKeyboardTextInput(node) {
        if (!node || !node.tagName) {
            return false;
        }

        const tagName = node.tagName.toLowerCase();
        return (
            node === editorTextarea ||
            node.isContentEditable ||
            tagName === 'input' ||
            tagName === 'textarea' ||
            tagName === 'select'
        );
    }

    function shouldHandleViewerImageNavigationKey(event) {
        if (!event || event.defaultPrevented) {
            return false;
        }
        if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) {
            return false;
        }
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') {
            return false;
        }
        if (!state.fileBrowserOpen || state.fileBrowserTab !== 'viewer' || state.previewContentType !== 'image' || !state.previewImagePath) {
            return false;
        }

        return !isKeyboardTextInput(document.activeElement);
    }

    function shouldKeepTabForUI(node) {
        if (isKeyboardTextInput(node)) {
            return true;
        }
        if (!node || !node.closest) {
            return false;
        }
        if (state.terminalSettingsOpen && node.closest('#terminal-settings')) {
            return true;
        }
        if (document.getElementById('login-view').style.display !== 'none' && node.closest('#login-view')) {
            return true;
        }
        return false;
    }

    function sendTerminalSequence(terminal, sequence) {
        if (!terminal || !terminal.ws || terminal.ws.readyState !== WebSocket.OPEN) {
            return false;
        }
        clearTerminalScrollTarget(terminal);
        resetTerminalScrollMode(terminal);
        terminal.ws.send(terminalEncoder.encode(sequence));
        return true;
    }

    function sendTerminalControl(terminal, payload) {
        if (!terminal || !terminal.ws || terminal.ws.readyState !== WebSocket.OPEN) {
            return false;
        }
        terminal.ws.send(JSON.stringify(payload));
        return true;
    }

    function requestTerminalScrollState(terminal) {
        if (!terminal || !state.uiConfig.tmux) {
            return false;
        }
        return sendTerminalControl(terminal, { type: 'scroll_state' });
    }

    function updateTerminalScrollState(terminal, payload) {
        if (!terminal || !payload) {
            return;
        }

        terminal.scrollStateAwaiting = false;
        terminal.tmuxScrollState = {
            inMode: !!payload.inMode,
            historySize: Math.max(0, Number(payload.historySize) || 0),
            scrollPosition: Math.max(0, Number(payload.scrollPosition) || 0),
            paneHeight: Math.max(1, Number(payload.paneHeight) || terminal.term.rows || 1)
        };
        terminal.scrollModeActive = terminal.tmuxScrollState.inMode;
        scheduleTerminalScrollbarSync(terminal);
    }

    function resetTerminalScrollMode(terminal) {
        if (!terminal || !terminal.scrollModeActive) {
            return;
        }
        if (terminal.scrollFlushTimer) {
            clearTimeout(terminal.scrollFlushTimer);
            terminal.scrollFlushTimer = null;
        }
        terminal.pendingScrollLines = 0;
        if (sendTerminalControl(terminal, { type: 'scroll_reset' })) {
            terminal.scrollModeActive = false;
            terminal.scrollStateAwaiting = true;
        }
    }

    function queueTerminalScrollLines(terminal, lineDelta) {
        var nextLines;

        if (!terminal || !state.uiConfig.tmux || !terminal.ws || terminal.ws.readyState !== WebSocket.OPEN || !lineDelta) {
            return false;
        }

        nextLines = Math.round(lineDelta);
        if (!nextLines) {
            return false;
        }

        terminal.pendingScrollLines = (terminal.pendingScrollLines || 0) + nextLines;

        if (terminal.scrollFlushTimer) {
            return true;
        }

        terminal.scrollFlushTimer = setTimeout(function() {
            var lines = terminal.pendingScrollLines || 0;

            terminal.scrollFlushTimer = null;
            terminal.pendingScrollLines = 0;
            if (!lines) {
                return;
            }
            if (sendTerminalControl(terminal, { type: 'scroll', lines: lines })) {
                terminal.scrollStateAwaiting = true;
                if (lines < 0 || terminal.scrollModeActive) {
                    terminal.scrollModeActive = true;
                }
            }
        }, 16);

        return true;
    }

    function queueTerminalScroll(terminal, deltaY) {
        var stepCount;
        var lineDelta;

        if (!deltaY) {
            return false;
        }

        stepCount = Math.max(1, Math.round(Math.abs(deltaY) / 60));
        lineDelta = stepCount * 3 * (deltaY < 0 ? -1 : 1);
        return queueTerminalScrollLines(terminal, lineDelta);
    }

    function sendActiveTerminalSequence(sequence) {
        const terminal = getActiveTerminal();
        if (!sendTerminalSequence(terminal, sequence)) {
            return false;
        }
        if (terminal.term) {
            terminal.term.focus();
        }
        return true;
    }

    function getActiveTerminal() {
        ensureActiveTerminalVisible();
        return state.activeId ? state.terminals[state.activeId] : null;
    }

    function getTermClipboardCopyText(term) {
        if (!term || typeof term.getSelection !== 'function') {
            return '';
        }
        return term.getSelection();
    }

    function getPageSelectionText() {
        if (!window.getSelection) {
            return '';
        }
        var selection = window.getSelection();
        return selection && selection.toString ? selection.toString() : '';
    }

    function getTerminalSelectionText(terminal) {
        if (!terminal || !terminal.term) {
            return getPageSelectionText();
        }
        return getTermClipboardCopyText(terminal.term) || getPageSelectionText();
    }

    function writeClipboardText(text) {
        if (!text) {
            return false;
        }
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).catch(function() {
                fallbackCopyText(text);
            });
            return true;
        }
        fallbackCopyText(text);
        return true;
    }

    function copyTerminalSelection(term, allowPageSelection) {
        var selection = getTermClipboardCopyText(term);
        if (!selection && allowPageSelection !== false) {
            selection = getPageSelectionText();
        }
        if (!writeClipboardText(selection)) {
            return false;
        }
        if (term && typeof term.clearSelection === 'function') {
            term.clearSelection();
        }
        return true;
    }

    function fallbackCopyText(text) {
        var textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.cssText = 'position:fixed;left:-9999px;top:-9999px;opacity:0';
        document.body.appendChild(textarea);
        textarea.focus();
        textarea.select();
        try { document.execCommand('copy'); } catch (_) {}
        document.body.removeChild(textarea);
    }

    function pasteToTerminal(terminal) {
        if (!terminal || !terminal.ws || terminal.ws.readyState !== WebSocket.OPEN) {
            return;
        }
        if (navigator.clipboard && navigator.clipboard.readText) {
            navigator.clipboard.readText()
                .then(function(text) {
                    if (text) {
                        sendTerminalSequence(terminal, text);
                    }
                })
                .catch(function() {
                    // Clipboard API not available or denied
                });
        }
    }

    function shouldHandleTerminalClipboardEvent() {
        const terminal = getActiveTerminal();
        if (!terminal || !terminal.term || !terminal.ws || terminal.ws.readyState !== WebSocket.OPEN) {
            return false;
        }
        if (document.getElementById('terminal-view').style.display === 'none') {
            return false;
        }
        if (shouldKeepTabForUI(document.activeElement)) {
            return false;
        }
        return true;
    }

    function handleDocumentCopy(event) {
        const terminal = getActiveTerminal();
        const selection = getTerminalSelectionText(terminal);

        if (!shouldHandleTerminalClipboardEvent() || !selection || !event.clipboardData) {
            return;
        }

        event.preventDefault();
        event.clipboardData.setData('text/plain', selection);
        if (terminal.term && typeof terminal.term.clearSelection === 'function') {
            terminal.term.clearSelection();
        }
    }

    function handleDocumentPaste(event) {
        const terminal = getActiveTerminal();
        let text;

        if (shouldHandleViewerEmptyClipboardPaste(event)) {
            if (openViewerDraftFromClipboard(event.clipboardData)) {
                event.preventDefault();
                return;
            }
        }

        if (!shouldHandleTerminalClipboardEvent() || !event.clipboardData) {
            return;
        }

        text = event.clipboardData.getData('text/plain');
        if (!text) {
            return;
        }

        event.preventDefault();
        sendTerminalSequence(terminal, text);
    }

    function handleGlobalTerminalClipboardShortcut(event) {
        const terminal = getActiveTerminal();
        if (!terminal || !terminal.term || !terminal.ws || terminal.ws.readyState !== WebSocket.OPEN) {
            return false;
        }

        if (document.getElementById('terminal-view').style.display === 'none') {
            return false;
        }

        if (!(event.ctrlKey || event.metaKey) || event.altKey) {
            return false;
        }

        if (shouldKeepTabForUI(document.activeElement)) {
            return false;
        }

        const key = (event.key || '').toLowerCase();
        const hasSelection = typeof terminal.term.hasSelection === 'function' && terminal.term.hasSelection();
        const pageHasSelection = !hasSelection && getPageSelectionText().trim().length > 0;

        if (key === 'c') {
            if (!event.shiftKey && !hasSelection) {
                return false;
            }
            if (hasSelection) {
                event.preventDefault();
                copyTerminalSelection(terminal.term);
                return true;
            }
            if (pageHasSelection) {
                event.preventDefault();
                copyTerminalSelection(terminal.term);
                return true;
            }
            return false;
        }

        if (key === 'v' || event.key === 'Insert' || event.code === 'Insert') {
            event.preventDefault();
            pasteToTerminal(terminal);
            return true;
        }

        return false;
    }

    window.copyActiveTerminalSelection = function() {
        const terminal = getActiveTerminal();
        const selection = getTerminalSelectionText(terminal);

        if (!terminal || !terminal.term || !selection) {
            return false;
        }

        return copyTerminalSelection(terminal.term);
    };

    window.pasteActiveTerminal = function() {
        const terminal = getActiveTerminal();
        if (!terminal) {
            return false;
        }
        pasteToTerminal(terminal);
        return true;
    };

    function getViewerCopyText() {
        let editor;

        if (state.previewContentType !== 'text' || !state.previewImagePath) {
            return '';
        }

        if (state.previewEditMode) {
            if (editorTextarea && editorTextarea.dataset.path === state.previewImagePath) {
                return editorTextarea.value;
            }
            editor = findEditor(state.previewImagePath);
            if (editor) {
                return editor.content || '';
            }
        }

        return typeof state.previewTextContent === 'string' ? state.previewTextContent : '';
    }

    window.copyViewerText = function() {
        return writeClipboardText(getViewerCopyText());
    };

    function shouldRouteTabToTerminal(e) {
        const activeTerminal = getActiveTerminal();
        const activeElement = document.activeElement;

        if (e.key !== 'Tab' || e.ctrlKey || e.metaKey || e.altKey) {
            return false;
        }
        if (document.getElementById('terminal-view').style.display === 'none') {
            return false;
        }
        if (state.terminalSettingsOpen || !activeTerminal || !activeTerminal.ws || activeTerminal.ws.readyState !== WebSocket.OPEN) {
            return false;
        }
        if (shouldKeepTabForUI(activeElement)) {
            return false;
        }
        return true;
    }

    function setCtrlActive(active) {
        state.ctrlActive = active;
        document.getElementById('ctrl-btn').classList.toggle('active', active);
    }

    function setLoginError(message) {
        const errorEl = document.getElementById('login-error');
        errorEl.textContent = message || '';
        errorEl.style.display = message ? 'block' : 'none';
    }

    function clampNumber(value, min, max, fallback) {
        const num = Number(value);
        if (!Number.isFinite(num)) {
            return fallback;
        }
        return Math.max(min, Math.min(max, Math.round(num)));
    }

    function sanitizeUIConfig(raw) {
        const terminal = raw && raw.terminal ? raw.terminal : null;

        return {
            title: raw && typeof raw.title === 'string' && raw.title.trim()
                ? raw.title.trim()
                : DEFAULT_UI_CONFIG.title,
            motd: raw && typeof raw.motd === 'string'
                ? raw.motd.trim()
                : DEFAULT_UI_CONFIG.motd,
            scrollback: clampNumber(terminal && terminal.scrollback, 1, 200000, DEFAULT_UI_CONFIG.scrollback),
            tmux: Boolean(terminal && terminal.tmux)
        };
    }

    function applyUIConfig() {
        const uiConfig = state.uiConfig;

        document.title = uiConfig.title;
        document.getElementById('header-title').textContent = uiConfig.title;
        loginTitle.textContent = uiConfig.title;
        loginSubtitle.textContent = uiConfig.motd || t('login.subtitle');

        Object.keys(state.terminals).forEach(function(id) {
            state.terminals[id].term.options.scrollback = uiConfig.scrollback;
        });
    }

    async function loadUIConfig() {
        try {
            state.uiConfig = sanitizeUIConfig(await fetchJSON('/api/ui-config'));
        } catch (_) {
            state.uiConfig = sanitizeUIConfig(null);
        }

        applyUIConfig();
    }

    function normalizeHexColor(value, fallback) {
        return /^#[0-9a-fA-F]{6}$/.test(value || '') ? value : fallback;
    }

    function hexToRgba(hex, alpha) {
        const normalized = normalizeHexColor(hex, '#000000');
        const r = parseInt(normalized.slice(1, 3), 16);
        const g = parseInt(normalized.slice(3, 5), 16);
        const b = parseInt(normalized.slice(5, 7), 16);
        return 'rgba(' + r + ', ' + g + ', ' + b + ', ' + alpha + ')';
    }

    function sanitizeAnsiPalette(raw, fallback) {
        const next = {};

        TERMINAL_ANSI_FIELDS.forEach(function(field) {
            next[field.key] = normalizeHexColor(raw && raw[field.key], fallback[field.key]);
        });

        return next;
    }

    function getAnsiInputId(field) {
        return 'terminal-ansi-' + field.idSuffix;
    }

    function sanitizeTerminalSettings(raw) {
        const next = Object.assign({}, DEFAULT_TERMINAL_SETTINGS);
        const presetName = raw && typeof raw.themePreset === 'string' ? raw.themePreset : DEFAULT_TERMINAL_SETTINGS.themePreset;
        const preset = TERMINAL_THEME_PRESETS[presetName] || null;

        next.fontFamily = raw && typeof raw.fontFamily === 'string' && TERMINAL_FONT_FAMILIES.indexOf(raw.fontFamily) !== -1
            ? raw.fontFamily
            : DEFAULT_TERMINAL_SETTINGS.fontFamily;
        next.fontSize = clampNumber(raw && raw.fontSize, 11, 24, DEFAULT_TERMINAL_SETTINGS.fontSize);
        next.cursorBlink = raw && Object.prototype.hasOwnProperty.call(raw, 'cursorBlink')
            ? Boolean(raw.cursorBlink)
            : DEFAULT_TERMINAL_SETTINGS.cursorBlink;
        next.themePreset = presetName === 'custom' || preset ? presetName : DEFAULT_TERMINAL_SETTINGS.themePreset;

        next.background = normalizeHexColor(raw && raw.background, preset ? preset.background : DEFAULT_TERMINAL_SETTINGS.background);
        next.foreground = normalizeHexColor(raw && raw.foreground, preset ? preset.foreground : DEFAULT_TERMINAL_SETTINGS.foreground);
        next.cursor = normalizeHexColor(raw && raw.cursor, preset ? preset.cursor : DEFAULT_TERMINAL_SETTINGS.cursor);
        next.selection = normalizeHexColor(raw && raw.selection, preset ? preset.selection : DEFAULT_TERMINAL_SETTINGS.selection);
        next.ansi = sanitizeAnsiPalette(raw && raw.ansi, preset ? preset.ansi : DEFAULT_TERMINAL_SETTINGS.ansi);
        return next;
    }

    function buildTerminalTheme(settings) {
        return Object.assign({
            background: settings.background,
            foreground: settings.foreground,
            cursor: settings.cursor,
            cursorAccent: settings.background,
            selectionBackground: hexToRgba(settings.selection, 0.45),
            selectionForeground: '#ffffff',
            selectionInactiveBackground: hexToRgba(settings.selection, 0.25)
        }, settings.ansi);
    }

    function getTerminalOptions() {
        const settings = state.terminalSettings;
        return {
            cursorBlink: settings.cursorBlink,
            fontSize: settings.fontSize,
            fontFamily: settings.fontFamily,
            scrollback: state.uiConfig.scrollback,
            theme: buildTerminalTheme(settings),
            allowProposedApi: true,
            rightClickSelectsWord: true,
            allowTransparency: false
        };
    }

    function syncTerminalSettingsPreview() {
        const settings = state.terminalSettings;
        const selection = terminalSettingsPreviewBody.querySelector('.terminal-settings-preview-selection');
        const muted = terminalSettingsPreviewBody.querySelectorAll('.terminal-settings-preview-muted');
        const accent = terminalSettingsPreviewBody.querySelector('.terminal-settings-preview-accent');

        terminalSettingsPreview.style.backgroundColor = settings.background;
        terminalSettingsPreviewBody.style.backgroundColor = settings.background;
        terminalSettingsPreviewBody.style.color = settings.foreground;
        terminalSettingsPreviewBody.style.fontFamily = settings.fontFamily;
        terminalSettingsPreviewBody.style.fontSize = settings.fontSize + 'px';
        terminalSettingsPreviewCursor.style.backgroundColor = settings.cursor;
        selection.style.backgroundColor = hexToRgba(settings.selection, 0.35);
        accent.style.color = settings.cursor;

        muted.forEach(function(node) {
            node.style.color = hexToRgba(settings.foreground, 0.62);
        });

        TERMINAL_ANSI_FIELDS.forEach(function(field) {
            const color = settings.ansi[field.key];
            const chip = terminalSettingsPreview.querySelector('[data-palette-preview="' + field.key + '"]');
            const samples = terminalSettingsPreviewBody.querySelectorAll('[data-ansi-preview="' + field.key + '"]');

            if (chip) {
                chip.style.backgroundColor = color;
            }

            samples.forEach(function(node) {
                node.style.color = color;
            });
        });
    }

    function syncTerminalSettingsForm() {
        const settings = state.terminalSettings;

        syncLanguageControl();
        document.getElementById('terminal-font-family').value = settings.fontFamily;
        document.getElementById('terminal-theme-preset').value = settings.themePreset;
        document.getElementById('terminal-cursor-blink').checked = settings.cursorBlink;
        document.getElementById('terminal-color-background').value = settings.background;
        document.getElementById('terminal-color-foreground').value = settings.foreground;
        document.getElementById('terminal-color-cursor').value = settings.cursor;
        document.getElementById('terminal-color-selection').value = settings.selection;
        TERMINAL_ANSI_FIELDS.forEach(function(field) {
            document.getElementById(getAnsiInputId(field)).value = settings.ansi[field.key];
        });
        terminalFontSizeInput.value = settings.fontSize;
        terminalFontSizeValue.textContent = settings.fontSize + 'px';
    }

    function saveTerminalSettings() {
        try {
            window.localStorage.setItem(TERMINAL_SETTINGS_STORAGE_KEY, JSON.stringify(state.terminalSettings));
        } catch (_) {
            // Ignore storage failures and continue with in-memory settings.
        }
    }

    function applyTerminalSettings() {
        const settings = state.terminalSettings;

        Object.keys(state.terminals).forEach(function(id) {
            const entry = state.terminals[id];
            entry.term.options.fontFamily = settings.fontFamily;
            entry.term.options.fontSize = settings.fontSize;
            entry.term.options.cursorBlink = settings.cursorBlink;
            entry.term.options.scrollback = state.uiConfig.scrollback;
            entry.term.options.theme = buildTerminalTheme(settings);
        });

        syncTerminalSettingsPreview();
        scheduleFitActiveTerminal();
    }

    function commitTerminalSettings(nextSettings) {
        state.terminalSettings = sanitizeTerminalSettings(nextSettings);
        syncTerminalSettingsForm();
        saveTerminalSettings();
        applyTerminalSettings();
    }

    function readTerminalSettingsFromControls() {
        const ansi = {};

        TERMINAL_ANSI_FIELDS.forEach(function(field) {
            ansi[field.key] = document.getElementById(getAnsiInputId(field)).value;
        });

        return sanitizeTerminalSettings({
            fontFamily: document.getElementById('terminal-font-family').value,
            fontSize: terminalFontSizeInput.value,
            cursorBlink: document.getElementById('terminal-cursor-blink').checked,
            themePreset: document.getElementById('terminal-theme-preset').value,
            background: document.getElementById('terminal-color-background').value,
            foreground: document.getElementById('terminal-color-foreground').value,
            cursor: document.getElementById('terminal-color-cursor').value,
            selection: document.getElementById('terminal-color-selection').value,
            ansi: ansi
        });
    }

    function handleTerminalSettingsChange(event) {
        let nextSettings;

        if (event.target.id === 'terminal-theme-preset') {
            nextSettings = Object.assign({}, state.terminalSettings, {
                themePreset: event.target.value
            });

            if (TERMINAL_THEME_PRESETS[event.target.value]) {
                Object.assign(nextSettings, TERMINAL_THEME_PRESETS[event.target.value]);
            }

            commitTerminalSettings(nextSettings);
            return;
        }

        nextSettings = readTerminalSettingsFromControls();
        if (
            event.target.id === 'terminal-color-background' ||
            event.target.id === 'terminal-color-foreground' ||
            event.target.id === 'terminal-color-cursor' ||
            event.target.id === 'terminal-color-selection' ||
            event.target.id.indexOf('terminal-ansi-') === 0
        ) {
            nextSettings.themePreset = 'custom';
        }

        commitTerminalSettings(nextSettings);
    }

    function bindTerminalSettingsControls() {
        document.getElementById('terminal-font-family').addEventListener('change', handleTerminalSettingsChange);
        terminalFontSizeInput.addEventListener('input', handleTerminalSettingsChange);
        document.getElementById('terminal-cursor-blink').addEventListener('change', handleTerminalSettingsChange);
        document.getElementById('terminal-theme-preset').addEventListener('change', handleTerminalSettingsChange);
        document.getElementById('terminal-color-background').addEventListener('input', handleTerminalSettingsChange);
        document.getElementById('terminal-color-foreground').addEventListener('input', handleTerminalSettingsChange);
        document.getElementById('terminal-color-cursor').addEventListener('input', handleTerminalSettingsChange);
        document.getElementById('terminal-color-selection').addEventListener('input', handleTerminalSettingsChange);
        TERMINAL_ANSI_FIELDS.forEach(function(field) {
            document.getElementById(getAnsiInputId(field)).addEventListener('input', handleTerminalSettingsChange);
        });
        if (interfaceLanguageSelect) {
            interfaceLanguageSelect.addEventListener('change', function(event) {
                setLanguage(event.target.value);
            });
        }
    }

    function loadTerminalSettings() {
        let parsed = null;

        try {
            parsed = JSON.parse(window.localStorage.getItem(TERMINAL_SETTINGS_STORAGE_KEY) || 'null');
        } catch (_) {
            parsed = null;
        }

        if (!parsed) {
            TERMINAL_SETTINGS_FALLBACK_KEYS.some(function(key) {
                let fallback = null;

                try {
                    fallback = JSON.parse(window.localStorage.getItem(key) || 'null');
                } catch (_) {
                    fallback = null;
                }

                if (!fallback || typeof fallback !== 'object') {
                    return false;
                }

                parsed = {
                    fontFamily: fallback.fontFamily,
                    fontSize: fallback.fontSize,
                    cursorBlink: fallback.cursorBlink,
                    themePreset: typeof fallback.themePreset === 'string' && TERMINAL_THEME_PRESETS[fallback.themePreset]
                        ? fallback.themePreset
                        : DEFAULT_TERMINAL_SETTINGS.themePreset
                };

                if (TERMINAL_THEME_PRESETS[parsed.themePreset]) {
                    Object.assign(parsed, TERMINAL_THEME_PRESETS[parsed.themePreset]);
                }

                return true;
            });
        }

        state.terminalSettings = sanitizeTerminalSettings(parsed);
        syncTerminalSettingsForm();
        syncTerminalSettingsPreview();
        saveTerminalSettings();
    }

    window.openTerminalSettings = function() {
        setFileBrowserOpen(false);
        window.closeImagePreview();
        state.terminalSettingsOpen = true;
        syncTerminalSettingsForm();
        syncTerminalSettingsPreview();
        terminalSettingsModal.style.display = 'block';
    };

    window.closeTerminalSettings = function() {
        state.terminalSettingsOpen = false;
        terminalSettingsModal.style.display = 'none';
    };

    window.resetTerminalSettings = function() {
        commitTerminalSettings(Object.assign({}, DEFAULT_TERMINAL_SETTINGS));
    };

    function clearUploadHideTimer() {
        if (uploadHideTimer) {
            clearTimeout(uploadHideTimer);
            uploadHideTimer = null;
        }
    }

    function syncUploadCancelButton() {
        const button = document.getElementById('file-upload-cancel-btn');
        if (!button) {
            return;
        }
        button.style.display = state.upload.active ? 'inline-flex' : 'none';
        button.disabled = !state.upload.active;
    }

    function setUploadButtonBusy(active) {
        const button = document.getElementById('upload-btn');
        button.disabled = active;
        button.classList.toggle('is-busy', active);
        button.setAttribute('aria-busy', active ? 'true' : 'false');
    }

    function setUploadStatus(label, detail, percent, tone) {
        const status = document.getElementById('file-upload-status');
        const progressBar = document.getElementById('file-upload-progress-bar');

        clearUploadHideTimer();
        status.className = 'file-upload-status' + (tone ? ' ' + tone : '');
        status.style.display = 'block';
        document.getElementById('file-upload-label').textContent = label;
        document.getElementById('file-upload-detail').textContent = detail || '';
        document.getElementById('file-upload-percent').textContent = Math.round(percent) + '%';
        progressBar.style.width = Math.max(0, Math.min(100, percent)) + '%';
        syncUploadCancelButton();
    }

    function abortActiveUpload(silent) {
        const xhr = state.upload.currentXHR;

        if (!xhr) {
            return false;
        }

        state.upload.cancelRequested = !silent;
        state.upload.currentXHR = null;
        try {
            xhr.abort();
        } catch (_) {
            return false;
        }
        return true;
    }

    function resetUploadState() {
        const status = document.getElementById('file-upload-status');

        clearUploadHideTimer();
        state.upload.active = false;
        state.upload.totalFiles = 0;
        state.upload.totalBytes = 0;
        state.upload.completedBytes = 0;
        state.upload.currentFileIndex = 0;
        state.upload.currentFileName = '';
        state.upload.currentFileLoaded = 0;
        state.upload.currentFileSize = 0;
        state.upload.currentXHR = null;
        state.upload.cancelRequested = false;
        setFileDropActive(false);
        setUploadButtonBusy(false);
        status.style.display = 'none';
        status.className = 'file-upload-status';
        document.getElementById('file-upload-label').textContent = t('upload.uploading');
        document.getElementById('file-upload-detail').textContent = '';
        document.getElementById('file-upload-percent').textContent = '0%';
        document.getElementById('file-upload-progress-bar').style.width = '0%';
        syncUploadCancelButton();
    }

    function scheduleUploadStatusHide(delay) {
        clearUploadHideTimer();
        uploadHideTimer = setTimeout(function() {
            resetUploadState();
        }, delay);
    }

    function getOverallUploadPercent() {
        const upload = state.upload;
        const totalLoaded = upload.completedBytes + upload.currentFileLoaded;

        if (upload.totalBytes > 0) {
            return Math.max(0, Math.min(100, Math.round((totalLoaded / upload.totalBytes) * 100)));
        }
        if (upload.totalFiles > 0) {
            return Math.max(0, Math.min(100, Math.round(((upload.currentFileIndex - 1) / upload.totalFiles) * 100)));
        }
        return 0;
    }

    function updateUploadProgress() {
        const upload = state.upload;
        let detail = upload.currentFileName;

        if (upload.currentFileSize > 0) {
            detail += ' • ' + formatSize(upload.currentFileLoaded) + ' / ' + formatSize(upload.currentFileSize);
        }

        setUploadButtonBusy(true);
        setUploadStatus(
            t('upload.progress', { current: upload.currentFileIndex, total: upload.totalFiles }),
            detail,
            getOverallUploadPercent(),
            ''
        );
    }

    function showUploadComplete() {
        const upload = state.upload;
        const detail = upload.totalFiles === 1
            ? t('upload.completeSingle', { name: upload.currentFileName })
            : t('upload.completeMultiple', { count: upload.totalFiles, size: formatSize(upload.totalBytes) });

        state.upload.active = false;
        state.upload.currentXHR = null;
        state.upload.cancelRequested = false;
        setUploadButtonBusy(false);
        setUploadStatus(t('upload.complete'), detail, 100, 'is-success');
        scheduleUploadStatusHide(1800);
    }

    function showUploadCanceled() {
        state.upload.active = false;
        state.upload.currentXHR = null;
        state.upload.cancelRequested = false;
        setUploadButtonBusy(false);
        setUploadStatus(t('upload.canceled'), state.upload.currentFileName || '', Math.max(0, getOverallUploadPercent()), '');
        scheduleUploadStatusHide(1800);
    }

    function showUploadError(message) {
        state.upload.active = false;
        state.upload.currentXHR = null;
        state.upload.cancelRequested = false;
        setUploadButtonBusy(false);
        setUploadStatus(t('upload.failed'), message, Math.max(6, getOverallUploadPercent()), 'is-error');
        scheduleUploadStatusHide(3600);
    }

    function buildRequestError(response, data, fallbackMessage) {
        const message = data && data.error ? data.error : fallbackMessage;
        const error = new Error(message || t('common.requestFailed'));
        error.status = response.status;
        return error;
    }

    function isHandledAuthError(err) {
        return Boolean(err && err.authHandled);
    }

    function resetSessionState() {
        stopMemoryStatusPolling();
        Object.keys(state.terminals).forEach(function(id) {
            closeTerminal(id, true, true);
        });
        state.activeId = null;
        persistEditorDrafts();
        resetEditorState();
        window.closeTerminalSettings();
        abortActiveUpload(true);
        resetUploadState();
        setFileBrowserOpen(false);
        window.closeImagePreview();
        state.username = '';
        state.currentPath = '';
        state.files = [];
        state.showHidden = false;
        state.fileFilterQuery = '';
        if (fileFilterInput) {
            fileFilterInput.value = '';
        }
        state.loadingTerminals = false;
        state.maximized = 'terminal';
        setCtrlActive(false);
        resetMemoryStatus();
    }

    function showLoginView(message) {
        authRedirectActive = true;
        resetSessionState();
        showView('login');
        setLoginPasswordVisible(false);
        setLoginError(message || '');
    }

    function showTerminalView(username) {
        authRedirectActive = false;
        state.username = username;
        setLoginError('');
        setFileBrowserOpen(false);
        window.closeImagePreview();
        showView('terminal');
        restoreEditorDrafts();
        startMemoryStatusPolling();
    }

    function handleUnauthorized(message) {
        const text = message === 'not authenticated'
            ? t('auth.relogin')
            : t('auth.sessionExpiredRelogin');

        if (authRedirectActive) {
            return;
        }

        showLoginView(text);
    }

    async function fetchAPI(url, options, settings) {
        const response = await fetch(typeof url === 'string' ? withBasePath(url) : url, options);

        if (settings && settings.authRequired && response.status === 401) {
            let data = null;

            try {
                data = await response.json();
            } catch (_) {
                data = null;
            }

            const error = buildRequestError(response, data, t('auth.sessionExpired'));
            error.authHandled = true;
            handleUnauthorized(error.message);
            throw error;
        }

        return response;
    }

    async function fetchJSON(url, options, settings) {
        const response = await fetchAPI(url, options, settings);
        let data = null;

        try {
            data = await response.json();
        } catch (_) {
            data = null;
        }

        if (!response.ok) {
            throw buildRequestError(response, data, t('common.requestFailed'));
        }

        return data;
    }

    async function isSessionStillValid() {
        try {
            const response = await fetch(withBasePath('/api/auth/status'), { cache: 'no-store' });
            let data = null;

            try {
                data = await response.json();
            } catch (_) {
                data = null;
            }

            if (!response.ok) {
                return response.status >= 500;
            }

            if (data && data.authenticated) {
                return true;
            }

            handleUnauthorized(data && data.error ? data.error : t('auth.sessionExpired'));
            return false;
        } catch (_) {
            return true;
        }
    }

    async function shouldReconnectTerminal(id) {
        if (!await isSessionStillValid()) {
            return false;
        }

        try {
            const sessions = await fetchJSON('/api/terminals', undefined, { authRequired: true });
            if (!Array.isArray(sessions)) {
                return true;
            }
            return sessions.some(function(session) {
                return session.id === id;
            });
        } catch (err) {
            if (isHandledAuthError(err)) {
                return false;
            }
            return true;
        }
    }

    // ========== Auth ==========

    function removeAppLoader() {
        var el = document.getElementById('app-loader');
        if (el) el.remove();
        document.body.classList.remove('app-loading');
    }

    function checkAuth() {
        fetch(withBasePath('/api/auth/status'))
            .then(r => r.json())
            .then(data => {
                if (data.authenticated) {
                    showTerminalView(data.username);
                    loadTerminals();
                } else {
                    showLoginView('');
                }
                removeAppLoader();
            })
            .catch(() => {
                showLoginView('');
                removeAppLoader();
            });
    }

    window.doLogin = function() {
        const username = document.getElementById('login-username').value.trim();
        const password = document.getElementById('login-password').value;
        setLoginError('');

        if (!username || !password) {
            setLoginError(t('auth.enterCredentials'));
            return;
        }

        fetchJSON('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username: username, password: password })
        })
            .then(data => {
                document.getElementById('login-password').value = '';
                setLoginPasswordVisible(false);
                showTerminalView(data.username);
                loadTerminals();
            })
            .catch(err => {
                setLoginError(err.message);
            });
    };

    window.doLogout = function() {
        fetch(withBasePath('/api/auth/logout'), { method: 'POST' })
            .catch(function() {
                return null;
            })
            .finally(function() {
                showLoginView('');
            });
    };

    window.goBack = function() {
        if (confirm(t('auth.disconnectConfirm'))) {
            window.doLogout();
        }
    };

    function showView(name) {
        syncBodyViewState(name);
        document.getElementById('login-view').style.display = name === 'login' ? 'flex' : 'none';
        document.getElementById('terminal-view').style.display = name === 'terminal' ? 'flex' : 'none';
        if (name === 'terminal') {
            syncVisibleTerminalConnections();
            applyMaximizeState();
            scheduleFitActiveTerminal();
        }
        if (name === 'login') {
            syncVisibleTerminalConnections();
            window.closeTerminalSettings();
            setFileBrowserOpen(false);
            window.closeImagePreview();
            // Force-hide all overlays that might bleed through
            document.getElementById('file-panel').classList.remove('open');
            document.getElementById('file-overlay').style.display = 'none';
            terminalSettingsModal.style.display = 'none';
        }
    }

    // ========== Terminals ==========

    function getNextDefaultTerminalName() {
        let maxNumber = 0;

        Object.keys(state.terminals).forEach(function(id) {
            const entry = state.terminals[id];
            const number = entry && entry.name ? extractDefaultTerminalNumber(entry.name) : 0;
            if (number) {
                maxNumber = Math.max(maxNumber, number);
            }
        });

        return getDefaultTerminalName(maxNumber + 1);
    }

    function loadTerminals() {
        hydrateWorkspaceState()
            .then(function() {
                return fetchJSON('/api/terminals', undefined, { authRequired: true });
            })
            .then(sessions => {
                if (!Array.isArray(sessions) || sessions.length === 0) {
                    window.createTerminal();
                    return;
                }

                state.loadingTerminals = true;
                sessions.forEach(function(session) {
                    connectTerminal(session.id, session.name);
                });
                reconcileWorkspacesAfterTerminalLoad(sessions[sessions.length - 1].id);
                state.loadingTerminals = false;
                renderTabBar();
                renderActiveWorkspace();
                syncWorkspaceFileBrowserDefaults('terminal-load');
                scheduleFitActiveTerminal();
            })
            .catch(function(err) {
                state.loadingTerminals = false;
                if (!isHandledAuthError(err)) {
                    alert(t('terminal.loadFailed', { message: err.message }));
                }
            });
    }

    function requestCreateTerminal() {
        return fetchJSON('/api/terminals', { method: 'POST' }, { authRequired: true })
            .then(function(data) {
                connectTerminal(data.id, data.name || null);
                return data;
            });
    }

    window.createTerminal = function() {
        var hadNoTerminals = Object.keys(state.terminals).length === 0;

        requestCreateTerminal()
            .then(function(data) {
                var workspace = createWorkspace('1', [data.id], '', getNextWorkspaceLabelNumber());

                if (hadNoTerminals) {
                    state.workspaces = [workspace];
                } else {
                    state.workspaces.push(workspace);
                }
                state.activeWorkspaceId = workspace.id;
                state.activeId = data.id;
                saveWorkspaceState();
                renderTabBar();
                renderActiveWorkspace();
                scheduleFitActiveTerminal();
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(t('terminal.createFailed', { message: err.message }));
                }
            });
    };

    window.createWorkspaceTab = function() {
        var activeWorkspace = getActiveWorkspace();
        var workspace = createWorkspace(activeWorkspace ? activeWorkspace.layout : '1', [], '', getNextWorkspaceLabelNumber());

        state.workspaces.push(workspace);
        state.activeWorkspaceId = workspace.id;
        rebalanceWorkspaceAssignments(workspace.id);
        ensureActiveTerminalVisible();
        saveWorkspaceState();
        renderTabBar();
        renderActiveWorkspace();
        syncWorkspaceFileBrowserDefaults('workspace-change');
        scheduleFitActiveTerminal();
    };

    window.setActiveWorkspaceLayout = function(layout) {
        var workspace = getActiveWorkspace();

        if (!workspace) {
            return;
        }

        workspace.layout = normalizeWorkspaceLayout(layout);
        rebalanceWorkspaceAssignments(workspace.id);
        saveWorkspaceState();
        renderTabBar();
        renderActiveWorkspace();
        syncWorkspaceFileBrowserDefaults('layout-change');
        scheduleFitActiveTerminal();
    };

    function assignTerminalToWorkspaceSlot(workspaceId, slotIndex, terminalId) {
        var workspace = getWorkspaceById(workspaceId);

        if (!workspace || slotIndex < 0 || slotIndex > 3) {
            return;
        }

        if (terminalId) {
            clearTerminalAssignment(terminalId, workspace.id, slotIndex);
        }
        workspace.terminalIds[slotIndex] = terminalId || '';
        rebalanceWorkspaceAssignments(workspace.id);
        ensureActiveTerminalVisible(terminalId || state.activeId);
        saveWorkspaceState();
        renderTabBar();
        renderActiveWorkspace();
        scheduleFitActiveTerminal();
    }

    function createTerminalForWorkspaceSlot(workspaceId, slotIndex) {
        requestCreateTerminal()
            .then(function(data) {
                assignTerminalToWorkspaceSlot(workspaceId, slotIndex, data.id);
                switchTab(workspaceId);
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(t('terminal.createFailed', { message: err.message }));
                }
            });
    }

    function closeWorkspaceTab(id) {
        var index = state.workspaces.findIndex(function(workspace) {
            return workspace.id === id;
        });

        if (index === -1) {
            return;
        }

        state.workspaces.splice(index, 1);
        if (state.activeWorkspaceId === id) {
            if (state.workspaces.length) {
                state.activeWorkspaceId = state.workspaces[Math.max(0, index - 1)].id;
            } else {
                state.activeWorkspaceId = null;
            }
        }
        rebalanceWorkspaceAssignments(state.activeWorkspaceId);
        ensureActiveTerminalVisible();
        saveWorkspaceState();
        renderTabBar();
        renderActiveWorkspace();
        syncWorkspaceFileBrowserDefaults('workspace-change');
        scheduleFitActiveTerminal();
    }

    function replaceMissingTerminal(id) {
        closeTerminal(id, true);
    }

    function getTerminalViewport(entry) {
        if (!entry) {
            return null;
        }
        if (entry.viewport && entry.viewport.isConnected) {
            return entry.viewport;
        }
        entry.viewport = entry.wrapper ? entry.wrapper.querySelector('.xterm-viewport') : null;
        return entry.viewport || null;
    }

    function getTerminalBuffer(entry) {
        if (!entry || !entry.term || !entry.term.buffer) {
            return null;
        }
        return entry.term.buffer.active || entry.term.buffer;
    }

    function getTerminalScrollMetrics(entry) {
        const tmuxState = entry && entry.tmuxScrollState;
        const buffer = getTerminalBuffer(entry);
        const rows = entry && entry.term && typeof entry.term.rows === 'number'
            ? Math.max(entry.term.rows, 1)
            : 1;
        let totalLines = rows;
        let viewportTop = 0;
        let scrollPosition = 0;

        if (state.uiConfig.tmux && tmuxState && typeof tmuxState.paneHeight === 'number') {
            totalLines = Math.max(tmuxState.historySize + tmuxState.paneHeight, tmuxState.paneHeight);
            scrollPosition = Math.max(0, Math.min(tmuxState.scrollPosition || 0, tmuxState.historySize || 0));
            return {
                totalLines: totalLines,
                visibleLines: Math.max(tmuxState.paneHeight, 1),
                maxTop: Math.max(tmuxState.historySize, 0),
                viewportTop: Math.max(0, (tmuxState.historySize || 0) - scrollPosition),
                scrollPosition: scrollPosition,
                source: 'tmux'
            };
        }

        if (buffer) {
            if (typeof buffer.length === 'number') {
                totalLines = Math.max(totalLines, buffer.length);
            } else if (buffer.lines && typeof buffer.lines.length === 'number') {
                totalLines = Math.max(totalLines, buffer.lines.length);
            } else if (typeof buffer.baseY === 'number') {
                totalLines = Math.max(totalLines, buffer.baseY + rows);
            } else if (typeof buffer.ybase === 'number') {
                totalLines = Math.max(totalLines, buffer.ybase + rows);
            }

            if (typeof buffer.viewportY === 'number') {
                viewportTop = buffer.viewportY;
            } else if (typeof buffer.ydisp === 'number') {
                viewportTop = buffer.ydisp;
            } else if (typeof buffer.baseY === 'number') {
                viewportTop = buffer.baseY;
            } else if (typeof buffer.ybase === 'number') {
                viewportTop = buffer.ybase;
            }
        }

        totalLines = Math.max(totalLines, rows);
        return {
            totalLines: totalLines,
            visibleLines: rows,
            maxTop: Math.max(totalLines - rows, 0),
            viewportTop: Math.max(0, viewportTop),
            scrollPosition: Math.max(0, Math.max(totalLines - rows, 0) - Math.max(0, viewportTop)),
            source: 'xterm'
        };
    }

    function clearTerminalScrollTarget(entry) {
        if (!entry) {
            return;
        }
        entry.scrollTargetTop = null;
        if (entry.scrollTargetTimer) {
            clearTimeout(entry.scrollTargetTimer);
            entry.scrollTargetTimer = null;
        }
    }

    function scheduleTerminalScrollTarget(entry) {
        if (!entry || entry.scrollTargetTop == null || entry.scrollTargetTimer || (state.uiConfig.tmux && entry.scrollStateAwaiting)) {
            return;
        }

        entry.scrollTargetTimer = setTimeout(function() {
            entry.scrollTargetTimer = null;
            flushTerminalScrollTarget(entry);
        }, 24);
    }

    function scheduleTerminalScrollbarSync(entry) {
        if (!entry || entry.scrollbarSyncFrame) {
            return;
        }

        entry.scrollbarSyncFrame = window.requestAnimationFrame(function() {
            entry.scrollbarSyncFrame = null;
            syncTerminalScrollbar(entry);
            if (entry.scrollTargetTop != null) {
                scheduleTerminalScrollTarget(entry);
            }
        });
    }

    function syncTerminalScrollbar(entry) {
        const track = entry && entry.scrollbar;
        const thumb = entry && entry.scrollbarThumb;
        const metrics = getTerminalScrollMetrics(entry);
        let trackHeight;
        let thumbHeight;
        let thumbTop;

        if (!track || !thumb) {
            return;
        }

        trackHeight = track.clientHeight || 0;

        if (trackHeight <= 0 || metrics.totalLines <= 0 || metrics.visibleLines <= 0) {
            thumb.style.top = '0px';
            thumb.style.height = '24px';
            return;
        }

        thumbHeight = metrics.maxTop > 0
            ? Math.max(24, Math.round((metrics.visibleLines / metrics.totalLines) * trackHeight))
            : trackHeight;
        thumbHeight = Math.min(trackHeight, thumbHeight);
        thumbTop = metrics.maxTop > 0
            ? Math.round((Math.min(metrics.viewportTop, metrics.maxTop) / metrics.maxTop) * Math.max(trackHeight - thumbHeight, 0))
            : 0;

        thumb.style.height = thumbHeight + 'px';
        thumb.style.top = thumbTop + 'px';
    }

    function flushTerminalScrollTarget(entry) {
        const metrics = getTerminalScrollMetrics(entry);
        const targetTop = entry ? entry.scrollTargetTop : null;
        let clampedTarget;
        let delta;
        let desiredScrollPosition;

        if (!entry || targetTop == null) {
            return;
        }

        clampedTarget = Math.max(0, Math.min(metrics.maxTop, Math.round(targetTop)));
        entry.scrollTargetTop = clampedTarget;

        if (clampedTarget >= Math.max(metrics.maxTop - 1, 0)) {
            clearTerminalScrollTarget(entry);
            if (state.uiConfig.tmux) {
                resetTerminalScrollMode(entry);
            } else if (entry.term && typeof entry.term.scrollToBottom === 'function') {
                entry.term.scrollToBottom();
                scheduleTerminalScrollbarSync(entry);
            }
            return;
        }

        if (state.uiConfig.tmux) {
            if (entry.scrollStateAwaiting) {
                scheduleTerminalScrollTarget(entry);
                return;
            }

            desiredScrollPosition = Math.max(0, metrics.maxTop - clampedTarget);
            delta = (metrics.scrollPosition || 0) - desiredScrollPosition;
        } else {
            delta = clampedTarget - Math.min(metrics.viewportTop, metrics.maxTop);
        }

        if (Math.abs(delta) <= 1) {
            clearTerminalScrollTarget(entry);
            return;
        }

        if (state.uiConfig.tmux) {
            if (!queueTerminalScrollLines(entry, Math.max(-100, Math.min(100, delta)))) {
                clearTerminalScrollTarget(entry);
            }
            return;
        }

        if (entry.term && typeof entry.term.scrollToLine === 'function') {
            entry.term.scrollToLine(clampedTarget);
        } else if (entry.term && typeof entry.term.scrollLines === 'function') {
            entry.term.scrollLines(delta);
        }
        clearTerminalScrollTarget(entry);
        scheduleTerminalScrollbarSync(entry);
    }

    function getTerminalScrollTargetFromPointer(entry, clientY, pointerOffset) {
        const track = entry && entry.scrollbar;
        const thumb = entry && entry.scrollbarThumb;
        const metrics = getTerminalScrollMetrics(entry);
        const trackRect = track ? track.getBoundingClientRect() : null;
        const thumbHeight = thumb ? thumb.offsetHeight : 0;
        let maxThumbTop;
        let targetThumbTop;
        let ratio;

        if (!trackRect || trackRect.height <= 0) {
            return null;
        }

        maxThumbTop = Math.max(trackRect.height - thumbHeight, 0);
        targetThumbTop = clientY - trackRect.top - pointerOffset;
        targetThumbTop = Math.max(0, Math.min(maxThumbTop, targetThumbTop));
        ratio = maxThumbTop > 0 ? (targetThumbTop / maxThumbTop) : 0;
        return Math.round(ratio * metrics.maxTop);
    }

    function scrollTerminalViewportFromPointer(entry, clientY, pointerOffset) {
        const targetTop = getTerminalScrollTargetFromPointer(entry, clientY, pointerOffset);

        if (!entry || targetTop == null) {
            return;
        }

        entry.scrollTargetTop = targetTop;
        flushTerminalScrollTarget(entry);
        scheduleTerminalScrollbarSync(entry);
    }

    function beginTerminalScrollbarDrag(entry, event, fromThumb) {
        const thumb = entry && entry.scrollbarThumb;
        const thumbRect = thumb ? thumb.getBoundingClientRect() : null;

        if (!entry || !thumb || !thumbRect) {
            return;
        }

        event.preventDefault();
        event.stopPropagation();
        state.terminalScrollbarDrag = {
            terminalId: entry.id,
            pointerId: event.pointerId,
            pointerOffset: fromThumb ? (event.clientY - thumbRect.top) : (thumbRect.height / 2)
        };
        thumb.classList.add('is-dragging');
        if (entry.scrollbar && entry.scrollbar.setPointerCapture) {
            entry.scrollbar.setPointerCapture(event.pointerId);
        }
        scrollTerminalViewportFromPointer(entry, event.clientY, state.terminalScrollbarDrag.pointerOffset);
    }

    function handleTerminalScrollbarDrag(event) {
        const drag = state.terminalScrollbarDrag;
        const entry = drag ? state.terminals[drag.terminalId] : null;

        if (!drag || !entry || drag.pointerId !== event.pointerId) {
            return;
        }

        event.preventDefault();
        scrollTerminalViewportFromPointer(entry, event.clientY, drag.pointerOffset);
    }

    function endTerminalScrollbarDrag(event) {
        const drag = state.terminalScrollbarDrag;
        const entry = drag ? state.terminals[drag.terminalId] : null;

        if (!drag || drag.pointerId !== event.pointerId) {
            return;
        }

        if (entry && entry.scrollbarThumb) {
            entry.scrollbarThumb.classList.remove('is-dragging');
        }
        if (entry && entry.scrollbar && entry.scrollbar.releasePointerCapture) {
            entry.scrollbar.releasePointerCapture(event.pointerId);
        }
        state.terminalScrollbarDrag = null;
    }

    function attachTerminalScrollbar(entry) {
        const viewport = getTerminalViewport(entry);

        if (!entry || !entry.wrapper || !viewport) {
            return;
        }

        entry.scrollbar = document.createElement('div');
        entry.scrollbar.className = 'terminal-scrollbar';
        entry.scrollbarThumb = document.createElement('div');
        entry.scrollbarThumb.className = 'terminal-scrollbar-thumb';
        entry.scrollbar.appendChild(entry.scrollbarThumb);
        entry.wrapper.appendChild(entry.scrollbar);

        viewport.addEventListener('scroll', function() {
            scheduleTerminalScrollbarSync(entry);
        });
        entry.scrollbar.addEventListener('pointerdown', function(event) {
            beginTerminalScrollbarDrag(entry, event, event.target === entry.scrollbarThumb);
        });

        if (typeof entry.term.onScroll === 'function') {
            entry.term.onScroll(function() {
                scheduleTerminalScrollbarSync(entry);
            });
        }

        if (typeof window.ResizeObserver === 'function') {
            entry.scrollbarObserver = new window.ResizeObserver(function() {
                scheduleTerminalScrollbarSync(entry);
            });
            entry.scrollbarObserver.observe(entry.surface);
            entry.scrollbarObserver.observe(entry.scrollbar);
        }

        scheduleTerminalScrollbarSync(entry);
    }

    function connectTerminal(id, name) {
        if (state.terminals[id]) {
            return;
        }

        const term = new Terminal(getTerminalOptions());

        const fitAddon = new FitAddon.FitAddon();
        term.loadAddon(fitAddon);

        const wrapper = document.createElement('div');
        wrapper.className = 'terminal-wrapper';
        wrapper.id = 'term-' + id;
        const surface = document.createElement('div');
        surface.className = 'terminal-surface';
        wrapper.appendChild(surface);
        terminalDock.appendChild(wrapper);

        term.open(surface);

        const t = {
            id: id,
            term: term,
            ws: null,
            connecting: false,
            desiredConnected: false,
            detachRequested: false,
            scrollModeActive: false,
            pendingScrollLines: 0,
            scrollFlushTimer: null,
            scrollStateAwaiting: false,
            tmuxScrollState: null,
            scrollTargetTop: null,
            scrollTargetTimer: null,
            fitAddon: fitAddon,
            wrapper: wrapper,
            surface: surface,
            name: name && name.trim ? name.trim() : name
        };
        if (!t.name) {
            t.name = getNextDefaultTerminalName();
        }
        state.terminals[id] = t;
        attachTerminalScrollbar(t);

        wrapper.addEventListener('pointerdown', function() {
            if (getVisibleTerminalIds().indexOf(id) !== -1) {
                state.activeId = id;
                syncActiveTerminalPane();
            }
        });

        wrapper.addEventListener('wheel', function(event) {
            const metrics = getTerminalScrollMetrics(t);

            if (event.ctrlKey || event.metaKey || event.altKey) {
                return;
            }
            if (state.uiConfig.tmux && event.deltaY > 0 && metrics.viewportTop >= Math.max(metrics.maxTop - 1, 0)) {
                clearTerminalScrollTarget(t);
                resetTerminalScrollMode(t);
                scheduleTerminalScrollbarSync(t);
                event.preventDefault();
                return;
            }
            if (!queueTerminalScroll(t, event.deltaY)) {
                return;
            }
            if (getVisibleTerminalIds().indexOf(id) !== -1) {
                state.activeId = id;
                syncActiveTerminalPane();
            }
            event.preventDefault();
        }, { passive: false });

        if (typeof term.onFocus === 'function') {
            term.onFocus(function() {
                if (getVisibleTerminalIds().indexOf(id) !== -1) {
                    state.activeId = id;
                    syncActiveTerminalPane();
                }
            });
        }

        term.attachCustomKeyEventHandler(function(event) {
            if (event.type !== 'keydown') {
                return true;
            }

            const key = (event.key || '').toLowerCase();
            const hasTerminalSelection = typeof term.hasSelection === 'function' && term.hasSelection();
            const hasPageSelection = !hasTerminalSelection && getPageSelectionText().trim().length > 0;

            // Tab / Shift+Tab → send to terminal
            if (event.key === 'Tab' && !event.ctrlKey && !event.metaKey && !event.altKey) {
                event.preventDefault();
                sendTerminalSequence(t, event.shiftKey ? '\x1b[Z' : '\t');
                return false;
            }

            // Ctrl+C / Cmd+C: copy if text selected, otherwise send SIGINT
            if (key === 'c' && (event.ctrlKey || event.metaKey) && !event.shiftKey && !event.altKey) {
                if (hasTerminalSelection || hasPageSelection) {
                    copyTerminalSelection(term);
                    return false;
                }
                return true; // let xterm send SIGINT
            }

            // Ctrl+Shift+C / Cmd+Shift+C: always copy
            if (key === 'c' && (event.ctrlKey || event.metaKey) && event.shiftKey && !event.altKey) {
                if (hasTerminalSelection || hasPageSelection) {
                    copyTerminalSelection(term);
                }
                return false;
            }

            // Ctrl+V / Cmd+V / Ctrl+Shift+V: paste from clipboard
            if ((key === 'v') && (event.ctrlKey || event.metaKey) && !event.altKey) {
                pasteToTerminal(t);
                return false;
            }

            return true;
        });

        term.onData(function(data) {
            const activeWs = t.ws;
            if (!activeWs || activeWs.readyState !== WebSocket.OPEN) {
                return;
            }

            if (state.ctrlActive) {
                const code = data.charCodeAt(0);
                let out = data;

                if (code >= 97 && code <= 122) {
                    out = String.fromCharCode(code - 96);
                } else if (code >= 65 && code <= 90) {
                    out = String.fromCharCode(code - 64);
                }

                sendTerminalSequence(t, out);
                setCtrlActive(false);
                return;
            }

            sendTerminalSequence(t, data);
        });

        term.onResize(function(size) {
            scheduleTerminalScrollbarSync(t);
            if (t.ws && t.ws.readyState === WebSocket.OPEN) {
                sendResize(t.ws, size.rows, size.cols);
                requestTerminalScrollState(t);
            }
        });

        if (!state.workspaces.length) {
            state.workspaces = [createWorkspace('1', [id], '', 1)];
            state.activeWorkspaceId = state.workspaces[0].id;
            state.activeId = id;
            saveWorkspaceState();
        }
        renderTabBar();
        renderActiveWorkspace();
    }

    function openTerminalSocket(id, terminalEntry, isReconnect) {
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(proto + '//' + location.host + withBasePath('/api/terminals/ws/' + id));
        ws.binaryType = 'arraybuffer';
        terminalEntry.connecting = true;
        terminalEntry.detachRequested = false;
        terminalEntry.ws = ws;

        ws.onopen = function() {
            terminalEntry.connecting = false;
            terminalEntry.scrollModeActive = false;
            terminalEntry.pendingScrollLines = 0;
            terminalEntry.scrollStateAwaiting = false;
            clearTerminalScrollTarget(terminalEntry);
            if (!terminalEntry.desiredConnected || !isTerminalViewVisible() || !isTerminalVisibleInActiveWorkspace(id)) {
                terminalEntry.detachRequested = true;
                ws.close();
                return;
            }
            terminalEntry.reconnectAttempts = 0;
            if (isReconnect) {
                terminalEntry.term.write('\r\n\x1b[32m[' + t('terminal.reconnected') + ']\x1b[0m\r\n');
            }
            scheduleFitActiveTerminal();
            scheduleTerminalScrollbarSync(terminalEntry);
            sendResize(terminalEntry.ws, terminalEntry.term.rows, terminalEntry.term.cols);
            requestTerminalScrollState(terminalEntry);
        };

        ws.onmessage = function(event) {
            if (event.data instanceof ArrayBuffer) {
                terminalEntry.term.write(new Uint8Array(event.data));
            } else {
                if (typeof event.data === 'string') {
                    try {
                        const payload = JSON.parse(event.data);

                        if (payload && payload.type === 'scroll_state') {
                            updateTerminalScrollState(terminalEntry, payload);
                            return;
                        }
                    } catch (error) {
                        // fall through and treat plain text frames as terminal output
                    }
                }
                terminalEntry.term.write(event.data);
            }
            scheduleTerminalScrollbarSync(terminalEntry);
        };

        ws.onerror = function() {
            terminalEntry.term.write('\r\n\x1b[31m[' + t('terminal.connectionError') + ']\x1b[0m\r\n');
        };

        ws.onclose = function() {
            if (!state.terminals[id] || state.terminals[id].ws !== ws) {
                return;
            }

            terminalEntry.connecting = false;
            terminalEntry.ws = null;
            terminalEntry.scrollModeActive = false;
            terminalEntry.pendingScrollLines = 0;
            terminalEntry.scrollStateAwaiting = false;
            clearTerminalScrollTarget(terminalEntry);
            if (terminalEntry.scrollFlushTimer) {
                clearTimeout(terminalEntry.scrollFlushTimer);
                terminalEntry.scrollFlushTimer = null;
            }
            if (terminalEntry.detachRequested || !terminalEntry.desiredConnected || !isTerminalViewVisible() || !isTerminalVisibleInActiveWorkspace(id)) {
                terminalEntry.detachRequested = false;
                return;
            }

            shouldReconnectTerminal(id).then(function(shouldReconnect) {
                var delay;

                if (!state.terminals[id] || state.terminals[id].ws || !terminalEntry.desiredConnected || !isTerminalViewVisible() || !isTerminalVisibleInActiveWorkspace(id)) {
                    return;
                }

                if (!shouldReconnect) {
                    replaceMissingTerminal(id);
                    return;
                }

                terminalEntry.term.write('\r\n\x1b[33m[' + t('terminal.disconnected') + ']\x1b[0m\r\n');
                delay = Math.min(2000 * Math.pow(2, (terminalEntry.reconnectAttempts || 0)), 30000);
                terminalEntry.reconnectAttempts = (terminalEntry.reconnectAttempts || 0) + 1;

                setTimeout(function() {
                    if (state.terminals[id] && !state.terminals[id].ws && terminalEntry.desiredConnected && isTerminalViewVisible() && isTerminalVisibleInActiveWorkspace(id)) {
                        openTerminalSocket(id, terminalEntry, true);
                    }
                }, delay);
            });
        };
    }

    function closeTerminal(id, skipServer, preserveWorkspaceAssignments) {
        const t = state.terminals[id];
        if (!t) {
            return;
        }

        if (t.scrollFlushTimer) {
            clearTimeout(t.scrollFlushTimer);
            t.scrollFlushTimer = null;
        }
        clearTerminalScrollTarget(t);
        if (t.ws) {
            t.ws.close();
        }
        if (t.scrollbarObserver) {
            t.scrollbarObserver.disconnect();
        }
        if (t.scrollbarSyncFrame) {
            window.cancelAnimationFrame(t.scrollbarSyncFrame);
            t.scrollbarSyncFrame = null;
        }
        if (state.terminalScrollbarDrag && state.terminalScrollbarDrag.terminalId === id) {
            state.terminalScrollbarDrag = null;
        }
        if (t.term) {
            t.term.dispose();
        }
        if (t.wrapper) {
            t.wrapper.remove();
        }

        delete state.terminals[id];

        if (!preserveWorkspaceAssignments) {
            state.workspaces.forEach(function(workspace) {
                workspace.terminalIds = workspace.terminalIds.map(function(terminalId) {
                    return terminalId === id ? '' : terminalId;
                });
            });
            reconcileWorkspacesAfterTerminalLoad();
        } else if (state.activeId === id) {
            state.activeId = null;
        }

        if (!skipServer) {
            fetchJSON('/api/terminals/' + id, { method: 'DELETE' }, { authRequired: true })
                .catch(function() {
                    return null;
                });
        }

        renderTabBar();
        renderActiveWorkspace();
        scheduleFitActiveTerminal();
    }

    function switchTab(id) {
        if (!getWorkspaceById(id)) {
            return;
        }
        state.activeWorkspaceId = id;
        ensureActiveTerminalVisible();
        saveWorkspaceState();
        renderTabBar();
        renderActiveWorkspace();
        syncWorkspaceFileBrowserDefaults('workspace-change');
        scheduleFitActiveTerminal();
    }

    function fitActiveTerminal() {
        const visibleIds = getVisibleTerminalIds();
        let active;

        if (!visibleIds.length) {
            return;
        }

        visibleIds.forEach(function(id) {
            try {
                state.terminals[id].fitAddon.fit();
                scheduleTerminalScrollbarSync(state.terminals[id]);
            } catch (_) {
                // Ignore transient fit errors while the layout is settling.
            }
        });

        active = getActiveTerminal();
        if (active && document.activeElement !== editorTextarea) {
            try {
                active.term.focus();
            } catch (_) {
                // Ignore focus failures if xterm is still mounting.
            }
        }
    }

    function scheduleFitActiveTerminal() {
        clearTimeout(fitTimer);
        fitTimer = setTimeout(fitActiveTerminal, 40);
    }

    function sendResize(ws, rows, cols) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'resize', rows: rows, cols: cols }));
        }
    }

    function updateLayoutSwitchButtons() {
        var activeWorkspace = getActiveWorkspace();

        ['1', '2', '4', '4w'].forEach(function(layout) {
            var button = document.getElementById('layout-btn-' + layout);
            if (!button) {
                return;
            }
            button.classList.toggle('active', Boolean(activeWorkspace) && activeWorkspace.layout === layout);
        });
    }

    function buildTerminalPanePlaceholder(terminalId, isDuplicate) {
        var placeholder = document.createElement('div');
        var title = document.createElement('div');
        var copy = document.createElement('div');

        placeholder.className = 'terminal-pane-placeholder';
        title.className = 'terminal-pane-placeholder-title';
        copy.className = 'terminal-pane-placeholder-copy';
        title.textContent = t('workspace.emptySlot');
        copy.textContent = isDuplicate ? t('workspace.duplicateHint') : t('workspace.emptyHint');

        if (terminalId && state.terminals[terminalId] && isDuplicate) {
            title.textContent = state.terminals[terminalId].name;
        }

        placeholder.appendChild(title);
        placeholder.appendChild(copy);
        return placeholder;
    }

    function renderActiveWorkspace() {
        var activeWorkspace = getActiveWorkspace();
        var slotCount = activeWorkspace ? getWorkspaceSlotCount(activeWorkspace.layout) : 1;
        var mounted = {};

        if (!terminalGrid || !terminalDock) {
            return;
        }

        Object.keys(state.terminals).forEach(function(id) {
            var entry = state.terminals[id];

            if (entry && entry.wrapper && entry.wrapper.parentNode !== terminalDock) {
                terminalDock.appendChild(entry.wrapper);
            }
        });

        terminalGrid.innerHTML = '';
        terminalGrid.dataset.layout = activeWorkspace ? activeWorkspace.layout : '1';

        if (!activeWorkspace) {
            updateLayoutSwitchButtons();
            return;
        }

        for (var index = 0; index < slotCount; index += 1) {
            (function(slotIndex) {
                var terminalId = activeWorkspace.terminalIds[slotIndex] || '';
                var pane = document.createElement('section');
                var toolbar = document.createElement('div');
                var badge = document.createElement('span');
                var select = document.createElement('select');
                var actions = document.createElement('div');
                var copyBtn = document.createElement('button');
                var renameBtn = document.createElement('button');
                var closeBtn = document.createElement('button');
                var host = document.createElement('div');
                var option;

                pane.className = 'terminal-pane';
                pane.dataset.slotIndex = String(slotIndex);

                toolbar.className = 'terminal-pane-toolbar';
                badge.className = 'terminal-pane-badge';
                badge.textContent = String(slotIndex + 1);
                toolbar.appendChild(badge);

                select.className = 'terminal-pane-select';
                select.setAttribute('aria-label', t('workspace.selectTerminal'));

                option = document.createElement('option');
                option.value = '';
                option.textContent = t('workspace.selectTerminal');
                select.appendChild(option);

                Object.keys(state.terminals).sort(function(left, right) {
                    return state.terminals[left].name.localeCompare(state.terminals[right].name);
                }).forEach(function(id) {
                    var terminalOption = document.createElement('option');
                    terminalOption.value = id;
                    terminalOption.textContent = state.terminals[id].name;
                    select.appendChild(terminalOption);
                });

                option = document.createElement('option');
                option.value = '__create__';
                option.textContent = '+ ' + t('workspace.createHere');
                select.appendChild(option);
                select.value = terminalId && state.terminals[terminalId] ? terminalId : '';
                select.onchange = function(event) {
                    var nextValue = event.target.value;

                    if (nextValue === '__create__') {
                        event.target.value = terminalId && state.terminals[terminalId] ? terminalId : '';
                        createTerminalForWorkspaceSlot(activeWorkspace.id, slotIndex);
                        return;
                    }

                    assignTerminalToWorkspaceSlot(activeWorkspace.id, slotIndex, nextValue);
                };
                toolbar.appendChild(select);

                actions.className = 'terminal-pane-actions';

                copyBtn.className = 'terminal-pane-action';
                copyBtn.type = 'button';
                copyBtn.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="9" width="10" height="10" rx="2"></rect><path d="M15 9V7a2 2 0 0 0-2-2H7a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2"></path></svg>';
                copyBtn.title = t('terminal.copySelection');
                copyBtn.setAttribute('aria-label', t('terminal.copySelection'));
                copyBtn.disabled = !terminalId || !state.terminals[terminalId];
                copyBtn.onpointerdown = function(event) {
                    event.preventDefault();
                    event.stopPropagation();
                };
                copyBtn.onclick = function(event) {
                    event.stopPropagation();
                    if (terminalId && state.terminals[terminalId]) {
                        state.activeId = terminalId;
                        syncActiveTerminalPane();
                        copyTerminalSelection(state.terminals[terminalId].term, false);
                    }
                };
                actions.appendChild(copyBtn);

                renameBtn.className = 'terminal-pane-action';
                renameBtn.type = 'button';
                renameBtn.textContent = '\u270E';
                renameBtn.title = t('workspace.renameTerminal');
                renameBtn.disabled = !terminalId || !state.terminals[terminalId];
                renameBtn.onclick = function(event) {
                    event.stopPropagation();
                    if (terminalId && state.terminals[terminalId]) {
                        renameTerminal(terminalId);
                    }
                };
                actions.appendChild(renameBtn);

                closeBtn.className = 'terminal-pane-action danger';
                closeBtn.type = 'button';
                closeBtn.textContent = '\u00d7';
                closeBtn.title = t('workspace.closeTerminal');
                closeBtn.disabled = !terminalId || !state.terminals[terminalId];
                closeBtn.onclick = function(event) {
                    event.stopPropagation();
                    if (terminalId && state.terminals[terminalId]) {
                        closeTerminal(terminalId);
                    }
                };
                actions.appendChild(closeBtn);

                toolbar.appendChild(actions);
                pane.appendChild(toolbar);

                host.className = 'terminal-pane-host';
                pane.appendChild(host);

                if (terminalId && state.terminals[terminalId] && !mounted[terminalId]) {
                    mounted[terminalId] = true;
                    host.appendChild(state.terminals[terminalId].wrapper);
                } else {
                    host.appendChild(buildTerminalPanePlaceholder(terminalId, Boolean(terminalId && mounted[terminalId])));
                }

                pane.addEventListener('pointerdown', function() {
                    if (pane.dataset.terminalId) {
                        state.activeId = pane.dataset.terminalId;
                        syncActiveTerminalPane();
                    }
                });

                terminalGrid.appendChild(pane);
            }(index));
        }

        ensureActiveTerminalVisible();
        syncActiveTerminalPane();
        updateLayoutSwitchButtons();
        syncWorkspaceFileBrowserDefaults('render');
        if (state.loadingTerminals) {
            return;
        }
        syncVisibleTerminalConnections();
        scheduleFitActiveTerminal();
    }

    function clearWorkspaceTabDragHighlights() {
        const bar = document.getElementById('tab-bar');
        if (!bar) {
            return;
        }

        bar.querySelectorAll('.tab').forEach(function(tab) {
            tab.classList.remove('is-drag-over');
            tab.classList.remove('is-drag-over-end');
        });
    }

    function clearWorkspaceTabDragState(pointerId) {
        const sourceTab = workspaceTabDrag && workspaceTabDrag.sourceTab ? workspaceTabDrag.sourceTab : null;

        clearWorkspaceTabDragHighlights();
        document.body.style.userSelect = '';
        document.body.style.cursor = '';

        if (sourceTab) {
            sourceTab.classList.remove('is-dragging');
            sourceTab.style.zIndex = '';
            sourceTab.style.position = '';
            if (typeof pointerId === 'number' && sourceTab.releasePointerCapture) {
                try {
                    if (!sourceTab.hasPointerCapture || sourceTab.hasPointerCapture(pointerId)) {
                        sourceTab.releasePointerCapture(pointerId);
                    }
                } catch (_) {
                    // Ignore browsers that reject release after DOM churn.
                }
            }
        }

        const bar = document.getElementById('tab-bar');
        if (bar) {
            bar.querySelectorAll('.tab').forEach(function(tab) {
                tab.style.removeProperty('--tab-shift-x');
                tab.style.removeProperty('--tab-lift-y');
                tab.style.removeProperty('--tab-scale');
                tab.style.removeProperty('--tab-rotate');
            });
        }

        workspaceTabDrag = null;
    }

    function canStartWorkspaceTabDrag() {
        return state.workspaces.length > 1;
    }

    function isWorkspaceTabActionTarget(target) {
        return target && target.closest && target.closest('.tab-action');
    }

    function clampWorkspaceTabDropIndex(index) {
        if (typeof index !== 'number' || Number.isNaN(index)) {
            return null;
        }
        if (index < 0) {
            return 0;
        }
        if (state.workspaces.length <= 1) {
            return 0;
        }
        if (index >= state.workspaces.length) {
            return state.workspaces.length;
        }
        return index;
    }

    function resolveWorkspaceTabTargetIndex(dropIndex) {
        var normalizedDropIndex;
        var targetIndex;

        if (workspaceTabDrag === null || typeof dropIndex !== 'number' || Number.isNaN(dropIndex)) {
            return null;
        }

        normalizedDropIndex = clampWorkspaceTabDropIndex(dropIndex);
        if (typeof normalizedDropIndex !== 'number') {
            return null;
        }

        targetIndex = normalizedDropIndex > workspaceTabDrag.sourceIndex
            ? normalizedDropIndex - 1
            : normalizedDropIndex;

        if (targetIndex < 0) {
            targetIndex = 0;
        }
        if (targetIndex >= state.workspaces.length) {
            targetIndex = state.workspaces.length - 1;
        }

        return targetIndex === workspaceTabDrag.sourceIndex ? null : targetIndex;
    }

    function resolveWorkspaceTabDropIndexFromPointer(x) {
        const bar = document.getElementById('tab-bar');
        if (!bar) {
            return null;
        }

        const tabs = Array.from(bar.querySelectorAll('.tab'));
        const visibleTabs = tabs.filter(function(tab) {
            return tab.offsetParent !== null;
        });

        if (!visibleTabs.length) {
            return null;
        }

        const sourceTab = workspaceTabDrag ? workspaceTabDrag.sourceTab : null;

        for (let i = 0; i < visibleTabs.length; i += 1) {
            const tab = visibleTabs[i];
            // Skip the dragged tab — its getBoundingClientRect() is shifted
            // by the CSS transform and would block drop index advancement
            // when dragging left-to-right.
            if (tab === sourceTab) {
                continue;
            }
            const rect = tab.getBoundingClientRect();
            const midpoint = rect.left + rect.width / 2;

            if (x < midpoint) {
                return i;
            }
        }

        return visibleTabs.length;
    }

    function applyWorkspaceTabDragHint(dropIndex) {
        const bar = document.getElementById('tab-bar');
        if (!bar || workspaceTabDrag === null) {
            return;
        }

        clearWorkspaceTabDragHighlights();
        if (typeof dropIndex !== 'number' || Number.isNaN(dropIndex) || resolveWorkspaceTabTargetIndex(dropIndex) === null) {
            return;
        }

        const tabs = Array.from(bar.querySelectorAll('.tab')).filter(function(tab) {
            return tab.offsetParent !== null;
        });
        if (!tabs.length) {
            return;
        }

        if (dropIndex >= tabs.length) {
            if (workspaceTabDrag.sourceId !== tabs[tabs.length - 1].dataset.workspaceId) {
                tabs[tabs.length - 1].classList.add('is-drag-over-end');
            }
            return;
        }

        tabs.forEach(function(tab, index) {
            if (index === dropIndex && workspaceTabDrag.sourceId !== tab.dataset.workspaceId) {
                tab.classList.add('is-drag-over');
            }
        });
    }

    function applyWorkspaceTabReflow(dropIndex) {
        const bar = document.getElementById('tab-bar');
        if (!bar || workspaceTabDrag === null) {
            return;
        }

        const tabs = Array.from(bar.querySelectorAll('.tab')).filter(function(tab) {
            return tab.offsetParent !== null;
        });
        const sourceTab = workspaceTabDrag.sourceTab;
        const sourceIndex = workspaceTabDrag.sourceIndex;
        const sourceWidth = workspaceTabDrag.tabWidth || (sourceTab ? sourceTab.offsetWidth : 0);

        tabs.forEach(function(tab) {
            if (tab !== sourceTab) {
                tab.style.setProperty('--tab-shift-x', '0px');
                tab.style.setProperty('--tab-lift-y', '0px');
                tab.style.setProperty('--tab-scale', '1');
            }
        });

        if (typeof dropIndex !== 'number' || Number.isNaN(dropIndex) || !sourceWidth || resolveWorkspaceTabTargetIndex(dropIndex) === null) {
            return;
        }

        tabs.forEach(function(tab, index) {
            var shift = 0;

            if (tab === sourceTab) {
                return;
            }

            if (dropIndex > sourceIndex && index > sourceIndex && index < dropIndex) {
                shift = -sourceWidth;
            } else if (dropIndex < sourceIndex && index >= dropIndex && index < sourceIndex) {
                shift = sourceWidth;
            }

            tab.style.setProperty('--tab-shift-x', shift + 'px');
            tab.style.setProperty('--tab-lift-y', shift === 0 ? '0px' : '-2px');
            tab.style.setProperty('--tab-scale', shift === 0 ? '1' : '1.015');
        });
    }

    function beginWorkspaceTabDrag(event, workspaceId, index, tab) {
        if (!canStartWorkspaceTabDrag() || event.pointerType === 'touch' || isWorkspaceTabActionTarget(event.target) || (event.button !== undefined && event.button !== 0 && event.button !== -1)) {
            return;
        }

        var tabRect = tab ? tab.getBoundingClientRect() : null;

        workspaceTabDrag = {
            sourceId: workspaceId,
            sourceIndex: index,
            pointerId: event.pointerId,
            startX: event.clientX,
            startY: event.clientY,
            started: false,
            dropIndex: index,
            sourceTab: tab || null,
            tabWidth: tabRect ? tabRect.width : 0,
            tabStartLeft: tabRect ? tabRect.left : 0,
            offsetX: tabRect ? (event.clientX - tabRect.left) : 0
        };

        workspaceTabDragIgnoreClickId = null;
    }

    function handleWorkspaceTabDrag(event) {
        if (!workspaceTabDrag || workspaceTabDrag.pointerId !== event.pointerId) {
            return;
        }

        var rawDx = event.clientX - workspaceTabDrag.startX;

        if (!workspaceTabDrag.started) {
            if (Math.abs(rawDx) < 6) {
                return;
            }
            workspaceTabDrag.started = true;
            document.body.style.userSelect = 'none';
            var sourceTab = workspaceTabDrag.sourceTab;
            if (sourceTab) {
                sourceTab.classList.add('is-dragging');
                sourceTab.style.zIndex = '100';
                sourceTab.style.position = 'relative';
                if (sourceTab.setPointerCapture) {
                    try {
                        sourceTab.setPointerCapture(event.pointerId);
                    } catch (_) {
                        // Ignore browsers that do not allow capture.
                    }
                }
            }
            document.body.style.cursor = 'grabbing';
        }

        var dx = rawDx;
        var distance = Math.abs(dx);
        var sourceTab = workspaceTabDrag.sourceTab;
        if (sourceTab) {
            sourceTab.style.setProperty('--tab-lift-y', '-' + Math.min(6, 2 + distance * 0.018).toFixed(2) + 'px');
            sourceTab.style.setProperty('--tab-scale', Math.max(0.982, 0.995 - distance * 0.00012).toFixed(3));
            sourceTab.style.setProperty('--tab-shift-x', dx + 'px');
            sourceTab.style.removeProperty('--tab-rotate');
        }

        var dragCenterX = workspaceTabDrag.startX
            + dx
            + ((workspaceTabDrag.tabWidth || 0) / 2 - (workspaceTabDrag.offsetX || 0));
        var dropIndex = resolveWorkspaceTabDropIndexFromPointer(dragCenterX);
        dropIndex = clampWorkspaceTabDropIndex(dropIndex);
        workspaceTabDrag.dropIndex = typeof dropIndex === 'number' ? dropIndex : null;
        applyWorkspaceTabReflow(dropIndex);
        applyWorkspaceTabDragHint(dropIndex);
        event.preventDefault();
    }

    function endWorkspaceTabDrag(event) {
        var dragged = workspaceTabDrag;
        var targetIndex = null;

        if (!dragged || dragged.pointerId !== event.pointerId) {
            return;
        }

        if (dragged.started && typeof dragged.dropIndex === 'number') {
            targetIndex = resolveWorkspaceTabTargetIndex(dragged.dropIndex);
        }

        var willReorder = dragged.started
            && typeof targetIndex === 'number'
            && targetIndex !== dragged.sourceIndex;

        clearWorkspaceTabDragState(event.pointerId);
        workspaceTabDragIgnoreClickId = willReorder ? dragged.sourceId : null;

        if (willReorder) {
            moveWorkspaceTab(dragged.sourceIndex, targetIndex);
        }
    }

    function moveWorkspaceTab(fromIndex, toIndex) {
        var workspaces;
        var movedWorkspace;

        if (fromIndex === toIndex) {
            return false;
        }

        if (fromIndex < 0 || fromIndex >= state.workspaces.length || toIndex < 0 || toIndex >= state.workspaces.length) {
            return false;
        }

        workspaces = state.workspaces.slice();
        movedWorkspace = workspaces.splice(fromIndex, 1)[0];
        if (!movedWorkspace) {
            return false;
        }

        workspaces.splice(toIndex, 0, movedWorkspace);
        state.workspaces = workspaces;
        saveWorkspaceState();
        renderTabBar();
        renderActiveWorkspace();

        return true;
    }

    function focusWorkspaceTabById(workspaceId) {
        var bar = document.getElementById('tab-bar');

        if (!bar) {
            return;
        }

        Array.from(bar.querySelectorAll('.tab-main')).some(function(tab) {
            if (tab.dataset.workspaceId === workspaceId) {
                tab.focus();
                return true;
            }
            return false;
        });
    }

    function getWorkspaceTabScrollStep(bar) {
        return Math.max(120, Math.floor((bar ? bar.clientWidth : 0) * 0.75));
    }

    function getWorkspaceTabElementById(workspaceId) {
        var bar = document.getElementById('tab-bar');

        if (!bar) {
            return null;
        }

        return Array.from(bar.querySelectorAll('.tab')).find(function(tab) {
            return tab.dataset.workspaceId === workspaceId;
        }) || null;
    }

    function updateWorkspaceTabScrollButtons() {
        var bar = document.getElementById('tab-bar');
        var left = document.getElementById('workspace-tabs-scroll-left');
        var right = document.getElementById('workspace-tabs-scroll-right');
        var maxScroll;
        var canScroll;

        if (!bar || !left || !right) {
            return;
        }

        maxScroll = Math.max(0, bar.scrollWidth - bar.clientWidth);
        canScroll = maxScroll > 2;
        left.hidden = !canScroll;
        right.hidden = !canScroll;
        left.disabled = !canScroll || bar.scrollLeft <= 1;
        right.disabled = !canScroll || bar.scrollLeft >= maxScroll - 1;
    }

    function ensureWorkspaceTabVisible(workspaceId) {
        var bar = document.getElementById('tab-bar');
        var tab = getWorkspaceTabElementById(workspaceId);
        var padding = 8;
        var tabLeft;
        var tabRight;
        var visibleLeft;
        var visibleRight;
        var nextLeft = null;

        if (!bar || !tab) {
            updateWorkspaceTabScrollButtons();
            return;
        }

        tabLeft = tab.offsetLeft;
        tabRight = tabLeft + tab.offsetWidth;
        visibleLeft = bar.scrollLeft;
        visibleRight = visibleLeft + bar.clientWidth;

        if (tabLeft < visibleLeft + padding) {
            nextLeft = Math.max(0, tabLeft - padding);
        } else if (tabRight > visibleRight - padding) {
            nextLeft = Math.min(bar.scrollWidth - bar.clientWidth, tabRight - bar.clientWidth + padding);
        }

        if (typeof nextLeft === 'number') {
            bar.scrollTo({
                left: nextLeft,
                behavior: 'smooth'
            });
        }
        updateWorkspaceTabScrollButtons();
    }

    window.scrollWorkspaceTabs = function(direction) {
        var bar = document.getElementById('tab-bar');
        var delta;

        if (!bar) {
            return;
        }

        delta = getWorkspaceTabScrollStep(bar) * (direction < 0 ? -1 : 1);
        bar.scrollBy({
            left: delta,
            behavior: 'smooth'
        });
        window.setTimeout(updateWorkspaceTabScrollButtons, 220);
    };

    function renderTabBar() {
        const bar = document.getElementById('tab-bar');
        const workspacePanel = document.getElementById('workspace');
        bar.innerHTML = '';
        bar.setAttribute('role', 'tablist');
        bar.setAttribute('aria-label', 'Workspace views');
        bar.setAttribute('aria-orientation', 'horizontal');
        bar.onscroll = updateWorkspaceTabScrollButtons;

        state.workspaces.forEach(function(workspace, index) {
            const isActive = workspace.id === state.activeWorkspaceId;
            const tab = document.createElement('div');
            const tabButton = document.createElement('button');
            tab.className = 'tab' + (isActive ? ' active' : '') + (canStartWorkspaceTabDrag() ? ' is-reorderable' : '');
            tab.dataset.workspaceId = workspace.id;
            tab.setAttribute('role', 'presentation');
            tabButton.type = 'button';
            tabButton.className = 'tab-main';
            tabButton.id = 'workspace-tab-' + workspace.id;
            tabButton.dataset.workspaceId = workspace.id;
            tabButton.tabIndex = isActive ? 0 : -1;
            tabButton.setAttribute('role', 'tab');
            tabButton.setAttribute('aria-selected', isActive ? 'true' : 'false');
            tabButton.setAttribute('aria-controls', 'workspace');
            tab.onpointerdown = function(event) {
                if (event.pointerType === 'mouse' && event.button !== 0) {
                    return;
                }
                beginWorkspaceTabDrag(event, workspace.id, index, tab);
            };
            tabButton.onkeydown = function(event) {
                var targetIndex = index;
                var targetWorkspace;

                if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    switchTab(workspace.id);
                    return;
                }
                if (event.key === 'ArrowRight') {
                    targetIndex = Math.min(state.workspaces.length - 1, index + 1);
                } else if (event.key === 'ArrowLeft') {
                    targetIndex = Math.max(0, index - 1);
                } else if (event.key === 'Home') {
                    targetIndex = 0;
                } else if (event.key === 'End') {
                    targetIndex = state.workspaces.length - 1;
                } else {
                    return;
                }

                event.preventDefault();
                targetWorkspace = state.workspaces[targetIndex];
                if (!targetWorkspace) {
                    return;
                }

                switchTab(targetWorkspace.id);
                window.requestAnimationFrame(function() {
                    focusWorkspaceTabById(targetWorkspace.id);
                });
            };
            tabButton.onclick = function(e) {
                if (workspaceTabDragIgnoreClickId === workspace.id) {
                    workspaceTabDragIgnoreClickId = null;
                    return;
                }
                switchTab(workspace.id);
            };

            const name = document.createElement('span');
            name.className = 'tab-name';
            name.textContent = getWorkspaceLabel(workspace, index);
            name.ondblclick = function(e) {
                e.stopPropagation();
                renameWorkspace(workspace.id);
            };
            tabButton.appendChild(name);
            tab.appendChild(tabButton);

            const actions = document.createElement('span');
            actions.className = 'tab-actions';

            const rename = document.createElement('button');
            rename.type = 'button';
            rename.className = 'tab-action tab-rename';
            rename.textContent = '\u270E';
            rename.title = t('workspace.renameView');
            rename.setAttribute('aria-label', t('workspace.renameView'));
            rename.onclick = function(e) {
                e.stopPropagation();
                renameWorkspace(workspace.id);
            };
            actions.appendChild(rename);

            const close = document.createElement('button');
            close.type = 'button';
            close.className = 'tab-action tab-close';
            close.textContent = '\u00d7';
            close.title = t('workspace.closeView');
            close.setAttribute('aria-label', t('workspace.closeView'));
            close.onclick = function(e) {
                e.stopPropagation();
                closeWorkspaceTab(workspace.id);
            };
            actions.appendChild(close);

            tab.appendChild(actions);

            bar.appendChild(tab);
        });

        if (workspacePanel) {
            const activeTab = bar.querySelector('.tab.active .tab-main');
            workspacePanel.setAttribute('role', 'tabpanel');
            if (activeTab && activeTab.id) {
                workspacePanel.setAttribute('aria-labelledby', activeTab.id);
            } else {
                workspacePanel.removeAttribute('aria-labelledby');
            }
        }

        updateLayoutSwitchButtons();
        window.requestAnimationFrame(function() {
            ensureWorkspaceTabVisible(state.activeWorkspaceId);
        });
    }

    function renameWorkspace(id) {
        var workspace = getWorkspaceById(id);
        var index;
        var currentLabel;
        var nextName;
        var trimmed;

        if (!workspace) {
            return;
        }

        index = state.workspaces.findIndex(function(item) {
            return item.id === id;
        });
        currentLabel = getWorkspaceLabel(workspace, Math.max(index, 0));
        nextName = prompt(t('workspace.renamePrompt'), workspace.name || currentLabel);

        if (nextName === null) {
            return;
        }

        trimmed = nextName.trim();
        if (trimmed === workspace.name) {
            return;
        }

        workspace.name = trimmed;
        saveWorkspaceState();
        renderTabBar();
    }

    function renameTerminal(id) {
        const terminal = state.terminals[id];
        if (!terminal) {
            return;
        }

        const nextName = prompt(t('terminal.renamePrompt'), terminal.name);
        if (nextName === null) {
            return;
        }

        const trimmed = nextName.trim();
        if (!trimmed || trimmed === terminal.name) {
            return;
        }

        fetchJSON('/api/terminals/' + encodeURIComponent(id) + '/rename', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: trimmed })
        }, { authRequired: true })
            .then(function() {
                terminal.name = trimmed;
                renderTabBar();
                renderActiveWorkspace();
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(t('terminal.renameFailed', { message: err.message }));
                }
            });
    }

    // ========== Extra Keys ==========

    window.sendKey = function(key) {
        const terminal = getActiveTerminal();
        if (!terminal) {
            return;
        }

        const keyMap = {
            tab: '\t',
            esc: '\x1b',
            pipe: '|',
            tilde: '~',
            slash: '/',
            dash: '-',
            up: '\x1b[A',
            down: '\x1b[B',
            left: '\x1b[D',
            right: '\x1b[C'
        };

        const seq = keyMap[key];
        if (seq && sendTerminalSequence(terminal, seq)) {
            terminal.term.focus();
        }
    };

    window.toggleCtrl = function() {
        setCtrlActive(!state.ctrlActive);
    };

    // ========== File Browser ==========

    function setFileDropActive(active) {
        state.fileDropActive = Boolean(active && state.fileBrowserOpen && !state.upload.active);
        filePanel.classList.toggle('drop-active', state.fileDropActive);
        if (fileDropzone) {
            fileDropzone.setAttribute('aria-hidden', state.fileDropActive ? 'false' : 'true');
        }
    }

    function eventCarriesFiles(event) {
        const dataTransfer = event && event.dataTransfer;
        const types = dataTransfer && dataTransfer.types ? Array.from(dataTransfer.types) : [];
        return types.indexOf('Files') !== -1;
    }

    function handleFilePanelDragEnter(event) {
        if (!eventCarriesFiles(event) || !state.fileBrowserOpen || state.upload.active) {
            return;
        }
        event.preventDefault();
        fileDragDepth += 1;
        setFileDropActive(true);
    }

    function handleFilePanelDragOver(event) {
        if (!eventCarriesFiles(event) || !state.fileBrowserOpen || state.upload.active) {
            return;
        }
        event.preventDefault();
        if (event.dataTransfer) {
            event.dataTransfer.dropEffect = 'copy';
        }
        setFileDropActive(true);
    }

    function handleFilePanelDragLeave(event) {
        if (!state.fileBrowserOpen || !state.fileDropActive) {
            return;
        }
        event.preventDefault();
        if (event.relatedTarget && filePanel.contains(event.relatedTarget)) {
            return;
        }
        fileDragDepth = Math.max(0, fileDragDepth - 1);
        if (fileDragDepth === 0) {
            setFileDropActive(false);
        }
    }

    function handleFilePanelDrop(event) {
        let files;

        if (!state.fileBrowserOpen || !eventCarriesFiles(event)) {
            return;
        }

        event.preventDefault();
        fileDragDepth = 0;
        setFileDropActive(false);
        files = Array.from((event.dataTransfer && event.dataTransfer.files) || []);
        if (!files.length || state.upload.active) {
            return;
        }

        uploadFiles(files)
            .then(function() {
                window.refreshFiles();
            })
            .catch(function() {
                return null;
            });
    }

    function handleFileFilterInput(event) {
        state.fileFilterQuery = event.target.value || '';
        renderFileList(state.files);
    }

    function handleFileFilterKeyDown(event) {
        if (event.key !== 'Escape') {
            return;
        }

        if (!state.fileFilterQuery) {
            event.stopPropagation();
            return;
        }

        event.preventDefault();
        event.stopPropagation();
        state.fileFilterQuery = '';
        fileFilterInput.value = '';
        renderFileList(state.files);
    }

    function getBrowserPathContext() {
        const isViewer = state.fileBrowserTab === 'viewer';

        if (isViewer && state.previewImagePath) {
            return {
                path: state.previewImagePath,
                leafIsFile: true
            };
        }

        return {
            path: state.currentPath || '~',
            leafIsFile: false
        };
    }

    function buildPathBreadcrumbs(path) {
        const normalized = typeof path === 'string' && path.trim() ? path.trim() : '~';
        const isHome = normalized === '~' || normalized.indexOf('~/') === 0;
        const root = isHome ? '~' : '/';
        const remainder = isHome
            ? normalized.slice(1).replace(/^\/+/, '')
            : normalized.replace(/^\/+/, '');
        const parts = remainder ? remainder.split('/').filter(Boolean) : [];
        const crumbs = [{ label: root, path: root }];
        let current = root;

        parts.forEach(function(part) {
            current = joinPath(current, part);
            crumbs.push({
                label: part,
                path: current
            });
        });

        return crumbs;
    }

    function renderFileBreadcrumbs(path, leafIsFile) {
        const breadcrumbs = buildPathBreadcrumbs(path);

        if (!filePathLabel) {
            return;
        }

        filePathLabel.innerHTML = '';

        breadcrumbs.forEach(function(crumb, index) {
            const isLast = index === breadcrumbs.length - 1;
            let node;

            if (index > 0) {
                const separator = document.createElement('span');
                separator.className = 'browser-breadcrumb-sep';
                separator.textContent = '/';
                filePathLabel.appendChild(separator);
            }

            if (isLast) {
                node = document.createElement('span');
                node.className = 'browser-breadcrumb-current';
            } else {
                node = document.createElement('button');
                node.type = 'button';
                node.className = 'browser-breadcrumb';
                node.onclick = function() {
                    setFileBrowserTab('files');
                    loadDirectory(crumb.path);
                };
            }

            if (leafIsFile && isLast) {
                node.className = 'browser-breadcrumb-current';
            }

            node.textContent = crumb.label;
            node.title = crumb.path;
            filePathLabel.appendChild(node);
        });
    }

    function getNormalizedFileFilterQuery() {
        return String(state.fileFilterQuery || '').trim().toLocaleLowerCase();
    }

    function filterFilesForDisplay(files) {
        const query = getNormalizedFileFilterQuery();
        const visibleFiles = state.showHidden
            ? files
            : files.filter(function(file) {
                return !file.name.startsWith('.');
            });

        return {
            visibleFiles: visibleFiles,
            filteredFiles: query
                ? visibleFiles.filter(function(file) {
                    return String(file.name || '').toLocaleLowerCase().indexOf(query) !== -1;
                })
                : visibleFiles,
            query: query
        };
    }

    function updateFileFilterSummary(filteredCount, visibleCount, query) {
        if (!fileFilterSummary) {
            return;
        }

        if (state.fileBrowserTab === 'viewer') {
            fileFilterSummary.textContent = '';
            return;
        }

        fileFilterSummary.textContent = query && visibleCount > 0
            ? (String(filteredCount) + ' / ' + String(visibleCount))
            : '';
    }

    function getFilePath(file) {
        return joinPath(state.currentPath || '~', file.name);
    }

    function isFilePathSelected(path) {
        return state.selectedFilePaths.indexOf(path) !== -1;
    }

    function getDisplayedFiles() {
        return sortFiles(filterFilesForDisplay(state.files).filteredFiles);
    }

    function getSelectedFileEntries() {
        return state.selectedFilePaths.map(function(path) {
            return findVisibleFileForPath(path);
        }).filter(Boolean);
    }

    function syncSelectedFilePaths() {
        state.selectedFilePaths = state.selectedFilePaths.filter(function(path) {
            return Boolean(findVisibleFileForPath(path));
        });
    }

    function updateFileSelectionUI() {
        syncSelectedFilePaths();

        const isViewer = state.fileBrowserTab === 'viewer';
        const selectionVisible = state.fileSelectionMode && !isViewer;
        const selectedEntries = getSelectedFileEntries();
        const displayedFiles = getDisplayedFiles();
        const selectedCount = selectedEntries.length;
        const allDisplayedSelected = displayedFiles.length > 0 && displayedFiles.every(function(file) {
            return isFilePathSelected(getFilePath(file));
        });
        const canDownload = selectedCount === 1 && !selectedEntries[0].isDir;

        if (fileToolbar) {
            fileToolbar.style.display = isViewer || selectionVisible ? 'none' : 'flex';
        }
        if (fileSelectionBar) {
            fileSelectionBar.style.display = selectionVisible ? 'flex' : 'none';
        }
        if (fileSelectModeBtn) {
            fileSelectModeBtn.classList.toggle('active', state.fileSelectionMode);
        }
        if (fileSelectionSummary) {
            fileSelectionSummary.textContent = selectedCount > 0
                ? t('files.selectionSummary', { count: selectedCount })
                : t('files.selectionReady');
        }
        if (fileSelectionAllBtn) {
            fileSelectionAllBtn.disabled = displayedFiles.length === 0 || allDisplayedSelected;
        }
        if (fileSelectionCopyBtn) {
            fileSelectionCopyBtn.disabled = selectedCount !== 1;
        }
        if (fileSelectionMoveBtn) {
            fileSelectionMoveBtn.disabled = selectedCount === 0;
        }
        if (fileSelectionRenameBtn) {
            fileSelectionRenameBtn.disabled = selectedCount !== 1;
        }
        if (fileSelectionDownloadBtn) {
            fileSelectionDownloadBtn.disabled = !canDownload;
        }
        if (fileSelectionDeleteBtn) {
            fileSelectionDeleteBtn.disabled = selectedCount === 0;
        }
    }

    function resetFileSelection(options) {
        const config = options || {};

        state.selectedFilePaths = [];
        if (!config.keepMode) {
            state.fileSelectionMode = false;
        }
        updateFileSelectionUI();
    }

    function setFileSelection(path, selected) {
        const nextSelected = Boolean(selected);

        if (!path) {
            return;
        }

        if (nextSelected) {
            if (!isFilePathSelected(path)) {
                state.selectedFilePaths.push(path);
            }
        } else {
            state.selectedFilePaths = state.selectedFilePaths.filter(function(item) {
                return item !== path;
            });
        }

        syncSelectedFilePaths();
        updateFileSelectionUI();
    }

    function appendHighlightedFileName(container, name, query) {
        const displayName = String(name || '');
        const normalizedName = displayName.toLocaleLowerCase();
        const matchIndex = query ? normalizedName.indexOf(query) : -1;

        container.textContent = '';

        if (!query || matchIndex === -1) {
            container.textContent = displayName;
            return;
        }

        if (matchIndex > 0) {
            container.appendChild(document.createTextNode(displayName.slice(0, matchIndex)));
        }

        const match = document.createElement('span');
        match.className = 'file-name-match';
        match.textContent = displayName.slice(matchIndex, matchIndex + query.length);
        container.appendChild(match);

        if (matchIndex + query.length < displayName.length) {
            container.appendChild(document.createTextNode(displayName.slice(matchIndex + query.length)));
        }
    }

    window.toggleFileBrowser = function() {
        setFileBrowserOpen(!state.fileBrowserOpen, null, { userInitiated: true });
    };

    window.switchFileBrowserTab = function(tab) {
        if (!state.fileBrowserOpen) {
            setFileBrowserOpen(true, tab, { userInitiated: true });
            return;
        }
        setFileBrowserTab(tab);
    };

    function isDesktopWorkspaceViewport() {
        return Boolean(window.matchMedia && window.matchMedia('(min-width: 981px)').matches);
    }

    function isSinglePaneDesktopWorkspace(workspaceRecord) {
        return Boolean(workspaceRecord && workspaceRecord.layout === '1' && isDesktopWorkspaceViewport());
    }

    function shouldEmbedFileBrowser() {
        return Boolean(state.fileBrowserOpen && isSinglePaneDesktopWorkspace(getActiveWorkspace()));
    }

    function getEmbeddedFileBrowserContainerWidth() {
        if (terminalContainer) {
            return terminalContainer.getBoundingClientRect().width || 0;
        }
        return window.innerWidth || document.documentElement.clientWidth || 0;
    }

    function getEmbeddedFileBrowserDefaultWidth() {
        return Math.round(Math.min(
            getEmbeddedFileBrowserContainerWidth() * EMBEDDED_FILE_PANEL_DEFAULT_WIDTH_RATIO,
            EMBEDDED_FILE_PANEL_DEFAULT_MAX_WIDTH
        ));
    }

    function getEmbeddedFileBrowserMinWidth() {
        return getEmbeddedFileBrowserDefaultWidth();
    }

    function getEmbeddedFileBrowserMaxWidth() {
        const minWidth = getEmbeddedFileBrowserMinWidth();

        return Math.max(minWidth, Math.round(Math.min(
            EMBEDDED_FILE_PANEL_MAX_WIDTH,
            getEmbeddedFileBrowserContainerWidth() - EMBEDDED_FILE_PANEL_MIN_TERMINAL_WIDTH
        )));
    }

    function clampEmbeddedFileBrowserWidth(width) {
        const minWidth = getEmbeddedFileBrowserMinWidth();
        const maxWidth = getEmbeddedFileBrowserMaxWidth();
        const nextWidth = Number(width);

        if (!Number.isFinite(nextWidth)) {
            return minWidth;
        }

        return Math.max(minWidth, Math.min(Math.round(nextWidth), maxWidth));
    }

    function syncEmbeddedFileBrowserWidth() {
        let minWidth;
        let width;

        if (!terminalContainer) {
            return;
        }

        if (!shouldEmbedFileBrowser()) {
            terminalContainer.style.removeProperty('--embedded-file-panel-width');
            terminalContainer.style.removeProperty('--embedded-file-panel-min-width');
            if (filePanelResizer) {
                filePanelResizer.classList.remove('disabled');
            }
            return;
        }

        minWidth = getEmbeddedFileBrowserMinWidth();
        width = clampEmbeddedFileBrowserWidth(
            Number.isFinite(state.embeddedFileBrowserWidth) && state.embeddedFileBrowserWidth > 0
                ? state.embeddedFileBrowserWidth
                : minWidth
        );

        terminalContainer.style.setProperty('--embedded-file-panel-min-width', minWidth + 'px');
        terminalContainer.style.setProperty('--embedded-file-panel-width', width + 'px');
        if (filePanelResizer) {
            filePanelResizer.classList.toggle('disabled', getEmbeddedFileBrowserMaxWidth() <= minWidth + 1);
        }
    }

    function syncFileBrowserContainerMode() {
        const embedded = shouldEmbedFileBrowser();

        if (terminalContainer) {
            terminalContainer.classList.toggle('file-browser-embedded', embedded);
        }
        filePanel.classList.toggle('open', state.fileBrowserOpen);
        fileOverlay.style.display = state.fileBrowserOpen && !embedded ? 'block' : 'none';
        syncEmbeddedFileBrowserWidth();
    }

    function syncWorkspaceFileBrowserDefaults(reason) {
        const activeWorkspace = getActiveWorkspace();
        const shouldDefaultOpen = isSinglePaneDesktopWorkspace(activeWorkspace);
        const wasEmbedded = Boolean(terminalContainer && terminalContainer.classList.contains('file-browser-embedded'));
        const shouldCloseForContext = reason === 'workspace-change' || reason === 'layout-change' || reason === 'terminal-load';

        if (shouldDefaultOpen) {
            if (!state.fileBrowserOpen && !state.fileBrowserSingleDesktopDismissed) {
                setFileBrowserOpen(true, 'files', { autoSingleDesktopDefault: true });
                return;
            }

            syncFileBrowserContainerMode();
            if (reason === 'resize') {
                scheduleFitActiveTerminal();
            }
            return;
        }

        state.fileBrowserSingleDesktopDismissed = false;
        if (state.fileBrowserOpen && (wasEmbedded || shouldCloseForContext)) {
            setFileBrowserOpen(false);
            return;
        }

        syncFileBrowserContainerMode();
    }

    function setFileBrowserOpen(open, preferredTab, options) {
        const activeWorkspace = getActiveWorkspace();
        const inSinglePaneDesktop = isSinglePaneDesktopWorkspace(activeWorkspace);
        const config = options || {};

        if (preferredTab) {
            state.fileBrowserTab = preferredTab === 'viewer' ? 'viewer' : 'files';
        }

        if (config.userInitiated) {
            if (open) {
                state.fileBrowserSingleDesktopDismissed = false;
            } else if (inSinglePaneDesktop) {
                state.fileBrowserSingleDesktopDismissed = true;
            }
        } else if (config.autoSingleDesktopDefault) {
            state.fileBrowserSingleDesktopDismissed = false;
        }

        if (!open && config.userInitiated && shouldEmbedViewerEditor()) {
            if (!closeEditor(state.previewImagePath)) {
                return;
            }
        }

        state.fileBrowserOpen = open;
        if (!open) {
            fileDragDepth = 0;
            setFileDropActive(false);
            resetFileSelection();
            if (state.previewEditMode) {
                state.previewEditMode = false;
                applyEditorLayout();
            }
        }
        syncFileBrowserContainerMode();
        syncFileBrowserLayout();
        updateHiddenToggle();
        scheduleFitActiveTerminal();

        if (open && state.fileBrowserTab === 'viewer' && state.viewerDraft && !state.previewImagePath) {
            focusViewerDraftPrimaryField();
        }

        if (open && !state.currentPath) {
            loadDirectory('~');
        }
    }

    function setFileBrowserTab(tab) {
        const nextTab = tab === 'viewer' ? 'viewer' : 'files';
        state.fileBrowserTab = nextTab;
        syncFileBrowserLayout();
        if (nextTab === 'viewer' && state.fileBrowserOpen && state.viewerDraft && !state.previewImagePath) {
            focusViewerDraftPrimaryField();
        }
    }

    function syncFileBrowserLayout() {
        const isViewer = state.fileBrowserTab === 'viewer';
        const pathContext = getBrowserPathContext();
        const filesTab = document.getElementById('browser-tab-files');
        const viewerTab = document.getElementById('browser-tab-viewer');

        if (filesTab) {
            filesTab.classList.toggle('active', !isViewer);
        }
        if (viewerTab) {
            viewerTab.classList.toggle('active', isViewer);
        }
        if (fileBrowserView) {
            fileBrowserView.style.display = isViewer ? 'none' : 'flex';
        }
        if (fileViewerView) {
            fileViewerView.style.display = isViewer ? 'flex' : 'none';
        }
        if (fileFilterWrap) {
            fileFilterWrap.style.display = isViewer ? 'none' : 'flex';
        }
        if (fileFilterInput && fileFilterInput.value !== state.fileFilterQuery) {
            fileFilterInput.value = state.fileFilterQuery;
        }
        renderFileBreadcrumbs(pathContext.path, pathContext.leafIsFile);

        renderViewerPanel();
        updateFileSelectionUI();
    }

    window.refreshFiles = function() {
        loadDirectory(state.currentPath || '~');
    };

    window.loadHomeDirectory = function() {
        loadDirectory('~');
    };

    window.navigateUp = function() {
        if (!state.currentPath || state.currentPath === '/' || state.currentPath === '~') {
            return;
        }
        loadDirectory(parentPath(state.currentPath));
    };

    function applyDirectoryListing(path, files) {
        state.currentPath = path;
        state.files = Array.isArray(files) ? files : [];
        resetFileSelection();
        syncFileBrowserLayout();
        renderFileList(state.files);
    }

    function loadDirectory(path) {
        fetchJSON('/api/files?path=' + encodeURIComponent(path), undefined, { authRequired: true })
            .then(files => {
                applyDirectoryListing(path, files);
            })
            .catch(err => {
                if (isHandledAuthError(err)) {
                    return;
                }
                resetFileSelection();
                renderFileList([]);
                showFileListError(err.message);
            });
    }

    function showFileListError(message) {
        const fileList = document.getElementById('file-list');
        const errorDiv = document.createElement('div');
        errorDiv.className = 'file-empty';
        errorDiv.style.color = 'var(--error)';
        errorDiv.textContent = message;
        fileList.innerHTML = '';
        fileList.appendChild(errorDiv);
    }

    function getFileExtensionLabel(name) {
        var trimmed = String(name || '').trim();
        var lastDot = trimmed.lastIndexOf('.');

        if (lastDot === -1) {
            return '';
        }

        return trimmed.slice(lastDot + 1).toLowerCase();
    }

    var FILE_TYPE_ICON_SVGS = {
        folder: '<svg viewBox="0 0 24 24"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path></svg>',
        file: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path></svg>',
        image: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path><circle cx="10" cy="10" r="1.4"></circle><path d="M8.5 17l3-3 2.2 2.2 2.3-2.3 2 2.1"></path></svg>',
        pdf: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path><path d="M9 12h6"></path><path d="M9 15h6"></path><path d="M9 18h4"></path></svg>',
        sheet: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path><path d="M8 10.5h8"></path><path d="M8 14.5h8"></path><path d="M12 10v8"></path><path d="M16 10v8"></path></svg>',
        slides: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path><rect x="8.5" y="10" width="7" height="5.5" rx="1"></rect><path d="M12 15.5v3"></path><path d="M9.5 18.5h5"></path></svg>',
        archive: '<svg viewBox="0 0 24 24"><path d="M5 9h14v9a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2z"></path><path d="M9 5h6v4H9z"></path><path d="M10 12h4"></path><path d="M12 9v6"></path></svg>',
        code: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path><path d="M10.5 13.5l-2 2 2 2"></path><path d="M14.5 13.5l2 2-2 2"></path><path d="M13 12.5l-2 6"></path></svg>',
        text: '<svg viewBox="0 0 24 24"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"></path><path d="M14 3v5h5"></path><path d="M9 11h6"></path><path d="M9 14h6"></path><path d="M9 17h4"></path></svg>'
    };

    function getFileTypeBadgeInfo(file) {
        var extension = getFileExtensionLabel(file && file.name);
        var tone = 'generic';

        if (file && file.isDir) {
            return {
                icon: 'folder',
                title: 'Folder',
                tone: 'folder'
            };
        }

        if (extension === 'pdf') {
            tone = 'pdf';
        } else if ([
            'xlsx', 'xls', 'csv', 'tsv', 'numbers'
        ].indexOf(extension) !== -1) {
            tone = 'sheet';
        } else if ([
            'ppt', 'pptx', 'key'
        ].indexOf(extension) !== -1) {
            tone = 'slides';
        } else if ([
            'png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'heic', 'bmp', 'avif'
        ].indexOf(extension) !== -1) {
            tone = 'image';
        } else if ([
            'zip', 'tar', 'gz', 'bz2', 'xz', '7z', 'rar'
        ].indexOf(extension) !== -1) {
            tone = 'archive';
        } else if ([
            'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'json', 'html', 'css', 'go', 'py', 'sh', 'sql', 'yml', 'yaml', 'toml', 'env'
        ].indexOf(extension) !== -1) {
            tone = 'code';
        } else if ([
            'doc', 'docx', 'txt', 'md', 'rtf', 'pages'
        ].indexOf(extension) !== -1) {
            tone = 'text';
        }

        return {
            icon: tone === 'generic' ? 'file' : tone,
            title: extension ? extension.toUpperCase() : 'File',
            tone: tone
        };
    }

    function createFileTypeBadge(file) {
        var info = getFileTypeBadgeInfo(file);
        var badge = document.createElement('span');

        badge.className = 'file-icon file-type-icon is-' + info.tone;
        badge.innerHTML = FILE_TYPE_ICON_SVGS[info.icon] || FILE_TYPE_ICON_SVGS.file;
        badge.setAttribute('aria-hidden', 'true');
        badge.title = info.title;
        return badge;
    }

    window.setFileSort = function(key) {
        var nextDirection = 'asc';

        if (state.fileSort.key === key) {
            nextDirection = state.fileSort.direction === 'asc' ? 'desc' : 'asc';
        }

        state.fileSort = { key: key, direction: nextDirection };
        updateFileSortControls();
        renderFileList(state.files);
    };

    function updateFileSortControls() {
        var buttons = {
            name: fileSortNameBtn,
            size: fileSortSizeBtn,
            modified: fileSortModifiedBtn
        };
        var indicators = {
            name: fileSortNameIndicator,
            size: fileSortSizeIndicator,
            modified: fileSortModifiedIndicator
        };

        Object.keys(buttons).forEach(function(key) {
            var button = buttons[key];
            var indicator = indicators[key];
            var isActive = state.fileSort.key === key;
            var directionKey = isActive && state.fileSort.direction === 'desc'
                ? 'files.sortDescending'
                : 'files.sortAscending';
            var labelKey = key === 'name'
                ? 'files.columnName'
                : (key === 'size' ? 'files.columnSize' : 'files.columnModified');
            var label = t(labelKey);
            var directionLabel = t(directionKey);
            var ariaLabel = t('files.sortBy', { column: label, direction: directionLabel });

            if (!button || !indicator) {
                return;
            }

            button.classList.toggle('active', isActive);
            button.setAttribute('aria-label', ariaLabel);
            button.title = ariaLabel;
            indicator.textContent = isActive
                ? (state.fileSort.direction === 'asc' ? '↑' : '↓')
                : '↕';
        });
    }

    function sortFiles(files) {
        return files.slice().sort(compareFilesForDisplay);
    }

    function compareFilesForDisplay(left, right) {
        var key = state.fileSort.key;
        var direction = state.fileSort.direction === 'desc' ? -1 : 1;
        var compareValue = 0;

        if (left.isDir !== right.isDir) {
            return left.isDir ? -1 : 1;
        }

        if (key === 'size') {
            compareValue = compareNumbers(left.isDir ? 0 : left.size, right.isDir ? 0 : right.size);
        } else if (key === 'modified') {
            compareValue = compareNumbers(left.modTime, right.modTime);
        } else {
            compareValue = compareText(left.name, right.name);
        }

        if (compareValue !== 0) {
            return compareValue * direction;
        }

        return compareText(left.name, right.name);
    }

    function compareNumbers(left, right) {
        return Number(left || 0) - Number(right || 0);
    }

    function compareText(left, right) {
        return String(left || '').localeCompare(String(right || ''), undefined, {
            numeric: true,
            sensitivity: 'base'
        });
    }

    function renderFileList(files) {
        const list = document.getElementById('file-list');
        const fileSet = filterFilesForDisplay(files);
        const sortedFiles = sortFiles(fileSet.filteredFiles);
        list.innerHTML = '';
        updateFileFilterSummary(sortedFiles.length, fileSet.visibleFiles.length, fileSet.query);

        if (sortedFiles.length === 0) {
            list.innerHTML = '<div class="file-empty">' + (fileSet.query && fileSet.visibleFiles.length > 0 ? t('files.noMatches') : t('files.noVisibleFiles')) + '</div>';
            return;
        }

        sortedFiles.forEach(function(file) {
            const item = document.createElement('div');
            item.className = 'file-item';

            const main = document.createElement('div');
            main.className = 'file-item-main';
            const path = getFilePath(file);
            const isSelected = isFilePathSelected(path);

            if (state.fileSelectionMode) {
                const checkbox = document.createElement('input');
                checkbox.type = 'checkbox';
                checkbox.className = 'file-item-checkbox';
                checkbox.checked = isSelected;
                checkbox.setAttribute('aria-label', file.name);
                checkbox.addEventListener('click', function(event) {
                    event.stopPropagation();
                });
                checkbox.addEventListener('change', function(event) {
                    setFileSelection(path, event.target.checked);
                    renderFileList(state.files);
                });
                main.appendChild(checkbox);
            }

            const name = document.createElement('span');
            name.className = 'file-name';
            appendHighlightedFileName(name, file.name, fileSet.query);

            main.appendChild(createFileTypeBadge(file));
            main.appendChild(name);

            const size = document.createElement('span');
            size.className = 'file-size';
            size.textContent = file.isDir ? '—' : formatSize(file.size);

            const modified = document.createElement('span');
            modified.className = 'file-modified';
            modified.textContent = formatFileModTime(file.modTime);

            item.appendChild(main);
            item.appendChild(size);
            item.appendChild(modified);
            item.onclick = function() {
                if (state.fileSelectionMode) {
                    setFileSelection(path, !isSelected);
                    renderFileList(state.files);
                    return;
                }
                handleFileSelection(file);
            };
            item.classList.toggle('is-selected', isSelected);

            list.appendChild(item);
        });

        updateFileSelectionUI();
    }

    window.toggleFileSelectionMode = function() {
        state.fileSelectionMode = !state.fileSelectionMode;
        state.selectedFilePaths = [];
        updateFileSelectionUI();
        renderFileList(state.files);
    };

    window.clearFileSelectionMode = function() {
        resetFileSelection();
        renderFileList(state.files);
    };

    window.selectAllFiles = function() {
        if (!state.fileSelectionMode) {
            return;
        }

        state.selectedFilePaths = getDisplayedFiles().map(function(file) {
            return getFilePath(file);
        });
        updateFileSelectionUI();
        renderFileList(state.files);
    };

    window.copySelectedFiles = function() {
        const selectedEntries = getSelectedFileEntries();

        if (selectedEntries.length !== 1) {
            return;
        }

        copyFile(selectedEntries[0]).then(function(success) {
            if (success) {
                resetFileSelection();
                renderFileList(state.files);
            }
        });
    };

    window.renameSelectedFiles = function() {
        const selectedEntries = getSelectedFileEntries();

        if (selectedEntries.length !== 1) {
            return;
        }

        renameFile(selectedEntries[0]).then(function(success) {
            if (success) {
                resetFileSelection();
                renderFileList(state.files);
            }
        });
    };

    window.moveSelectedFiles = function() {
        const selectedEntries = getSelectedFileEntries();
        let rawTargetDir;
        let targetDir;
        let movePlans;

        if (!selectedEntries.length) {
            return;
        }

        rawTargetDir = prompt(t('files.movePrompt'), state.currentPath || '~');
        if (rawTargetDir === null) {
            return;
        }

        targetDir = normalizePromptPath(rawTargetDir, state.currentPath || '~');
        if (!targetDir) {
            return;
        }

        movePlans = selectedEntries.map(function(file) {
            const oldPath = getFilePath(file);
            const newPath = joinPath(targetDir, file.name);
            return {
                file: file,
                oldPath: oldPath,
                newPath: newPath
            };
        }).filter(function(plan) {
            return plan.newPath !== plan.oldPath;
        });

        if (!movePlans.length) {
            return;
        }

        for (const plan of movePlans) {
            if (findEditor(plan.newPath) && plan.newPath !== plan.oldPath) {
                alert(t('editor.pathAlreadyOpen', { path: plan.newPath }));
                return;
            }

            if (isKnownDirectoryPath(plan.newPath, plan.oldPath)) {
                alert(t('files.cannotOverwriteDirectory', { path: plan.newPath }));
                return;
            }
        }

        movePlans.reduce(function(chain, plan) {
            return chain.then(function() {
                return fetchJSON('/api/files/rename', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ oldPath: plan.oldPath, newPath: plan.newPath })
                }, { authRequired: true }).then(function() {
                    syncPathConsumersAfterRename(plan.oldPath, plan.newPath);
                });
            });
        }, Promise.resolve())
            .then(function() {
                resetFileSelection();
                window.refreshFiles();
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(t('files.moveFailed', { message: err.message }));
                }
            });
    };

    window.downloadSelectedFiles = function() {
        const selectedEntries = getSelectedFileEntries();

        if (selectedEntries.length !== 1 || selectedEntries[0].isDir) {
            return;
        }

        downloadFile(selectedEntries[0]);
        resetFileSelection();
        renderFileList(state.files);
    };

    window.deleteSelectedFiles = function() {
        const selectedEntries = getSelectedFileEntries();

        if (!selectedEntries.length) {
            return;
        }

        if (selectedEntries.length === 1) {
            deleteFile(selectedEntries[0]).then(function(success) {
                if (success) {
                    resetFileSelection();
                    renderFileList(state.files);
                }
            });
            return;
        }

        if (!confirm(t('files.bulkDeleteConfirm', { count: selectedEntries.length }))) {
            return;
        }

        selectedEntries.reduce(function(chain, file) {
            return chain.then(function() {
                return fetchJSON('/api/files?path=' + encodeURIComponent(getFilePath(file)), { method: 'DELETE' }, { authRequired: true });
            });
        }, Promise.resolve())
            .then(function() {
                resetFileSelection();
                window.refreshFiles();
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(err.message);
                }
            });
    };

    function handleFileSelection(file) {
        if (file.isDir) {
            loadDirectory(joinPath(state.currentPath, file.name));
            return;
        }

        if (isPreviewableImage(file.name)) {
            previewImage(file);
            return;
        }

        if (isPreviewablePdf(file.name)) {
            previewPdf(file);
            return;
        }

        if (isPreviewablePptx(file.name)) {
            previewPptx(file);
            return;
        }

        if (isPreviewableDocx(file.name)) {
            previewDocx(file);
            return;
        }

        if (isPreviewableXlsx(file.name)) {
            previewXlsx(file);
            return;
        }

        previewTextFile(file);
    }

    function deleteFile(file) {
        if (!confirm(t('files.deleteConfirm', { name: file.name }))) {
            return Promise.resolve(false);
        }

        const path = joinPath(state.currentPath, file.name);
        return fetchJSON('/api/files?path=' + encodeURIComponent(path), { method: 'DELETE' }, { authRequired: true })
            .then(function() {
                window.refreshFiles();
                return true;
            })
            .catch(function(err) {
                if (isHandledAuthError(err)) {
                    return false;
                }
                if (!isHandledAuthError(err)) {
                    alert(err.message);
                }
                return false;
            });
    }

    function downloadFile(file) {
        downloadPath(joinPath(state.currentPath, file.name));
        return true;
    }

    function downloadPath(path) {
        window.open(withBasePath('/api/files/download?path=' + encodeURIComponent(path)), '_blank');
    }

    function setPreviewTarget(file, type) {
        const path = joinPath(state.currentPath, file.name);

        state.previewImagePath = path;
        state.previewImageName = file.name;
        state.previewImageVersion = String(file.modTime || Date.now());
        state.previewImageSize = Number(file.size || 0);
        state.previewContentType = type || '';
        state.previewTextContent = '';
        state.previewEditMode = false;
    }

    function previewImage(file) {
        if (!discardViewerDraft({ render: false })) {
            return;
        }
        setPreviewTarget(file, 'image');
        renderViewerPanel(true);
        setFileBrowserOpen(true, 'viewer');
    }

    function getViewerImageSiblings() {
        return sortFiles(filterFilesForDisplay(state.files).visibleFiles).filter(function(file) {
            return !file.isDir && isPreviewableImage(file.name);
        });
    }

    function navigateViewerImageByOffset(offset) {
        const imageFiles = getViewerImageSiblings();
        const currentIndex = imageFiles.findIndex(function(file) {
            return getFilePath(file) === state.previewImagePath;
        });
        const targetIndex = currentIndex + offset;

        if (currentIndex === -1 || targetIndex < 0 || targetIndex >= imageFiles.length) {
            return false;
        }

        previewImage(imageFiles[targetIndex]);
        return true;
    }

    function previewPdf(file) {
        if (!discardViewerDraft({ render: false })) {
            return;
        }
        setPreviewTarget(file, 'pdf');
        renderViewerPanel(true);
        setFileBrowserOpen(true, 'viewer');
    }

    function previewPptx(file) {
        if (!discardViewerDraft({ render: false })) {
            return;
        }
        setPreviewTarget(file, 'pptx');
        renderViewerPanel(true);
        setFileBrowserOpen(true, 'viewer');
    }

    function previewDocx(file) {
        if (!discardViewerDraft({ render: false })) {
            return;
        }
        setPreviewTarget(file, 'docx');
        renderViewerPanel(true);
        setFileBrowserOpen(true, 'viewer');
    }

    function previewXlsx(file) {
        if (!discardViewerDraft({ render: false })) {
            return;
        }
        setPreviewTarget(file, 'xlsx');
        renderViewerPanel(true);
        setFileBrowserOpen(true, 'viewer');
    }

    async function previewTextFile(file, options) {
        const path = joinPath(state.currentPath, file.name);
        const config = options || {};

        if (!discardViewerDraft({ render: false })) {
            return;
        }

        setPreviewTarget(file, 'text');
        renderViewerPanel(true);
        setFileBrowserOpen(true, 'viewer');

        try {
            const data = await fetchJSON('/api/files/read?path=' + encodeURIComponent(path), undefined, { authRequired: true });
            state.previewTextContent = typeof data.content === 'string' ? data.content : '';
            state.previewImageVersion = String(file.modTime || Date.now());
            renderViewerPanel(true);

            if (config.edit) {
                await enableViewerEditMode(path, file.name);
            }
        } catch (err) {
            if (isHandledAuthError(err)) {
                return;
            }

            const message = t('files.downloadInstead', { name: file.name, message: err.message });
            if (confirm(message)) {
                downloadPath(path);
            } else {
                window.closeImagePreview();
            }
        }
    }

    window.closeImagePreview = function() {
        if (shouldEmbedViewerEditor()) {
            closeEditor(state.previewImagePath);
            return;
        }

        const wasEditMode = state.previewEditMode;

        resetViewerPreviewState();
        if (wasEditMode) {
            applyEditorLayout();
        }
        renderViewerPanel();
    };

    window.downloadPreviewImage = function() {
        if (!state.previewImagePath) {
            return;
        }
        downloadPath(state.previewImagePath);
    };

    function shouldEmbedViewerEditor() {
        return Boolean(
            state.fileBrowserOpen &&
            state.fileBrowserTab === 'viewer' &&
            state.previewContentType === 'text' &&
            state.previewEditMode &&
            state.previewImagePath &&
            state.activeEditorPath === state.previewImagePath &&
            fileViewerEditorHost
        );
    }

    function resetViewerPreviewState() {
        state.previewImagePath = '';
        state.previewImageName = '';
        state.previewImageVersion = '';
        state.previewImageSize = 0;
        state.previewContentType = '';
        state.previewTextContent = '';
        state.previewEditMode = false;
    }

    function restoreEditorPaneHost() {
        if (!editorPane || !workspace || !splitter) {
            return;
        }

        if (editorPane.parentNode !== workspace) {
            workspace.insertBefore(editorPane, splitter);
        }
        editorPane.classList.remove('editor-pane-embedded');
    }

    function syncEmbeddedEditorHost() {
        if (!editorPane) {
            return;
        }

        if (shouldEmbedViewerEditor()) {
            if (editorPane.parentNode !== fileViewerEditorHost) {
                fileViewerEditorHost.appendChild(editorPane);
            }
            editorPane.classList.add('editor-pane-embedded');
            return;
        }

        restoreEditorPaneHost();
    }

    async function enableViewerEditMode(path, displayName) {
        const targetPath = path || state.previewImagePath;

        if (!targetPath || state.previewContentType !== 'text') {
            return;
        }

        state.previewEditMode = true;
        await openEditorPath(targetPath, displayName || state.previewImageName || baseName(targetPath));
        applyEditorLayout();
        renderViewerPanel(true);
        focusEditor();
    }

    window.openViewerEditMode = function() {
        if (state.previewContentType !== 'text') {
            return;
        }
        if (state.previewEditMode) {
            focusEditor();
            return;
        }
        enableViewerEditMode();
    };

    function pad2(n) {
        return n < 10 ? '0' + n : String(n);
    }

    function formatBytesShort(bytes) {
        var value = Number(bytes) || 0;
        if (value < 1024) {
            return value + ' B';
        }
        if (value < 1024 * 1024) {
            return (value / 1024).toFixed(1) + ' KB';
        }
        return (value / 1024 / 1024).toFixed(2) + ' MB';
    }

    function defaultDraftFilename(kind, mime) {
        var now = new Date();
        var stamp = now.getFullYear()
            + pad2(now.getMonth() + 1)
            + pad2(now.getDate())
            + '-'
            + pad2(now.getHours())
            + pad2(now.getMinutes())
            + pad2(now.getSeconds());

        if (kind === 'image') {
            return 'pasted-' + stamp + extensionForImageMime(mime || 'image/png');
        }
        return 'untitled-' + stamp + '.txt';
    }

    function extensionForImageMime(mime) {
        var normalized = (mime || '').toLowerCase();
        if (normalized === 'image/jpeg' || normalized === 'image/jpg') {
            return '.jpg';
        }
        if (normalized === 'image/gif') {
            return '.gif';
        }
        if (normalized === 'image/webp') {
            return '.webp';
        }
        if (normalized === 'image/svg+xml') {
            return '.svg';
        }
        if (normalized === 'image/bmp') {
            return '.bmp';
        }
        return '.png';
    }

    function createViewerDraft() {
        return {
            kind: 'text',
            filename: defaultDraftFilename('text'),
            textContent: '',
            imageBlob: null,
            imageObjectUrl: '',
            imageMime: '',
            saving: false
        };
    }

    function viewerDraftHasContent(draft) {
        var target = draft || state.viewerDraft;
        return Boolean(target && (target.textContent || target.imageBlob));
    }

    function focusViewerDraftPrimaryField() {
        window.requestAnimationFrame(function() {
            if (!state.viewerDraft) {
                return;
            }
            if (state.viewerDraft.kind === 'image') {
                if (fileViewerDraftFilename) {
                    fileViewerDraftFilename.focus();
                }
                return;
            }
            if (fileViewerDraftTextarea) {
                var end = fileViewerDraftTextarea.value.length;
                fileViewerDraftTextarea.focus();
                fileViewerDraftTextarea.setSelectionRange(end, end);
            }
        });
    }

    function ensureViewerDraft(options) {
        var config = options || {};
        if (!state.viewerDraft) {
            state.viewerDraft = createViewerDraft();
        }
        if (config.render !== false) {
            renderViewerPanel();
        }
        if (config.focus !== false) {
            focusViewerDraftPrimaryField();
        }
        return state.viewerDraft;
    }

    function revokeDraftImageUrl() {
        if (state.viewerDraft && state.viewerDraft.imageObjectUrl) {
            try {
                URL.revokeObjectURL(state.viewerDraft.imageObjectUrl);
            } catch (_) {
                // ignore
            }
            state.viewerDraft.imageObjectUrl = '';
        }
    }

    function setViewerDraftStatus(message, tone) {
        if (!fileViewerDraftStatus) {
            return;
        }
        fileViewerDraftStatus.textContent = message || '';
        fileViewerDraftStatus.classList.remove('is-error', 'is-success');
        if (tone === 'error') {
            fileViewerDraftStatus.classList.add('is-error');
        } else if (tone === 'success') {
            fileViewerDraftStatus.classList.add('is-success');
        }
    }

    function replaceFilenameExtension(name, nextExt) {
        var trimmed = (name || '').trim();
        if (!trimmed) {
            return '';
        }
        var dot = trimmed.lastIndexOf('.');
        if (dot <= 0) {
            return trimmed + nextExt;
        }
        return trimmed.slice(0, dot) + nextExt;
    }

    function discardViewerDraft(options) {
        var config = options || {};
        if (!state.viewerDraft) {
            return true;
        }
        if (!config.force && state.viewerDraft.saving) {
            return false;
        }
        if (!config.force && viewerDraftHasContent(state.viewerDraft)) {
            if (!window.confirm(t('viewer.newDraft.discardConfirm'))) {
                return false;
            }
        }
        revokeDraftImageUrl();
        state.viewerDraft = null;
        setViewerDraftStatus('');
        if (config.render !== false) {
            renderViewerPanel();
        }
        return true;
    }

    function getClipboardImageFile(clipboard) {
        var imageFile = null;
        if (!clipboard || !clipboard.items) {
            return null;
        }
        for (var i = 0; i < clipboard.items.length; i += 1) {
            var item = clipboard.items[i];
            if (item && item.kind === 'file' && item.type && item.type.indexOf('image/') === 0) {
                imageFile = item.getAsFile();
                if (imageFile) {
                    return imageFile;
                }
            }
        }
        return null;
    }

    function acceptDraftText(text, options) {
        var config = options || {};
        var pastedText = typeof text === 'string' ? text : '';
        var wasImage;

        if (!state.viewerDraft || !pastedText) {
            return false;
        }

        wasImage = state.viewerDraft.kind === 'image';
        if (wasImage) {
            if (!config.skipConfirm && !window.confirm(t('viewer.newDraft.switchToText'))) {
                return false;
            }
            revokeDraftImageUrl();
            state.viewerDraft.imageBlob = null;
            state.viewerDraft.imageMime = '';
            state.viewerDraft.filename = replaceFilenameExtension(state.viewerDraft.filename, '.txt')
                || defaultDraftFilename('text');
        } else if (!state.viewerDraft.filename) {
            state.viewerDraft.filename = defaultDraftFilename('text');
        }

        state.viewerDraft.kind = 'text';
        state.viewerDraft.textContent = pastedText;
        setViewerDraftStatus('');
        renderViewerPanel();
        if (config.focus !== false) {
            focusViewerDraftPrimaryField();
        }
        return true;
    }

    function openViewerDraftFromClipboard(clipboard) {
        var imageFile = getClipboardImageFile(clipboard);
        var pastedText = clipboard && clipboard.getData ? clipboard.getData('text/plain') : '';

        if (!imageFile && !pastedText) {
            return false;
        }

        ensureViewerDraft({ render: false, focus: false });
        if (imageFile) {
            if (!acceptDraftImage(imageFile)) {
                return false;
            }
            focusViewerDraftPrimaryField();
            return true;
        }
        return acceptDraftText(pastedText, { focus: true, skipConfirm: true });
    }

    function shouldHandleViewerEmptyClipboardPaste(event) {
        var activeElement = document.activeElement;

        if (!event || event.defaultPrevented || !event.clipboardData) {
            return false;
        }
        if (!state.fileBrowserOpen || state.fileBrowserTab !== 'viewer') {
            return false;
        }
        if (state.previewImagePath || state.viewerDraft) {
            return false;
        }
        if (isKeyboardTextInput(activeElement) && (!fileViewerView || !fileViewerView.contains(activeElement))) {
            return false;
        }
        return true;
    }

    window.toggleViewerDraft = function() {
        if (state.viewerDraft) {
            cancelViewerDraft();
            return;
        }
        openViewerDraft();
    };

    function openViewerDraft(options) {
        ensureViewerDraft(options);
    }

    window.cancelViewerDraft = function() {
        discardViewerDraft();
    };

    function closeViewerDraft() {
        discardViewerDraft({ force: true });
    }

    window.clearViewerDraftImage = function() {
        if (!state.viewerDraft || state.viewerDraft.kind !== 'image') {
            return;
        }
        revokeDraftImageUrl();
        state.viewerDraft.imageBlob = null;
        state.viewerDraft.imageMime = '';
        state.viewerDraft.kind = 'text';
        state.viewerDraft.filename = replaceFilenameExtension(state.viewerDraft.filename, '.txt')
            || defaultDraftFilename('text');
        setViewerDraftStatus('');
        renderViewerPanel();
        if (fileViewerDraftTextarea) {
            fileViewerDraftTextarea.focus();
        }
    };

    function handleViewerDraftPaste(event) {
        if (!state.viewerDraft) {
            return;
        }
        var clipboard = event.clipboardData;
        if (!clipboard) {
            return;
        }

        var imageFile = getClipboardImageFile(clipboard);

        if (imageFile) {
            if (acceptDraftImage(imageFile)) {
                event.preventDefault();
            }
            return;
        }

        if (state.viewerDraft.kind === 'image') {
            var pastedText = clipboard.getData ? clipboard.getData('text/plain') : '';
            if (!pastedText) {
                return;
            }
            if (acceptDraftText(pastedText)) {
                event.preventDefault();
            }
        }
    }

    function acceptDraftImage(file) {
        if (!state.viewerDraft || !file) {
            return false;
        }
        if (state.viewerDraft.kind === 'text' && state.viewerDraft.textContent) {
            if (!window.confirm(t('viewer.newDraft.switchToImage'))) {
                return false;
            }
        }
        revokeDraftImageUrl();
        var objectUrl = '';
        try {
            objectUrl = URL.createObjectURL(file);
        } catch (_) {
            objectUrl = '';
        }
        state.viewerDraft.kind = 'image';
        state.viewerDraft.imageBlob = file;
        state.viewerDraft.imageMime = file.type || 'image/png';
        state.viewerDraft.imageObjectUrl = objectUrl;
        state.viewerDraft.textContent = '';
        var currentName = (state.viewerDraft.filename || '').trim();
        var isDefaultText = !currentName || /^untitled-\d{8}-\d{6}\.txt$/i.test(currentName);
        if (isDefaultText) {
            state.viewerDraft.filename = defaultDraftFilename('image', state.viewerDraft.imageMime);
        } else {
            state.viewerDraft.filename = replaceFilenameExtension(
                currentName,
                extensionForImageMime(state.viewerDraft.imageMime)
            );
        }
        setViewerDraftStatus(t('viewer.newDraft.imagePasted', { size: formatBytesShort(file.size) }));
        renderViewerPanel();
        return true;
    }

    window.saveViewerDraft = async function() {
        if (!state.viewerDraft || state.viewerDraft.saving) {
            return;
        }
        var filename = (fileViewerDraftFilename && fileViewerDraftFilename.value || '').trim();
        if (!filename) {
            setViewerDraftStatus(t('viewer.newDraft.filenameRequired'), 'error');
            if (fileViewerDraftFilename) {
                fileViewerDraftFilename.focus();
            }
            return;
        }
        if (filename.indexOf('/') !== -1 || filename.indexOf('\\') !== -1) {
            setViewerDraftStatus(t('viewer.newDraft.filenameInvalid'), 'error');
            return;
        }
        state.viewerDraft.filename = filename;

        var kind = state.viewerDraft.kind;
        if (kind === 'text') {
            var text = fileViewerDraftTextarea ? fileViewerDraftTextarea.value : '';
            state.viewerDraft.textContent = text;
            if (!text) {
                setViewerDraftStatus(t('viewer.newDraft.noContent'), 'error');
                return;
            }
        } else if (kind === 'image') {
            if (!state.viewerDraft.imageBlob) {
                setViewerDraftStatus(t('viewer.newDraft.noContent'), 'error');
                return;
            }
        } else {
            return;
        }

        var dir = state.currentPath || '~';
        var fullPath = joinPath(dir, filename);

        state.viewerDraft.saving = true;
        setViewerDraftStatus(t('viewer.newDraft.saving'));
        if (fileViewerDraftSaveBtn) {
            fileViewerDraftSaveBtn.disabled = true;
        }

        try {
            if (kind === 'text') {
                await fetchJSON('/api/files/save', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ path: fullPath, content: state.viewerDraft.textContent })
                }, { authRequired: true });
            } else {
                var formData = new FormData();
                formData.append('file', state.viewerDraft.imageBlob, filename);
                var response = await fetchAPI('/api/files/upload?path=' + encodeURIComponent(dir), {
                    method: 'POST',
                    body: formData
                }, { authRequired: true });
                if (!response.ok) {
                    var errData = null;
                    try { errData = await response.json(); } catch (_) { errData = null; }
                    throw buildRequestError(response, errData, t('common.requestFailed'));
                }
            }

            var savedSize = kind === 'image' && state.viewerDraft && state.viewerDraft.imageBlob
                ? state.viewerDraft.imageBlob.size
                : (state.viewerDraft && state.viewerDraft.textContent ? state.viewerDraft.textContent.length : 0);

            setViewerDraftStatus(t('viewer.newDraft.saved', { name: filename }), 'success');
            closeViewerDraft();

            var savedFile = {
                name: filename,
                modTime: Date.now(),
                size: savedSize
            };
            if (kind === 'text') {
                previewTextFile(savedFile);
            } else {
                previewImage(savedFile);
            }

            if (state.fileBrowserOpen && isSameDirectory(state.currentPath, fullPath)) {
                if (typeof window.refreshFiles === 'function') {
                    window.refreshFiles();
                }
            }
        } catch (err) {
            if (state.viewerDraft) {
                state.viewerDraft.saving = false;
            }
            if (isHandledAuthError(err)) {
                return;
            }
            setViewerDraftStatus(t('viewer.newDraft.saveFailed', { message: err && err.message ? err.message : '' }), 'error');
        } finally {
            if (state.viewerDraft) {
                state.viewerDraft.saving = false;
            }
            if (fileViewerDraftSaveBtn) {
                fileViewerDraftSaveBtn.disabled = false;
            }
        }
    };

    function renderViewerDraft() {
        var draft = state.viewerDraft;
        if (!draft || !fileViewerDraft) {
            return;
        }
        if (fileViewerDraftFilename && fileViewerDraftFilename.value !== draft.filename) {
            fileViewerDraftFilename.value = draft.filename || '';
        }
        var isImage = draft.kind === 'image';
        if (fileViewerDraftTextarea) {
            fileViewerDraftTextarea.style.display = isImage ? 'none' : 'block';
            if (!isImage && fileViewerDraftTextarea.value !== draft.textContent) {
                fileViewerDraftTextarea.value = draft.textContent || '';
            }
        }
        if (fileViewerDraftImage) {
            fileViewerDraftImage.style.display = isImage ? 'flex' : 'none';
        }
        if (isImage && fileViewerDraftImagePreview) {
            if (draft.imageObjectUrl) {
                if (fileViewerDraftImagePreview.getAttribute('src') !== draft.imageObjectUrl) {
                    fileViewerDraftImagePreview.src = draft.imageObjectUrl;
                }
            } else {
                fileViewerDraftImagePreview.removeAttribute('src');
            }
        }
        if (isImage && fileViewerDraftImageSize && draft.imageBlob) {
            fileViewerDraftImageSize.textContent = formatBytesShort(draft.imageBlob.size);
        } else if (fileViewerDraftImageSize) {
            fileViewerDraftImageSize.textContent = '';
        }
        if (fileViewerDraftSaveBtn) {
            fileViewerDraftSaveBtn.disabled = draft.saving === true;
        }
    }

    if (fileViewerDraft) {
        fileViewerDraft.addEventListener('paste', handleViewerDraftPaste);
    }
    if (fileViewerDraftFilename) {
        fileViewerDraftFilename.addEventListener('input', function() {
            if (state.viewerDraft) {
                state.viewerDraft.filename = fileViewerDraftFilename.value;
            }
        });
    }
    if (fileViewerDraftTextarea) {
        fileViewerDraftTextarea.addEventListener('input', function() {
            if (state.viewerDraft && state.viewerDraft.kind === 'text') {
                state.viewerDraft.textContent = fileViewerDraftTextarea.value;
            }
        });
        fileViewerDraftTextarea.addEventListener('keydown', function(event) {
            if (event.key === 'Escape') {
                event.preventDefault();
                window.cancelViewerDraft();
            }
        });
    }

    function renderViewerPanel(forceReload) {
        if (state.viewerDraft) {
            if (fileViewerEmpty) {
                fileViewerEmpty.style.display = 'none';
            }
            if (fileViewerCanvas) {
                fileViewerCanvas.style.display = 'none';
            }
            if (fileViewerDraft) {
                fileViewerDraft.style.display = 'flex';
            }
            if (fileViewerTitle) {
                fileViewerTitle.textContent = t('viewer.new');
            }
            if (fileViewerPath) {
                fileViewerPath.textContent = state.currentPath || '~';
            }
            if (fileViewerCopyBtn) {
                fileViewerCopyBtn.style.display = 'none';
            }
            if (fileViewerEditBtn) {
                fileViewerEditBtn.style.display = 'none';
            }
            if (fileViewerDownloadBtn) {
                fileViewerDownloadBtn.disabled = true;
            }
            if (fileViewerNewBtn) {
                fileViewerNewBtn.classList.add('active');
            }
            restoreEditorPaneHost();
            renderViewerDraft();
            return;
        }
        if (fileViewerDraft) {
            fileViewerDraft.style.display = 'none';
        }
        if (fileViewerNewBtn) {
            fileViewerNewBtn.classList.remove('active');
        }
        const type = state.previewContentType || '';
        const imageKey = state.previewImagePath + '::' + String(state.previewImageVersion || '');
        const hasPreview = Boolean(state.previewImagePath);
        const isText = type === 'text';
        const isPdf = type === 'pdf';
        const isPptx = type === 'pptx';
        const isDocx = type === 'docx';
        const isXlsx = type === 'xlsx';
        const isImage = type === 'image';
        const isEditMode = isText && state.previewEditMode;

        if (!fileViewerTitle || !fileViewerPath || !fileViewerImage || !fileViewerCanvas || !fileViewerEmpty) {
            return;
        }

        if (!hasPreview) {
            fileViewerTitle.textContent = t('viewer.tab');
            fileViewerPath.textContent = state.currentPath || '~';
            resetViewerImageState();
            fileViewerCanvas.style.display = 'none';
            fileViewerEmpty.style.display = 'flex';
            if (fileViewerCopyBtn) {
                fileViewerCopyBtn.disabled = true;
                fileViewerCopyBtn.style.display = 'none';
            }
            if (fileViewerDownloadBtn) {
                fileViewerDownloadBtn.disabled = true;
            }
            if (fileViewerEditBtn) {
                fileViewerEditBtn.style.display = 'none';
            }
            restoreEditorPaneHost();
            return;
        }

        fileViewerTitle.textContent = state.previewImageName || baseName(state.previewImagePath);
        fileViewerPath.textContent = state.previewImagePath;
        if (fileViewerCopyBtn) {
            fileViewerCopyBtn.style.display = isText ? 'inline-flex' : 'none';
            fileViewerCopyBtn.disabled = !getViewerCopyText();
        }
        if (fileViewerDownloadBtn) {
            fileViewerDownloadBtn.disabled = false;
        }
        if (fileViewerEditBtn) {
            fileViewerEditBtn.style.display = isText ? 'inline-flex' : 'none';
            fileViewerEditBtn.classList.toggle('active', isEditMode);
        }

        if (fileViewerImageStage) {
            fileViewerImageStage.style.display = isImage ? 'flex' : 'none';
        }
        if (fileViewerPdfFrame) {
            fileViewerPdfFrame.style.display = isPdf ? 'block' : 'none';
        }
        if (fileViewerPptxStage) {
            fileViewerPptxStage.style.display = isPptx ? 'block' : 'none';
        }
        if (fileViewerDocxStage) {
            fileViewerDocxStage.style.display = isDocx ? 'block' : 'none';
        }
        if (fileViewerXlsxStage) {
            fileViewerXlsxStage.style.display = isXlsx ? 'block' : 'none';
        }
        if (fileViewerTextContent) {
            fileViewerTextContent.style.display = isText && !isEditMode ? 'block' : 'none';
        }
        if (fileViewerEditorHost) {
            fileViewerEditorHost.style.display = isEditMode ? 'flex' : 'none';
        }

        if (isImage) {
            if (forceReload || fileViewerCanvas.dataset.imageKey !== imageKey || !fileViewerImage.getAttribute('src')) {
                loadViewerImage(imageKey);
            }
            restoreEditorPaneHost();
        } else if (isPdf) {
            resetViewerImageState();
            if (fileViewerPdfFrame && (forceReload || fileViewerPdfFrame.dataset.fileKey !== imageKey || !fileViewerPdfFrame.getAttribute('src'))) {
                fileViewerPdfFrame.dataset.fileKey = imageKey;
                fileViewerPdfFrame.src = withBasePath('/api/files/download?path=' + encodeURIComponent(state.previewImagePath) + '&inline=1&v=' + encodeURIComponent(String(state.previewImageVersion || Date.now())));
            }
            restoreEditorPaneHost();
        } else if (isPptx) {
            resetViewerImageState();
            loadViewerPptx(imageKey);
            restoreEditorPaneHost();
        } else if (isDocx) {
            resetViewerImageState();
            loadViewerDocx(imageKey);
            restoreEditorPaneHost();
        } else if (isXlsx) {
            resetViewerImageState();
            loadViewerXlsx(imageKey);
            restoreEditorPaneHost();
        } else if (isText) {
            resetViewerImageState();
            if (fileViewerTextContent) {
                fileViewerTextContent.innerHTML = highlightTextForPath(state.previewTextContent || '', state.previewImagePath || state.previewImageName || '');
            }
            if (isEditMode) {
                syncEmbeddedEditorHost();
            } else {
                restoreEditorPaneHost();
            }
        }

        fileViewerCanvas.style.display = 'flex';
        fileViewerEmpty.style.display = 'none';
    }

    function resetViewerImageState() {
        viewerLoadSequence += 1;
        fileViewerCanvas.dataset.imageKey = '';
        fileViewerCanvas.classList.remove('has-preview', 'is-loading', 'is-ready');
        fileViewerImage.onload = null;
        fileViewerImage.onerror = null;
        fileViewerImage.removeAttribute('src');
        if (fileViewerPdfFrame) {
            fileViewerPdfFrame.dataset.fileKey = '';
            fileViewerPdfFrame.removeAttribute('src');
        }
        resetPptxViewerState();
        resetDocxViewerState();
        resetXlsxViewerState();
        if (fileViewerTextContent) {
            fileViewerTextContent.innerHTML = '';
        }
        if (fileViewerPreviewImage) {
            fileViewerPreviewImage.onload = null;
            fileViewerPreviewImage.onerror = null;
            fileViewerPreviewImage.removeAttribute('src');
        }
        if (fileViewerLoadingIndicator) {
            fileViewerLoadingIndicator.style.display = '';
        }
    }

    function resetPptxViewerState() {
        if (pptxViewerResizeObserver) {
            pptxViewerResizeObserver.disconnect();
            pptxViewerResizeObserver = null;
        }
        if (pptxViewerResizeFrame) {
            window.cancelAnimationFrame(pptxViewerResizeFrame);
            pptxViewerResizeFrame = 0;
        }
        if (pptxViewerInstance && typeof pptxViewerInstance.destroy === 'function') {
            try {
                pptxViewerInstance.destroy();
            } catch (err) {
                console.warn('PPTX viewer cleanup failed', err);
            }
        }
        pptxViewerInstance = null;
        if (fileViewerPptxHost) {
            fileViewerPptxHost.innerHTML = '';
        }
        setPptxViewerStatus('');
    }

    function getPptxViewerConstructor() {
        if (!window.PptxViewJS || typeof window.PptxViewJS.PPTXViewer !== 'function') {
            return null;
        }
        return window.PptxViewJS.PPTXViewer;
    }

    function createPptxRenderHost(requestToken) {
        if (!fileViewerPptxHost) {
            return null;
        }

        const renderHost = document.createElement('div');
        renderHost.id = 'viewer-pptx-render-' + String(requestToken);
        renderHost.className = 'viewer-pptx-render-host';

        const toolbar = document.createElement('div');
        toolbar.className = 'viewer-pptx-toolbar';

        const prevBtn = document.createElement('button');
        prevBtn.type = 'button';
        prevBtn.className = 'viewer-pptx-nav';
        prevBtn.textContent = '\u2039';
        prevBtn.disabled = true;
        prevBtn.setAttribute('aria-label', t('viewer.pptxPrev'));
        prevBtn.title = t('viewer.pptxPrev');

        const page = document.createElement('span');
        page.className = 'viewer-pptx-page';
        page.textContent = '';

        const nextBtn = document.createElement('button');
        nextBtn.type = 'button';
        nextBtn.className = 'viewer-pptx-nav';
        nextBtn.textContent = '\u203a';
        nextBtn.disabled = true;
        nextBtn.setAttribute('aria-label', t('viewer.pptxNext'));
        nextBtn.title = t('viewer.pptxNext');

        const canvasWrap = document.createElement('div');
        canvasWrap.className = 'viewer-pptx-canvas-wrap';

        const canvas = document.createElement('canvas');
        canvas.className = 'viewer-pptx-canvas';

        toolbar.appendChild(prevBtn);
        toolbar.appendChild(page);
        toolbar.appendChild(nextBtn);
        canvasWrap.appendChild(canvas);
        renderHost.appendChild(toolbar);
        renderHost.appendChild(canvasWrap);
        fileViewerPptxHost.appendChild(renderHost);
        return {
            root: renderHost,
            toolbar: toolbar,
            canvasWrap: canvasWrap,
            canvas: canvas,
            prevBtn: prevBtn,
            nextBtn: nextBtn,
            page: page
        };
    }

    function getPptxSlideAspectRatio(viewer) {
        if (!viewer || typeof viewer.getSlideDimensions !== 'function') {
            return 4 / 3;
        }

        const slideSize = viewer.getSlideDimensions();
        const width = slideSize && Number(slideSize.cx);
        const height = slideSize && Number(slideSize.cy);

        if (!width || !height) {
            return 4 / 3;
        }

        return width / height;
    }

    function syncPptxCanvasSize(viewer, renderHost) {
        if (!renderHost || !renderHost.canvas) {
            return false;
        }

        const canvas = renderHost.canvas;
        const canvasWrap = renderHost.canvasWrap;
        const toolbar = renderHost.toolbar;
        const slideRatio = getPptxSlideAspectRatio(viewer);
        const stageRect = fileViewerPptxStage ? fileViewerPptxStage.getBoundingClientRect() : null;
        const wrapRect = canvasWrap ? canvasWrap.getBoundingClientRect() : null;
        const toolbarRect = toolbar ? toolbar.getBoundingClientRect() : null;
        const gutter = 36;
        let maxWidth = wrapRect ? Math.floor(wrapRect.width) : 0;
        let maxHeight = wrapRect ? Math.floor(wrapRect.height) : 0;

        if (!maxWidth && stageRect) {
            maxWidth = Math.max(0, Math.floor(stageRect.width - gutter));
        }
        if (!maxHeight && stageRect) {
            maxHeight = Math.max(0, Math.floor(stageRect.height - (toolbarRect ? toolbarRect.height : 0) - gutter));
        }
        if (!maxWidth || !maxHeight) {
            return false;
        }

        let displayWidth = maxWidth;
        let displayHeight = Math.round(displayWidth / slideRatio);

        if (displayHeight > maxHeight) {
            displayHeight = maxHeight;
            displayWidth = Math.round(displayHeight * slideRatio);
        }
        if (!displayWidth || !displayHeight) {
            return false;
        }

        const pixelRatio = window.devicePixelRatio || 1;
        const targetWidth = Math.max(1, Math.round(displayWidth * pixelRatio));
        const targetHeight = Math.max(1, Math.round(displayHeight * pixelRatio));

        if (
            canvas.width === targetWidth &&
            canvas.height === targetHeight &&
            canvas.style.width === displayWidth + 'px' &&
            canvas.style.height === displayHeight + 'px'
        ) {
            return false;
        }

        canvas.style.width = displayWidth + 'px';
        canvas.style.height = displayHeight + 'px';
        canvas.width = targetWidth;
        canvas.height = targetHeight;
        return true;
    }

    function schedulePptxViewerResize(viewer, renderHost) {
        if (!viewer || !renderHost || !renderHost.canvas) {
            return;
        }

        if (pptxViewerResizeFrame) {
            window.cancelAnimationFrame(pptxViewerResizeFrame);
        }

        pptxViewerResizeFrame = window.requestAnimationFrame(function() {
            pptxViewerResizeFrame = 0;

            if (viewer !== pptxViewerInstance || !renderHost.canvas.isConnected) {
                return;
            }
            if (!syncPptxCanvasSize(viewer, renderHost)) {
                return;
            }

            Promise.resolve(viewer.render(renderHost.canvas)).then(function() {
                updatePptxViewerControls(viewer, renderHost);
            }).catch(function(err) {
                console.warn('PPTX resize render failed', err);
            });
        });
    }

    function observePptxViewerResize(viewer, renderHost) {
        if (pptxViewerResizeObserver) {
            pptxViewerResizeObserver.disconnect();
            pptxViewerResizeObserver = null;
        }
        if (typeof window.ResizeObserver !== 'function' || !renderHost || !renderHost.canvasWrap) {
            return;
        }

        pptxViewerResizeObserver = new window.ResizeObserver(function() {
            schedulePptxViewerResize(viewer, renderHost);
        });
        pptxViewerResizeObserver.observe(renderHost.canvasWrap);
        if (fileViewerPptxStage) {
            pptxViewerResizeObserver.observe(fileViewerPptxStage);
        }
    }

    function updatePptxViewerControls(viewer, renderHost) {
        if (!viewer || !renderHost) {
            return;
        }

        const totalSlides = typeof viewer.getSlideCount === 'function' ? Number(viewer.getSlideCount()) : 0;
        const currentIndex = typeof viewer.getCurrentSlideIndex === 'function' ? Number(viewer.getCurrentSlideIndex()) : 0;
        const currentSlide = totalSlides > 0 ? currentIndex + 1 : 0;

        if (renderHost.page) {
            renderHost.page.textContent = totalSlides > 0
                ? t('viewer.pptxSlideStatus', { current: currentSlide, total: totalSlides })
                : '';
        }
        if (renderHost.prevBtn) {
            renderHost.prevBtn.disabled = totalSlides <= 1 || currentIndex <= 0;
        }
        if (renderHost.nextBtn) {
            renderHost.nextBtn.disabled = totalSlides <= 1 || currentIndex >= totalSlides - 1;
        }
    }

    function setPptxViewerStatus(message) {
        if (!fileViewerPptxStatus) {
            return;
        }
        if (!message) {
            fileViewerPptxStatus.textContent = '';
            fileViewerPptxStatus.style.display = 'none';
            return;
        }
        fileViewerPptxStatus.textContent = message;
        fileViewerPptxStatus.style.display = 'block';
    }

    function resetDocxViewerState() {
        if (fileViewerDocxHost) {
            fileViewerDocxHost.innerHTML = '';
        }
        setDocxViewerStatus('');
    }

    function resetXlsxViewerState() {
        if (fileViewerXlsxHost) {
            fileViewerXlsxHost.innerHTML = '';
        }
        setXlsxViewerStatus('');
    }

    function setDocxViewerStatus(message) {
        if (!fileViewerDocxStatus) {
            return;
        }
        if (!message) {
            fileViewerDocxStatus.textContent = '';
            fileViewerDocxStatus.style.display = 'none';
            return;
        }
        fileViewerDocxStatus.textContent = message;
        fileViewerDocxStatus.style.display = 'block';
    }

    function setXlsxViewerStatus(message) {
        if (!fileViewerXlsxStatus) {
            return;
        }
        if (!message) {
            fileViewerXlsxStatus.textContent = '';
            fileViewerXlsxStatus.style.display = 'none';
            return;
        }
        fileViewerXlsxStatus.textContent = message;
        fileViewerXlsxStatus.style.display = 'block';
    }

    function getDocxViewerApi() {
        if (!window.docx || typeof window.docx.renderAsync !== 'function') {
            return null;
        }
        return window.docx;
    }

    function getXlsxViewerApi() {
        if (!window.XLSX || typeof window.XLSX.read !== 'function' || !window.XLSX.utils || typeof window.XLSX.utils.sheet_to_html !== 'function') {
            return null;
        }
        return window.XLSX;
    }

    function loadExternalScript(src, isReady) {
        return new Promise(function(resolve, reject) {
            function isScriptReady() {
                if (typeof isReady !== 'function') {
                    return false;
                }
                return Boolean(isReady());
            }

            function attachExistingScriptListeners(script) {
                function cleanup() {
                    script.removeEventListener('load', onLoad);
                    script.removeEventListener('error', onError);
                }

                function onLoad() {
                    cleanup();
                    resolve();
                }

                function onError() {
                    cleanup();
                    if (script.parentNode) {
                        script.parentNode.removeChild(script);
                    }
                    reject(new Error('failed to load script: ' + src));
                }

                script.addEventListener('load', onLoad);
                script.addEventListener('error', onError);

                if (script.dataset.roambenchState === 'loaded' || isScriptReady()) {
                    cleanup();
                    resolve();
                    return true;
                }

                if (script.dataset.roambenchState === 'error') {
                    cleanup();
                    if (script.parentNode) {
                        script.parentNode.removeChild(script);
                    }
                    return false;
                }

                return true;
            }

            if (typeof isReady === 'function') {
                try {
                    if (isScriptReady()) {
                        resolve();
                        return;
                    }
                } catch (err) {
                    reject(err);
                    return;
                }
            }

            const selector = 'script[data-roambench-src="' + src + '"]';
            let existing = document.head ? document.head.querySelector(selector) : null;
            if (existing) {
                if (existing.dataset.roambenchState === 'loaded' || existing.dataset.roambenchState === 'error') {
                    if (existing.parentNode) {
                        existing.parentNode.removeChild(existing);
                    }
                    existing = null;
                } else if (attachExistingScriptListeners(existing)) {
                    return;
                } else {
                    existing = null;
                }
            }

            if (!document.head) {
                reject(new Error('failed to load script: ' + src));
                return;
            }

            const script = document.createElement('script');
            script.src = src;
            script.async = true;
            script.crossOrigin = 'anonymous';
            script.dataset.roambenchSrc = src;
            script.dataset.roambenchState = 'loading';
            script.onload = function() {
                script.dataset.roambenchState = 'loaded';
                resolve();
            };
            script.onerror = function() {
                script.dataset.roambenchState = 'error';
                if (script.parentNode) {
                    script.parentNode.removeChild(script);
                }
                reject(new Error('failed to load script: ' + src));
            };
            document.head.appendChild(script);
        });
    }

    function ensureDocxViewerLibrary() {
        const viewerApi = getDocxViewerApi();
        if (viewerApi && window.JSZip) {
            return Promise.resolve(viewerApi);
        }

        if (!docxViewerLibraryPromise) {
            docxViewerLibraryPromise = loadExternalScript(PPTX_VIEWER_JSZIP_URL, function() {
                return Boolean(window.JSZip);
            }).then(function() {
                return loadExternalScript(DOCX_VIEWER_SCRIPT_URL, function() {
                    return Boolean(getDocxViewerApi());
                });
            }).then(function() {
                const loadedViewerApi = getDocxViewerApi();
                if (!loadedViewerApi) {
                    throw new Error('docx viewer library unavailable');
                }
                return loadedViewerApi;
            }).catch(function(err) {
                docxViewerLibraryPromise = null;
                throw err;
            });
        }

        return docxViewerLibraryPromise;
    }

    function ensurePptxViewerLibrary() {
        const viewerCtor = getPptxViewerConstructor();
        if (viewerCtor && window.JSZip) {
            return Promise.resolve(viewerCtor);
        }

        if (!pptxViewerLibraryPromise) {
            pptxViewerLibraryPromise = loadExternalScript(PPTX_VIEWER_JSZIP_URL, function() {
                return Boolean(window.JSZip);
            }).then(function() {
                return loadExternalScript(PPTX_VIEWER_CHART_URL, function() {
                    return Boolean(window.Chart);
                });
            }).then(function() {
                return loadExternalScript(PPTX_VIEWER_SCRIPT_URL, function() {
                    return Boolean(getPptxViewerConstructor());
                });
            }).then(function() {
                const loadedViewerCtor = getPptxViewerConstructor();
                if (!loadedViewerCtor) {
                    throw new Error('pptx viewer library unavailable');
                }
                return loadedViewerCtor;
            }).catch(function(err) {
                pptxViewerLibraryPromise = null;
                throw err;
            });
        }

        return pptxViewerLibraryPromise;
    }

    function ensureXlsxViewerLibrary() {
        const viewerApi = getXlsxViewerApi();
        if (viewerApi) {
            return Promise.resolve(viewerApi);
        }

        if (!xlsxViewerLibraryPromise) {
            xlsxViewerLibraryPromise = loadExternalScript(XLSX_VIEWER_SCRIPT_URL, function() {
                return Boolean(getXlsxViewerApi());
            }).then(function() {
                const loadedViewerApi = getXlsxViewerApi();
                if (!loadedViewerApi) {
                    throw new Error('xlsx viewer library unavailable');
                }
                return loadedViewerApi;
            }).catch(function(err) {
                xlsxViewerLibraryPromise = null;
                throw err;
            });
        }

        return xlsxViewerLibraryPromise;
    }

    async function fetchViewerFileResponse(path, version) {
        const response = await fetchAPI('/api/files/download?path=' + encodeURIComponent(path) + '&v=' + encodeURIComponent(version), {
            cache: 'no-store'
        }, { authRequired: true });

        if (!response.ok) {
            let message = '';

            try {
                message = (await response.text()).trim();
            } catch (_) {
                message = '';
            }
            throw buildRequestError(response, message ? { error: message } : null, t('common.requestFailed'));
        }

        return response;
    }

    async function fetchViewerFileBlob(path, version) {
        const response = await fetchViewerFileResponse(path, version);
        return response.blob();
    }

    async function fetchViewerFileArrayBuffer(path, version) {
        const response = await fetchViewerFileResponse(path, version);
        return response.arrayBuffer();
    }

    async function loadViewerPptx(imageKey) {
        const requestToken = ++viewerLoadSequence;
        const path = state.previewImagePath;
        const version = String(state.previewImageVersion || Date.now());
        const encodedPath = encodeURIComponent(path);
        const encodedVersion = encodeURIComponent(version);
        const fullSrc = withBasePath('/api/files/download?path=' + encodedPath + '&inline=1&v=' + encodedVersion);

        fileViewerCanvas.dataset.imageKey = imageKey;
        fileViewerCanvas.classList.remove('has-preview', 'is-ready');
        fileViewerCanvas.classList.add('is-loading');
        resetPptxViewerState();
        setPptxViewerStatus(t('viewer.pptxLoading'));
        if (fileViewerLoadingIndicator) {
            fileViewerLoadingIndicator.style.display = 'inline-flex';
        }

        try {
            const ViewerCtor = await ensurePptxViewerLibrary();
            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerPptxHost) {
                return;
            }

            resetPptxViewerState();
            setPptxViewerStatus(t('viewer.pptxLoading'));

            const renderHost = createPptxRenderHost(requestToken);
            if (!renderHost || !renderHost.canvas) {
                return;
            }

            const viewer = new ViewerCtor({
                canvas: renderHost.canvas,
                slideSizeMode: 'fit'
            });
            await Promise.resolve(viewer.loadFromUrl(fullSrc));
            syncPptxCanvasSize(viewer, renderHost);

            if (!isActiveViewerRequest(requestToken, imageKey)) {
                if (typeof viewer.destroy === 'function') {
                    viewer.destroy();
                }
                return;
            }

            await Promise.resolve(viewer.render(renderHost.canvas));
            if (!isActiveViewerRequest(requestToken, imageKey)) {
                if (typeof viewer.destroy === 'function') {
                    viewer.destroy();
                }
                return;
            }

            renderHost.prevBtn.addEventListener('click', function() {
                syncPptxCanvasSize(viewer, renderHost);
                Promise.resolve(viewer.previousSlide(renderHost.canvas)).then(function() {
                    updatePptxViewerControls(viewer, renderHost);
                }).catch(function(err) {
                    console.error('PPTX previous-slide render failed', err);
                    setPptxViewerStatus(t('viewer.pptxUnavailable'));
                });
            });
            renderHost.nextBtn.addEventListener('click', function() {
                syncPptxCanvasSize(viewer, renderHost);
                Promise.resolve(viewer.nextSlide(renderHost.canvas)).then(function() {
                    updatePptxViewerControls(viewer, renderHost);
                }).catch(function(err) {
                    console.error('PPTX next-slide render failed', err);
                    setPptxViewerStatus(t('viewer.pptxUnavailable'));
                });
            });
            updatePptxViewerControls(viewer, renderHost);
            pptxViewerInstance = viewer;
            observePptxViewerResize(viewer, renderHost);

            fileViewerCanvas.classList.add('is-ready');
            fileViewerCanvas.classList.remove('is-loading');
            setPptxViewerStatus('');
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        } catch (err) {
            if (!isActiveViewerRequest(requestToken, imageKey)) {
                return;
            }

            console.error('PPTX preview failed', err);
            fileViewerCanvas.classList.remove('is-loading', 'is-ready');
            resetPptxViewerState();
            setPptxViewerStatus(t('viewer.pptxUnavailable'));
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        }
    }

    async function loadViewerDocx(imageKey) {
        const requestToken = ++viewerLoadSequence;
        const path = state.previewImagePath;
        const version = String(state.previewImageVersion || Date.now());

        fileViewerCanvas.dataset.imageKey = imageKey;
        fileViewerCanvas.classList.remove('has-preview', 'is-ready');
        fileViewerCanvas.classList.add('is-loading');
        resetDocxViewerState();
        setDocxViewerStatus(t('viewer.docxLoading'));
        if (fileViewerLoadingIndicator) {
            fileViewerLoadingIndicator.style.display = 'inline-flex';
        }

        try {
            const viewerApi = await ensureDocxViewerLibrary();
            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerDocxHost) {
                return;
            }

            const blob = await fetchViewerFileBlob(path, version);
            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerDocxHost) {
                return;
            }

            const renderShell = document.createElement('div');
            const renderStyles = document.createElement('div');
            const renderBody = document.createElement('div');

            renderShell.className = 'viewer-docx-render-shell';
            renderStyles.className = 'viewer-docx-render-styles';
            renderBody.className = 'viewer-docx-render-body';
            renderShell.appendChild(renderStyles);
            renderShell.appendChild(renderBody);

            await Promise.resolve(viewerApi.renderAsync(blob, renderBody, renderStyles, {
                inWrapper: true,
                ignoreFonts: true,
                useBase64URL: true
            }));

            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerDocxHost) {
                return;
            }

            resetDocxViewerState();
            fileViewerDocxHost.appendChild(renderShell);
            fileViewerCanvas.classList.add('is-ready');
            fileViewerCanvas.classList.remove('is-loading');
            setDocxViewerStatus('');
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        } catch (err) {
            if (!isActiveViewerRequest(requestToken, imageKey)) {
                return;
            }

            console.error('DOCX preview failed', err);
            fileViewerCanvas.classList.remove('is-loading', 'is-ready');
            resetDocxViewerState();
            setDocxViewerStatus(t('viewer.docxUnavailable'));
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        }
    }

    function renderXlsxWorkbook(viewerApi, workbook) {
        if (!fileViewerXlsxHost || !viewerApi || !workbook || !Array.isArray(workbook.SheetNames) || !workbook.SheetNames.length) {
            return;
        }

        const renderShell = document.createElement('div');
        const toolbar = document.createElement('div');
        const sheetWrap = document.createElement('div');
        const buttons = [];
        const sheetNames = workbook.SheetNames.slice();

        renderShell.className = 'viewer-xlsx-render-shell';
        toolbar.className = 'viewer-xlsx-toolbar';
        sheetWrap.className = 'viewer-xlsx-sheet';

        function renderSheet(index) {
            const normalizedIndex = index >= 0 && index < sheetNames.length ? index : 0;
            const sheetName = sheetNames[normalizedIndex];
            const worksheet = workbook.Sheets[sheetName];

            buttons.forEach(function(button, buttonIndex) {
                button.classList.toggle('active', buttonIndex === normalizedIndex);
            });

            sheetWrap.innerHTML = worksheet
                ? viewerApi.utils.sheet_to_html(worksheet)
                : '';
        }

        sheetNames.forEach(function(sheetName, index) {
            const button = document.createElement('button');

            button.type = 'button';
            button.className = 'viewer-xlsx-sheet-tab';
            button.textContent = sheetName || ('Sheet ' + String(index + 1));
            button.addEventListener('click', function() {
                renderSheet(index);
            });
            buttons.push(button);
            toolbar.appendChild(button);
        });

        renderShell.appendChild(toolbar);
        renderShell.appendChild(sheetWrap);
        fileViewerXlsxHost.appendChild(renderShell);
        renderSheet(0);
    }

    async function loadViewerXlsx(imageKey) {
        const requestToken = ++viewerLoadSequence;
        const path = state.previewImagePath;
        const version = String(state.previewImageVersion || Date.now());

        fileViewerCanvas.dataset.imageKey = imageKey;
        fileViewerCanvas.classList.remove('has-preview', 'is-ready');
        fileViewerCanvas.classList.add('is-loading');
        resetXlsxViewerState();
        setXlsxViewerStatus(t('viewer.xlsxLoading'));
        if (fileViewerLoadingIndicator) {
            fileViewerLoadingIndicator.style.display = 'inline-flex';
        }

        try {
            const viewerApi = await ensureXlsxViewerLibrary();
            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerXlsxHost) {
                return;
            }

            const data = await fetchViewerFileArrayBuffer(path, version);
            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerXlsxHost) {
                return;
            }

            const workbook = viewerApi.read(data, { type: 'array' });
            if (!workbook || !Array.isArray(workbook.SheetNames) || !workbook.SheetNames.length) {
                throw new Error('xlsx workbook contains no worksheets');
            }
            if (!isActiveViewerRequest(requestToken, imageKey) || !fileViewerXlsxHost) {
                return;
            }

            resetXlsxViewerState();
            renderXlsxWorkbook(viewerApi, workbook);
            fileViewerCanvas.classList.add('is-ready');
            fileViewerCanvas.classList.remove('is-loading');
            setXlsxViewerStatus('');
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        } catch (err) {
            if (!isActiveViewerRequest(requestToken, imageKey)) {
                return;
            }

            console.error('XLSX preview failed', err);
            fileViewerCanvas.classList.remove('is-loading', 'is-ready');
            resetXlsxViewerState();
            setXlsxViewerStatus(t('viewer.xlsxUnavailable'));
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        }
    }

    function loadViewerImage(imageKey) {
        const requestToken = ++viewerLoadSequence;
        const path = state.previewImagePath;
        const version = String(state.previewImageVersion || Date.now());
        const encodedPath = encodeURIComponent(path);
        const encodedVersion = encodeURIComponent(version);
        const fullSrc = withBasePath('/api/files/download?path=' + encodedPath + '&inline=1&v=' + encodedVersion);

        fileViewerCanvas.dataset.imageKey = imageKey;
        fileViewerCanvas.classList.remove('has-preview', 'is-ready');
        fileViewerCanvas.classList.add('is-loading');
        fileViewerImage.removeAttribute('src');
        if (fileViewerPreviewImage) {
            fileViewerPreviewImage.removeAttribute('src');
        }
        if (fileViewerLoadingIndicator) {
            fileViewerLoadingIndicator.style.display = 'inline-flex';
        }

        fileViewerImage.onload = function() {
            if (!isActiveViewerRequest(requestToken, imageKey)) {
                return;
            }
            fileViewerCanvas.classList.add('is-ready');
            fileViewerCanvas.classList.remove('is-loading');
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        };

        fileViewerImage.onerror = function() {
            if (!isActiveViewerRequest(requestToken, imageKey)) {
                return;
            }
            fileViewerCanvas.classList.remove('is-loading', 'is-ready');
            if (fileViewerLoadingIndicator) {
                fileViewerLoadingIndicator.style.display = 'none';
            }
        };

        if (fileViewerPreviewImage && shouldUseProgressivePreview()) {
            fileViewerPreviewImage.onload = function() {
                if (!isActiveViewerRequest(requestToken, imageKey)) {
                    return;
                }
                fileViewerCanvas.classList.add('has-preview');
            };
            fileViewerPreviewImage.onerror = function() {
                if (!isActiveViewerRequest(requestToken, imageKey)) {
                    return;
                }
                fileViewerCanvas.classList.remove('has-preview');
                fileViewerPreviewImage.removeAttribute('src');
            };
            fileViewerPreviewImage.src = withBasePath('/api/files/preview?path=' + encodedPath + '&size=96&v=' + encodedVersion);
        }

        fileViewerImage.src = fullSrc;
    }

    function isActiveViewerRequest(requestToken, imageKey) {
        return requestToken === viewerLoadSequence && fileViewerCanvas.dataset.imageKey === imageKey;
    }

    function shouldUseProgressivePreview() {
        if (!supportsLowResPreview(state.previewImageName || state.previewImagePath)) {
            return false;
        }
        return state.previewImageSize <= 0 || state.previewImageSize >= 128 * 1024;
    }

    window.promptNewFile = function() {
        const rawPath = prompt(t('files.newFilePrompt'));
        let targetPath;
        let existingVisible;

        if (rawPath === null) {
            return;
        }

        targetPath = normalizePromptPath(rawPath, state.currentPath || '~');
        if (!targetPath) {
            return;
        }

        if (isKnownDirectoryPath(targetPath, '')) {
            alert(t('files.cannotOverwriteDirectory', { path: targetPath }));
            return;
        }

        existingVisible = findVisibleFileForPath(targetPath);
        if (existingVisible && !existingVisible.isDir) {
            openEditorPath(targetPath, existingVisible.name)
                .then(function() {
                    setFileBrowserOpen(false);
                })
                .catch(function(err) {
                    if (!isHandledAuthError(err)) {
                        alert(t('editor.saveFailed', { message: err.message }));
                    }
                });
            return;
        }

        createTransientEditor(targetPath, baseName(targetPath), '');
        setFileBrowserOpen(false);
    };

    function renameFile(file) {
        const oldPath = joinPath(state.currentPath, file.name);
        const rawPath = prompt(t('files.renamePrompt'), file.name);
        let newPath;

        if (rawPath === null) {
            return Promise.resolve(false);
        }

        newPath = normalizePromptPath(rawPath, state.currentPath || '~');
        if (!newPath || newPath === oldPath) {
            return Promise.resolve(false);
        }

        if (findEditor(newPath) && newPath !== oldPath) {
            alert(t('editor.pathAlreadyOpen', { path: newPath }));
            return Promise.resolve(false);
        }

        if (isKnownDirectoryPath(newPath, oldPath)) {
            alert(t('files.cannotOverwriteDirectory', { path: newPath }));
            return Promise.resolve(false);
        }

        return fetchJSON('/api/files/rename', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ oldPath: oldPath, newPath: newPath })
        }, { authRequired: true })
            .then(function() {
                syncPathConsumersAfterRename(oldPath, newPath);
                window.refreshFiles();
                return true;
            })
            .catch(function(err) {
                if (isHandledAuthError(err)) {
                    return false;
                }
                if (!isHandledAuthError(err)) {
                    alert(t('files.renameFailed', { message: err.message }));
                }
                return false;
            });
    }

    function copyFile(file) {
        const oldPath = joinPath(state.currentPath, file.name);
        const rawPath = prompt(t('files.copyPrompt'), suggestCopyPath(oldPath));
        let newPath;

        if (rawPath === null) {
            return Promise.resolve(false);
        }

        newPath = normalizePromptPath(rawPath, state.currentPath || '~');
        if (!newPath || newPath === oldPath) {
            return Promise.resolve(false);
        }

        if (findEditor(newPath)) {
            alert(t('editor.pathAlreadyOpen', { path: newPath }));
            return Promise.resolve(false);
        }

        if (isKnownDirectoryPath(newPath, oldPath)) {
            alert(t('files.cannotOverwriteDirectory', { path: newPath }));
            return Promise.resolve(false);
        }

        return fetchJSON('/api/files/copy', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ oldPath: oldPath, newPath: newPath })
        }, { authRequired: true })
            .then(function() {
                window.refreshFiles();
                return true;
            })
            .catch(function(err) {
                if (isHandledAuthError(err)) {
                    return false;
                }
                if (!isHandledAuthError(err)) {
                    alert(t('files.copyFailed', { message: err.message }));
                }
                return false;
            });
    }

    window.promptNewFolder = function() {
        const rawPath = prompt(t('files.newFolderPrompt'));
        const path = normalizePromptPath(rawPath, state.currentPath || '~');

        if (!path) {
            return;
        }

        fetchJSON('/api/files/mkdir', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: path })
        }, { authRequired: true })
            .then(window.refreshFiles)
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(err.message);
                }
            });
    };

    window.triggerFileUpload = function() {
        if (state.upload.active) {
            return;
        }

        const input = document.getElementById('file-upload-input');
        input.value = '';
        input.click();
    };

    window.cancelUpload = function() {
        if (!state.upload.active) {
            return;
        }
        abortActiveUpload(false);
    };

    window.toggleHiddenFiles = function() {
        state.showHidden = !state.showHidden;
        updateHiddenToggle();
        renderFileList(state.files);
    };

    function handleFileUploadSelection(event) {
        const files = Array.from(event.target.files || []);
        if (!files.length || state.upload.active) {
            return;
        }

        uploadFiles(files)
            .then(function() {
                window.refreshFiles();
            })
            .catch(function(err) {
                return null;
            })
            .finally(function() {
                event.target.value = '';
            });
    }

    function parseXHRJSON(xhr) {
        if (xhr.response && typeof xhr.response === 'object') {
            return xhr.response;
        }
        if (!xhr.responseText) {
            return null;
        }

        try {
            return JSON.parse(xhr.responseText);
        } catch (_) {
            return null;
        }
    }

    async function confirmUploadedFileVisible(filename) {
        const path = state.currentPath || '~';
        const files = await fetchJSON('/api/files?path=' + encodeURIComponent(path), undefined, { authRequired: true });
        const visible = Array.isArray(files) && files.some(function(file) {
            return file && !file.isDir && file.name === filename;
        });

        if (visible) {
            applyDirectoryListing(path, files);
        }

        return visible;
    }

    function uploadFileWithFetchFallback(file) {
        return new Promise(async function(resolve, reject) {
            const controller = typeof AbortController === 'function' ? new AbortController() : null;
            const requestHandle = {
                abort: function() {
                    if (controller) {
                        controller.abort();
                    }
                }
            };
            const formData = new FormData();

            formData.append('file', file);
            state.upload.currentXHR = requestHandle;
            state.upload.cancelRequested = false;

            try {
                const response = await fetchAPI('/api/files/upload?path=' + encodeURIComponent(state.currentPath || '~'), {
                    method: 'POST',
                    body: formData,
                    signal: controller ? controller.signal : undefined
                }, { authRequired: true });
                let data = null;

                try {
                    data = await response.json();
                } catch (_) {
                    data = null;
                }

                if (state.upload.currentXHR === requestHandle) {
                    state.upload.currentXHR = null;
                }

                if (!response.ok) {
                    reject(buildRequestError(response, data, t('upload.failedFor', { name: file.name })));
                    return;
                }

                state.upload.currentFileLoaded = state.upload.currentFileSize;
                updateUploadProgress();
                resolve(data);
            } catch (err) {
                if (state.upload.currentXHR === requestHandle) {
                    state.upload.currentXHR = null;
                }
                if (err && err.name === 'AbortError') {
                    const error = new Error(t('upload.canceled'));
                    error.canceled = true;
                    error.silent = !state.upload.cancelRequested;
                    reject(error);
                    return;
                }
                reject(err);
            }
        });
    }

    function uploadFileWithProgress(file) {
        return new Promise(function(resolve, reject) {
            const formData = new FormData();
            const xhr = new XMLHttpRequest();
            let settled = false;
            let completionTimer = null;
            let internalAbort = false;

            function clearCompletionTimer() {
                if (completionTimer) {
                    clearTimeout(completionTimer);
                    completionTimer = null;
                }
            }

            function releaseRequestHandle() {
                clearCompletionTimer();
                if (state.upload.currentXHR === xhr) {
                    state.upload.currentXHR = null;
                }
            }

            function resolveOnce(data) {
                if (settled) {
                    return;
                }
                settled = true;
                releaseRequestHandle();
                resolve(data);
            }

            function rejectOnce(error) {
                if (settled) {
                    return;
                }
                settled = true;
                releaseRequestHandle();
                reject(error);
            }

            function scheduleCompletionFallback() {
                if (completionTimer || settled) {
                    return;
                }

                completionTimer = setTimeout(async function() {
                    if (settled) {
                        return;
                    }

                    try {
                        if (await confirmUploadedFileVisible(file.name)) {
                            state.upload.currentFileLoaded = state.upload.currentFileSize;
                            updateUploadProgress();
                            resolveOnce({ status: 'uploaded' });
                            return;
                        }
                    } catch (err) {
                        if (isHandledAuthError(err)) {
                            rejectOnce(err);
                            return;
                        }
                    }

                    internalAbort = true;
                    if (state.upload.currentXHR === xhr) {
                        state.upload.currentXHR = null;
                    }
                    try {
                        xhr.abort();
                    } catch (_) {
                        internalAbort = false;
                    }

                    uploadFileWithFetchFallback(file)
                        .then(resolveOnce)
                        .catch(rejectOnce);
                }, UPLOAD_RESPONSE_GRACE_MS);
            }

            formData.append('file', file);
            xhr.open('POST', withBasePath('/api/files/upload?path=' + encodeURIComponent(state.currentPath || '~')));
            xhr.responseType = 'json';
            state.upload.currentXHR = xhr;
            state.upload.cancelRequested = false;

            xhr.upload.onprogress = function(event) {
                if (event.lengthComputable) {
                    state.upload.currentFileLoaded = event.loaded;
                    state.upload.currentFileSize = event.total || file.size || 0;
                    updateUploadProgress();
                    if (event.total > 0 && event.loaded >= event.total) {
                        scheduleCompletionFallback();
                    }
                }
            };

            xhr.onload = function() {
                const data = parseXHRJSON(xhr);

                if (xhr.status === 401) {
                    const error = buildRequestError({ status: xhr.status }, data, t('auth.sessionExpired'));
                    error.authHandled = true;
                    handleUnauthorized(error.message);
                    rejectOnce(error);
                    return;
                }

                if (xhr.status < 200 || xhr.status >= 300) {
                    rejectOnce(buildRequestError({ status: xhr.status }, data, t('upload.failedFor', { name: file.name })));
                    return;
                }

                state.upload.currentFileLoaded = state.upload.currentFileSize;
                updateUploadProgress();
                resolveOnce(data);
            };

            xhr.onerror = function() {
                rejectOnce(new Error(t('upload.failedFor', { name: file.name })));
            };

            xhr.onabort = function() {
                if (internalAbort) {
                    internalAbort = false;
                    return;
                }
                const error = new Error(t('upload.canceled'));
                error.canceled = true;
                error.silent = !state.upload.cancelRequested;
                rejectOnce(error);
            };

            xhr.send(formData);
        });
    }

    async function uploadFiles(files) {
        const totalBytes = files.reduce(function(total, file) {
            return total + (file.size || 0);
        }, 0);

        clearUploadHideTimer();
        state.upload.active = true;
        state.upload.totalFiles = files.length;
        state.upload.totalBytes = totalBytes;
        state.upload.completedBytes = 0;
        state.upload.currentFileIndex = 0;
        state.upload.currentFileName = '';
        state.upload.currentFileLoaded = 0;
        state.upload.currentFileSize = 0;
        state.upload.currentXHR = null;
        state.upload.cancelRequested = false;
        setFileDropActive(false);
        setUploadButtonBusy(true);
        syncUploadCancelButton();

        try {
            for (let index = 0; index < files.length; index++) {
                const file = files[index];

                state.upload.currentFileIndex = index + 1;
                state.upload.currentFileName = file.name;
                state.upload.currentFileLoaded = 0;
                state.upload.currentFileSize = file.size || 0;
                updateUploadProgress();

                await uploadFileWithProgress(file);
                state.upload.completedBytes += file.size || 0;
            }

            showUploadComplete();
        } catch (err) {
            if (err && err.canceled) {
                if (err.silent) {
                    resetUploadState();
                } else {
                    showUploadCanceled();
                }
            } else if (!isHandledAuthError(err)) {
                showUploadError(err.message);
            } else {
                resetUploadState();
            }
            throw err;
        }
    }

    function updateHiddenToggle() {
        const btn = document.getElementById('hidden-btn');
        const label = document.getElementById('hidden-btn-label');
        const nextLabel = state.showHidden ? t('files.hideHidden') : t('files.showHidden');
        if (!btn) {
            return;
        }
        btn.classList.toggle('active', state.showHidden);
        btn.title = nextLabel;
        btn.setAttribute('aria-label', nextLabel);
        if (label) {
            label.textContent = nextLabel;
        }
    }

    function syncBodyViewState(name) {
        document.body.classList.toggle('view-login', name === 'login');
        document.body.classList.toggle('view-terminal', name === 'terminal');
    }

    // ========== Editor ==========

    function restoreEditorDrafts() {
        const payload = readEditorDraftPayload();
        const restored = [];
        const seen = {};

        if (!payload || !Array.isArray(payload.editors) || !payload.editors.length) {
            return;
        }
        if (state.editors.length > 0) {
            return;
        }

        payload.editors.forEach(function(candidate) {
            let editor;
            let path;

            path = candidate && typeof candidate.path === 'string' ? candidate.path.trim() : '';
            if (!path || seen[path]) {
                return;
            }

            editor = createEditorRecord(
                path,
                candidate && typeof candidate.name === 'string' ? candidate.name.trim() : baseName(path),
                candidate && typeof candidate.content === 'string' ? candidate.content : '',
                Boolean(candidate && candidate.isNew)
            );
            editor.savedContent = candidate && typeof candidate.savedContent === 'string'
                ? candidate.savedContent
                : '';
            editor.scrollTop = clampNumber(candidate && candidate.scrollTop, 0, 1 << 28, 0);
            editor.scrollLeft = clampNumber(candidate && candidate.scrollLeft, 0, 1 << 28, 0);
            editor.dirty = editor.isNew || editor.content !== editor.savedContent;

            if (!editor.dirty) {
                return;
            }

            seen[path] = true;
            restored.push(editor);
        });

        if (!restored.length) {
            clearEditorDraftStorage();
            return;
        }

        state.editors = restored;
        state.activeEditorPath = typeof payload.activePath === 'string' && seen[payload.activePath]
            ? payload.activePath
            : restored[restored.length - 1].path;
        applyEditorLayout();
        persistEditorDrafts();
    }

    function createEditorRecord(path, displayName, content, isNew) {
        const text = typeof content === 'string' ? content : '';

        return {
            path: path,
            name: displayName || baseName(path),
            content: text,
            savedContent: text,
            dirty: Boolean(isNew),
            saving: false,
            scrollTop: 0,
            scrollLeft: 0,
            isNew: Boolean(isNew)
        };
    }

    function createTransientEditor(path, displayName, content) {
        const existing = findEditor(path);
        let editor;

        if (existing) {
            activateEditor(path);
            return existing;
        }

        editor = createEditorRecord(path, displayName, content, true);
        state.editors.push(editor);
        state.activeEditorPath = path;
        state.editorPaneSuspended = false;
        applyEditorLayout();
        persistEditorDrafts();
        focusEditor();
        return editor;
    }

    async function openEditorPath(path, displayName) {
        const existing = findEditor(path);
        let editor;

        if (existing) {
            activateEditor(path);
            return existing;
        }

        const data = await fetchJSON('/api/files/read?path=' + encodeURIComponent(path), undefined, { authRequired: true });
        const content = typeof data.content === 'string' ? data.content : '';
        editor = createEditorRecord(path, displayName, content, false);

        state.editors.push(editor);
        state.activeEditorPath = path;
        state.editorPaneSuspended = false;
        applyEditorLayout();
        focusEditor();
        return editor;
    }

    function findEditor(path) {
        return state.editors.find(function(editor) {
            return editor.path === path;
        }) || null;
    }

    function getActiveEditor() {
        return findEditor(state.activeEditorPath);
    }

    function getSelectedEditorText() {
        const start = editorTextarea.selectionStart;
        const end = editorTextarea.selectionEnd;

        if (typeof start !== 'number' || typeof end !== 'number' || end <= start) {
            return '';
        }

        return editorTextarea.value.slice(start, end);
    }

    function getEditorSearchQuery() {
        return editorFindInput ? editorFindInput.value : '';
    }

    function getEditorReplaceText() {
        return editorReplaceInput ? editorReplaceInput.value : '';
    }

    function getEditorCursorPosition(text, index) {
        const value = String(text || '');
        const safeIndex = clampNumber(index, 0, value.length, 0);
        const before = value.slice(0, safeIndex);
        const lines = before.split('\n');

        return {
            line: lines.length,
            column: lines[lines.length - 1].length + 1
        };
    }

    function getEditorIndexForLineColumn(text, line, column) {
        const lines = String(text || '').split('\n');
        const safeLine = clampNumber(line, 1, Math.max(lines.length, 1), 1);
        let offset = 0;
        let index;
        const lineText = lines[safeLine - 1] || '';
        const safeColumn = clampNumber(column, 1, lineText.length + 1, 1);

        for (index = 0; index < safeLine - 1; index += 1) {
            offset += lines[index].length + 1;
        }

        return {
            line: safeLine,
            column: safeColumn,
            index: offset + safeColumn - 1
        };
    }

    function parseEditorLocationInput(value) {
        const match = String(value || '').trim().match(/^(\d+)(?:\s*[:.,]\s*(\d+))?$/);

        if (!match) {
            return null;
        }

        return {
            line: Number(match[1]),
            column: match[2] ? Number(match[2]) : 1
        };
    }

    function findEditorMatches(text, query) {
        const matches = [];
        let offset = 0;
        let foundAt;

        if (!query) {
            return matches;
        }

        while (offset <= text.length) {
            foundAt = text.indexOf(query, offset);
            if (foundAt === -1) {
                break;
            }
            matches.push({ start: foundAt, end: foundAt + query.length });
            offset = foundAt + Math.max(query.length, 1);
        }

        return matches;
    }

    function findSelectedEditorMatchIndex(matches, start, end) {
        let found = -1;

        matches.some(function(match, index) {
            if (match.start === start && match.end === end) {
                found = index;
                return true;
            }
            return false;
        });

        return found;
    }

    function updateEditorSearchCount() {
        if (!editorSearchCount) {
            return;
        }

        if (!state.editorSearchVisible || !getActiveEditor()) {
            editorSearchCount.textContent = '';
            return;
        }

        if (!getEditorSearchQuery()) {
            editorSearchCount.textContent = t('editor.searchHint');
            return;
        }

        if (!state.editorSearchMatches.length) {
            editorSearchCount.textContent = t('editor.searchNoResults');
            return;
        }

        editorSearchCount.textContent = t('editor.searchCount', {
            current: state.editorSearchCurrentIndex >= 0 ? state.editorSearchCurrentIndex + 1 : 0,
            total: state.editorSearchMatches.length
        });
    }

    function refreshEditorSearchState() {
        const editor = getActiveEditor();
        const query = getEditorSearchQuery();

        if (!editor || !query) {
            state.editorSearchMatches = [];
            state.editorSearchCurrentIndex = -1;
            updateEditorSearchCount();
            return [];
        }

        state.editorSearchMatches = findEditorMatches(editorTextarea.value, query);
        state.editorSearchCurrentIndex = findSelectedEditorMatchIndex(
            state.editorSearchMatches,
            editorTextarea.selectionStart,
            editorTextarea.selectionEnd
        );
        updateEditorSearchCount();
        return state.editorSearchMatches;
    }

    function updateEditorCursorStatus() {
        const editor = getActiveEditor();
        let position;

        if (!editor) {
            editorCaret.textContent = '';
            return;
        }

        position = getEditorCursorPosition(editorTextarea.value, editorTextarea.selectionStart);
        editorCaret.textContent = t('editor.cursorPosition', {
            line: position.line,
            column: position.column
        });
    }

    function syncEditorHighlightScroll() {
        if (!editorHighlight) {
            return;
        }
        editorHighlight.scrollTop = editorTextarea.scrollTop;
        editorHighlight.scrollLeft = editorTextarea.scrollLeft;
    }

    function renderEditorHighlight() {
        const editor = getActiveEditor();

        if (!editorHighlight) {
            return;
        }
        if (!editor) {
            editorHighlight.innerHTML = '';
            editorHighlight.scrollTop = 0;
            editorHighlight.scrollLeft = 0;
            return;
        }

        editorHighlight.innerHTML = highlightTextForPath(editor.content, editor.path);
        syncEditorHighlightScroll();
    }

    function scheduleEditorHighlightRefresh() {
        if (editorHighlightFrame) {
            window.cancelAnimationFrame(editorHighlightFrame);
        }
        editorHighlightFrame = window.requestAnimationFrame(function() {
            editorHighlightFrame = 0;
            renderEditorHighlight();
        });
    }

    function syncEditorLineNumberScroll() {
        if (!editorLineNumbers) {
            return;
        }
        editorLineNumbers.scrollTop = editorTextarea.scrollTop;
        syncEditorHighlightScroll();
    }

    function updateEditorLineNumbers() {
        const editor = getActiveEditor();
        let lineCount;
        let lines;
        let index;

        if (!editor) {
            editorLineNumbers.textContent = '';
            return;
        }

        lineCount = Math.max(1, editorTextarea.value.split('\n').length);
        lines = [];

        for (index = 1; index <= lineCount; index += 1) {
            lines.push(String(index));
        }

        editorLineNumbers.textContent = lines.join('\n');
        syncEditorLineNumberScroll();
    }

    function applyEditorViewOptions() {
        const hasEditor = Boolean(getActiveEditor());

        editorPane.classList.toggle('hide-line-numbers', !state.showEditorLineNumbers);
        if (editorLineNumbersBtn) {
            editorLineNumbersBtn.classList.toggle('active', state.showEditorLineNumbers);
            editorLineNumbersBtn.disabled = !hasEditor;
        }
        if (editorFindBtn) {
            editorFindBtn.disabled = !hasEditor;
        }
        if (editorGotoBtn) {
            editorGotoBtn.disabled = !hasEditor;
        }
        updateEditorLineNumbers();
    }

    function syncEditorSearchBar() {
        const hasEditor = Boolean(getActiveEditor());
        const visible = state.editorSearchVisible && hasEditor;

        editorSearchBar.style.display = visible ? 'flex' : 'none';
        editorFindBtn.classList.toggle('active', visible);
        if (!visible) {
            state.editorSearchMatches = [];
            state.editorSearchCurrentIndex = -1;
        }

        Array.from(editorSearchBar.querySelectorAll('input, button')).forEach(function(node) {
            node.disabled = !hasEditor;
        });

        updateEditorSearchCount();
    }

    function handleEditorSelectionChange() {
        updateEditorCursorStatus();
        refreshEditorSearchState();
    }

    function handleEditorSearchInput() {
        state.editorSearchVisible = true;
        syncEditorSearchBar();
        refreshEditorSearchState();
    }

    function handleEditorFindKeyDown(event) {
        if (event.key === 'Enter') {
            event.preventDefault();
            moveEditorMatch(event.shiftKey ? -1 : 1);
            return;
        }

        if (event.key === 'Escape') {
            event.preventDefault();
            window.closeEditorSearch();
        }
    }

    function handleEditorReplaceKeyDown(event) {
        if (event.key === 'Enter') {
            event.preventDefault();
            if (event.shiftKey) {
                window.replaceAllMatches();
            } else {
                window.replaceCurrentMatch();
            }
            return;
        }

        if (event.key === 'Escape') {
            event.preventDefault();
            window.closeEditorSearch();
        }
    }

    function handleEditorGoToLineKeyDown(event) {
        if (event.key === 'Enter') {
            event.preventDefault();
            window.goToEditorLine();
            return;
        }

        if (event.key === 'Escape') {
            event.preventDefault();
            window.closeEditorSearch();
        }
    }

    function setEditorSelection(start, end) {
        editorTextarea.focus();
        editorTextarea.setSelectionRange(start, end);
        handleEditorSelectionChange();
    }

    function scrollEditorToLine(line) {
        const lineHeight = parseFloat(window.getComputedStyle(editorTextarea).lineHeight) || 21;

        editorTextarea.scrollTop = Math.max(0, (line - 1) * lineHeight);
        syncEditorLineNumberScroll();
        rememberActiveEditorScroll();
    }

    function selectEditorMatch(index) {
        const match = state.editorSearchMatches[index];
        const location = match ? getEditorCursorPosition(editorTextarea.value, match.start) : null;

        if (!match || !location) {
            return false;
        }

        state.editorSearchCurrentIndex = index;
        scrollEditorToLine(location.line);
        setEditorSelection(match.start, match.end);
        return true;
    }

    function moveEditorMatch(direction) {
        const matches = refreshEditorSearchState();
        const start = editorTextarea.selectionStart;
        const end = editorTextarea.selectionEnd;
        let index;

        if (!matches.length) {
            return false;
        }

        if (direction < 0) {
            for (index = matches.length - 1; index >= 0; index -= 1) {
                if (matches[index].end <= start) {
                    return selectEditorMatch(index);
                }
            }
            return selectEditorMatch(matches.length - 1);
        }

        for (index = 0; index < matches.length; index += 1) {
            if (matches[index].start >= end) {
                return selectEditorMatch(index);
            }
        }

        return selectEditorMatch(0);
    }

    function openEditorSearch(focusTarget) {
        const editor = getActiveEditor();
        const selectedText = focusTarget === 'line' ? '' : getSelectedEditorText();
        let target = editorFindInput;

        if (!editor) {
            return;
        }

        if (selectedText) {
            editorFindInput.value = selectedText;
        }

        state.editorSearchVisible = true;
        syncEditorSearchBar();
        refreshEditorSearchState();

        if (focusTarget === 'replace') {
            target = editorReplaceInput;
        } else if (focusTarget === 'line') {
            target = editorGoToLineInput;
        }

        setTimeout(function() {
            target.focus();
            target.select();
        }, 0);
    }

    function goToEditorLocation(rawValue) {
        const editor = getActiveEditor();
        const parsed = parseEditorLocationInput(rawValue);
        let location;

        if (!editor || !parsed) {
            return false;
        }

        location = getEditorIndexForLineColumn(editorTextarea.value, parsed.line, parsed.column);
        scrollEditorToLine(location.line);
        setEditorSelection(location.index, location.index);
        return true;
    }

    window.toggleEditorLineNumbers = function() {
        state.showEditorLineNumbers = !state.showEditorLineNumbers;
        persistEditorUIPreferences();
        applyEditorViewOptions();
        focusEditor();
    };

    window.openEditorFind = function() {
        openEditorSearch('find');
    };

    window.openEditorReplace = function() {
        openEditorSearch('replace');
    };

    window.openEditorGoToLine = function() {
        openEditorSearch('line');
    };

    window.closeEditorSearch = function() {
        state.editorSearchVisible = false;
        syncEditorSearchBar();
        focusEditor();
    };

    window.findNextMatch = function() {
        moveEditorMatch(1);
    };

    window.findPreviousMatch = function() {
        moveEditorMatch(-1);
    };

    window.replaceCurrentMatch = function() {
        const editor = getActiveEditor();
        let matches;
        let selectedIndex;
        let start;
        let end;
        let nextValue;

        if (!editor || !getEditorSearchQuery()) {
            return;
        }

        matches = refreshEditorSearchState();
        selectedIndex = findSelectedEditorMatchIndex(matches, editorTextarea.selectionStart, editorTextarea.selectionEnd);
        if (selectedIndex === -1 && !moveEditorMatch(1)) {
            return;
        }

        matches = refreshEditorSearchState();
        selectedIndex = findSelectedEditorMatchIndex(matches, editorTextarea.selectionStart, editorTextarea.selectionEnd);
        if (selectedIndex === -1) {
            return;
        }

        start = editorTextarea.selectionStart;
        end = editorTextarea.selectionEnd;
        nextValue = editorTextarea.value.slice(0, start) + getEditorReplaceText() + editorTextarea.value.slice(end);
        editorTextarea.value = nextValue;
        editorTextarea.setSelectionRange(start, start + getEditorReplaceText().length);
        handleEditorInput();
        handleEditorSelectionChange();
    };

    window.replaceAllMatches = function() {
        const editor = getActiveEditor();
        const query = getEditorSearchQuery();
        const replacement = getEditorReplaceText();
        const matches = refreshEditorSearchState();

        if (!editor || !query || !matches.length) {
            return;
        }

        editorTextarea.value = editorTextarea.value.split(query).join(replacement);
        editorTextarea.setSelectionRange(0, 0);
        handleEditorInput();
        handleEditorSelectionChange();
    };

    window.goToEditorLine = function() {
        if (!goToEditorLocation(editorGoToLineInput.value)) {
            editorGoToLineInput.focus();
            editorGoToLineInput.select();
            return;
        }

        focusEditor();
    };

    function activateEditor(path) {
        rememberActiveEditorScroll();
        state.activeEditorPath = path;
        state.editorPaneSuspended = false;
        if (state.previewEditMode && path) {
            state.previewImagePath = path;
            state.previewImageName = baseName(path);
            state.previewContentType = 'text';
        }
        renderEditorTabs();
        syncActiveEditor(true);
        if (state.previewEditMode) {
            applyEditorLayout();
            renderViewerPanel();
        }
        persistEditorDrafts();
        focusEditor();
    }

    function handleEditorInput() {
        const editor = getActiveEditor();
        if (!editor) {
            return;
        }

        editor.content = editorTextarea.value;
        editor.scrollTop = editorTextarea.scrollTop;
        editor.dirty = editor.isNew || editor.content !== editor.savedContent;

        renderEditorTabs();
        updateEditorChrome();
        updateEditorLineNumbers();
        scheduleEditorHighlightRefresh();
        handleEditorSelectionChange();
        persistEditorDrafts();
    }

    function handleEditorTextareaKeyDown(e) {
        if (e.key !== 'Tab' || e.ctrlKey || e.metaKey || e.altKey) {
            return;
        }

        e.preventDefault();
        const start = editorTextarea.selectionStart;
        const end = editorTextarea.selectionEnd;
        const value = editorTextarea.value;

        editorTextarea.value = value.slice(0, start) + '\t' + value.slice(end);
        editorTextarea.selectionStart = start + 1;
        editorTextarea.selectionEnd = start + 1;
        handleEditorInput();
    }

    function rememberActiveEditorScroll() {
        const editor = getActiveEditor();
        if (!editor || editorTextarea.dataset.path !== editor.path) {
            return;
        }
        editor.scrollTop = editorTextarea.scrollTop;
        editor.scrollLeft = editorTextarea.scrollLeft;
        syncEditorLineNumberScroll();
        persistEditorDrafts();
    }

    function renderEditorTabs() {
        editorTabs.innerHTML = '';

        state.editors.forEach(function(editor) {
            const tab = document.createElement('div');
            tab.className = 'editor-tab' + (editor.path === state.activeEditorPath ? ' active' : '');
            tab.onclick = function(e) {
                if (!e.target.classList.contains('tab-close')) {
                    activateEditor(editor.path);
                }
            };

            const name = document.createElement('span');
            name.className = 'editor-tab-name';
            name.textContent = editor.name;
            tab.appendChild(name);

            if (editor.dirty) {
                const dirty = document.createElement('span');
                dirty.className = 'editor-tab-dirty';
                dirty.textContent = '\u2022';
                tab.appendChild(dirty);
            }

            const close = document.createElement('span');
            close.className = 'tab-close';
            close.textContent = '\u00d7';
            close.onclick = function(e) {
                e.stopPropagation();
                closeEditor(editor.path);
            };
            tab.appendChild(close);

            editorTabs.appendChild(tab);
        });
    }

    function syncActiveEditor(forceValue) {
        const editor = getActiveEditor();
        if (!editor) {
            editorTextarea.value = '';
            editorTextarea.dataset.path = '';
            editorLineNumbers.textContent = '';
            state.editorSearchMatches = [];
            state.editorSearchCurrentIndex = -1;
            renderEditorHighlight();
            updateEditorChrome();
            return;
        }

        if (forceValue || editorTextarea.dataset.path !== editor.path) {
            editorTextarea.value = editor.content;
            editorTextarea.dataset.path = editor.path;
            editorTextarea.scrollTop = editor.scrollTop || 0;
            editorTextarea.scrollLeft = editor.scrollLeft || 0;
        }

        updateEditorLineNumbers();
        renderEditorHighlight();
        handleEditorSelectionChange();
        updateEditorChrome();
    }

    function updateEditorChrome() {
        const editor = getActiveEditor();
        const saveAsBtn = document.getElementById('editor-save-as-btn');
        const editorMaxBtn = document.getElementById('editor-max-btn');
        let statusText = '';
        let statusState = '';

        if (!editor) {
            editorPath.textContent = t('editor.noFileOpen');
            editorCaret.textContent = '';
            editorStatus.textContent = '';
            editorStatus.dataset.state = '';
            editorSaveBtn.classList.remove('active');
            editorSaveBtn.disabled = true;
            if (saveAsBtn) {
                saveAsBtn.disabled = true;
            }
            if (editorMaxBtn) {
                editorMaxBtn.disabled = true;
            }
            applyEditorViewOptions();
            syncEditorSearchBar();
            return;
        }

        if (editor.saving) {
            statusText = t('editor.saving');
            statusState = 'saving';
        } else if (editor.dirty) {
            statusText = t('editor.modified');
            statusState = 'dirty';
        } else {
            statusText = t('editor.saved');
            statusState = 'saved';
        }

        editorPath.textContent = editor.path;
        editorStatus.textContent = statusText;
        editorStatus.dataset.state = statusState;
        editorSaveBtn.classList.toggle('active', editor.dirty || editor.saving);
        editorSaveBtn.disabled = editor.saving;
        if (saveAsBtn) {
            saveAsBtn.disabled = editor.saving;
        }
        if (editorMaxBtn) {
            editorMaxBtn.disabled = shouldEmbedViewerEditor();
        }
        applyEditorViewOptions();
        syncEditorSearchBar();
    }

    function applyEditorLayout() {
        const embedInViewer = shouldEmbedViewerEditor();

        if (state.editors.length === 0) {
            restoreEditorPaneHost();
            editorPane.style.display = 'none';
            splitter.style.display = 'none';
            state.activeEditorPath = null;
            state.editorPaneSuspended = false;
            editorTabs.innerHTML = '';
            syncActiveEditor(false);
            if (state.maximized === 'editor') {
                state.maximized = 'terminal';
            }
            applyMaximizeState();
            scheduleFitActiveTerminal();
            return;
        }

        if (state.editorPaneSuspended && !embedInViewer) {
            restoreEditorPaneHost();
            editorPane.style.display = 'none';
            splitter.style.display = 'none';
            state.activeEditorPath = null;
            renderEditorTabs();
            syncActiveEditor(false);
            if (state.maximized === 'editor') {
                state.maximized = 'terminal';
            }
            applyMaximizeState();
            scheduleFitActiveTerminal();
            return;
        }

        if (!getActiveEditor()) {
            state.activeEditorPath = state.editors[state.editors.length - 1].path;
        }

        // When opening a file while terminal is maximized, switch to split view
        if (state.maximized === 'terminal') {
            state.maximized = null;
        }
        if (embedInViewer && state.maximized === 'editor') {
            state.maximized = null;
        }

        syncEmbeddedEditorHost();
        editorPane.style.display = 'flex';
        if (embedInViewer) {
            splitter.style.display = 'none';
            editorPane.style.height = 'auto';
        } else {
            splitter.style.display = 'block';
            clampEditorHeight();
            editorPane.style.height = state.editorHeight + 'px';
        }

        renderEditorTabs();
        syncActiveEditor(true);
        applyMaximizeState();
        scheduleFitActiveTerminal();
    }

    function clampEditorHeight() {
        if (state.editors.length === 0) {
            return;
        }

        const splitterHeight = splitter.offsetHeight || 10;
        const maxHeight = Math.max(80, workspace.clientHeight - MIN_TERMINAL_HEIGHT - splitterHeight);
        const minHeight = Math.min(MIN_EDITOR_HEIGHT, maxHeight);
        state.editorHeight = Math.max(minHeight, Math.min(state.editorHeight, maxHeight));
    }

    function focusEditor() {
        setTimeout(function() {
            editorTextarea.focus();
        }, 30);
    }

    function closeEditor(path) {
        const editor = findEditor(path);
        const closingEmbeddedViewerEditor = shouldEmbedViewerEditor() && state.previewImagePath === path;
        if (!editor) {
            return false;
        }

        if (editor.dirty && !confirm(t('editor.closeWithoutSaving', { name: editor.name }))) {
            return false;
        }

        const index = state.editors.findIndex(function(item) {
            return item.path === path;
        });
        if (index === -1) {
            return false;
        }

        state.editors.splice(index, 1);

        if (closingEmbeddedViewerEditor) {
            state.editorPaneSuspended = state.editors.length > 0;
            state.activeEditorPath = null;
        } else if (state.activeEditorPath === path) {
            const fallback = state.editors[index] || state.editors[index - 1] || null;
            state.activeEditorPath = fallback ? fallback.path : null;
        }

        if (closingEmbeddedViewerEditor) {
            resetViewerPreviewState();
        } else if (state.previewImagePath === path && state.previewEditMode) {
            state.previewEditMode = false;
        }

        persistEditorDrafts();
        applyEditorLayout();
        renderViewerPanel();

        if (state.activeEditorPath) {
            focusEditor();
        } else {
            scheduleFitActiveTerminal();
        }

        return true;
    }

    function resetEditorState() {
        state.editors = [];
        state.activeEditorPath = null;
        state.editorPaneSuspended = false;
        state.editorSearchVisible = false;
        state.editorSearchMatches = [];
        state.editorSearchCurrentIndex = -1;
        editorTextarea.value = '';
        editorTextarea.dataset.path = '';
        editorLineNumbers.textContent = '';
        applyEditorLayout();
    }

    window.closeActiveEditor = function() {
        if (state.activeEditorPath) {
            closeEditor(state.activeEditorPath);
        }
    };

    function saveEditorToPath(editor, targetPath) {
        const nextPath = targetPath || (editor && editor.path) || '';
        let previousPath;
        let conflictingEditor;

        if (!editor || editor.saving || !nextPath) {
            return Promise.resolve();
        }

        conflictingEditor = findEditor(nextPath);
        if (conflictingEditor && conflictingEditor !== editor) {
            alert(t('editor.pathAlreadyOpen', { path: nextPath }));
            return Promise.resolve();
        }

        if (isKnownDirectoryPath(nextPath, editor.path)) {
            alert(t('files.cannotOverwriteDirectory', { path: nextPath }));
            return Promise.resolve();
        }

        if (editorTextarea.dataset.path === editor.path) {
            editor.content = editorTextarea.value;
            editor.scrollTop = editorTextarea.scrollTop;
        }

        previousPath = editor.path;
        editor.saving = true;
        updateEditorChrome();
        renderEditorTabs();

        return fetchJSON('/api/files/save', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                path: nextPath,
                content: editor.content
            })
        }, { authRequired: true })
            .then(function() {
                if (nextPath !== previousPath) {
                    editor.path = nextPath;
                    editor.name = baseName(nextPath);
                    if (state.activeEditorPath === previousPath) {
                        state.activeEditorPath = nextPath;
                    }
                    if (state.previewImagePath === previousPath) {
                        state.previewImagePath = nextPath;
                        state.previewImageName = baseName(nextPath);
                        state.previewImageVersion = String(Date.now());
                        renderViewerPanel(true);
                    }
                }
                editor.savedContent = editor.content;
                editor.isNew = false;
                editor.dirty = false;
                persistEditorDrafts();
                if (state.fileBrowserOpen && state.currentPath && (isSameDirectory(state.currentPath, previousPath) || isSameDirectory(state.currentPath, nextPath))) {
                    window.refreshFiles();
                }
            })
            .catch(function(err) {
                if (!isHandledAuthError(err)) {
                    alert(t('editor.saveFailed', { message: err.message }));
                }
            })
            .finally(function() {
                editor.saving = false;
                updateEditorChrome();
                renderEditorTabs();
                syncActiveEditor(true);
            });
    };

    window.saveActiveEditor = function() {
        return saveEditorToPath(getActiveEditor(), state.activeEditorPath);
    };

    window.saveActiveEditorAs = function() {
        const editor = getActiveEditor();
        const initialPath = editor ? editor.path : '';
        const rawPath = prompt(t('editor.saveAsPrompt'), initialPath);
        let nextPath;

        if (!editor || editor.saving || rawPath === null) {
            return Promise.resolve();
        }

        nextPath = normalizePromptPath(rawPath, parentPath(initialPath || (state.currentPath || '~')));
        if (!nextPath) {
            return Promise.resolve();
        }

        return saveEditorToPath(editor, nextPath);
    };

    function beginEmbeddedFileBrowserDrag(e) {
        if (!shouldEmbedFileBrowser() || !filePanel || !filePanelResizer || filePanelResizer.classList.contains('disabled')) {
            return;
        }

        state.embeddedFileBrowserDrag = {
            pointerId: e.pointerId,
            startX: e.clientX,
            startWidth: filePanel.getBoundingClientRect().width,
            width: filePanel.getBoundingClientRect().width
        };

        filePanelResizer.classList.add('dragging');
        document.body.style.userSelect = 'none';
        document.body.style.cursor = 'col-resize';
        if (filePanelResizer.setPointerCapture) {
            filePanelResizer.setPointerCapture(e.pointerId);
        }
        e.preventDefault();
    }

    function handleEmbeddedFileBrowserDrag(e) {
        const drag = state.embeddedFileBrowserDrag;
        let nextWidth;

        if (!drag || drag.pointerId !== e.pointerId || !terminalContainer) {
            return;
        }

        nextWidth = clampEmbeddedFileBrowserWidth(drag.startWidth + (drag.startX - e.clientX));
        drag.width = nextWidth;
        terminalContainer.style.setProperty('--embedded-file-panel-width', nextWidth + 'px');
        scheduleFitActiveTerminal();
    }

    function endEmbeddedFileBrowserDrag(e) {
        const drag = state.embeddedFileBrowserDrag;

        if (!drag || drag.pointerId !== e.pointerId) {
            return;
        }

        state.embeddedFileBrowserDrag = null;
        state.embeddedFileBrowserWidth = clampEmbeddedFileBrowserWidth(drag.width || drag.startWidth);
        persistEditorUIPreferences();
        if (filePanelResizer) {
            filePanelResizer.classList.remove('dragging');
            if (filePanelResizer.releasePointerCapture) {
                filePanelResizer.releasePointerCapture(e.pointerId);
            }
        }
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        syncEmbeddedFileBrowserWidth();
        scheduleFitActiveTerminal();
    }

    function beginEditorDrag(e) {
        if (state.editors.length === 0) {
            return;
        }

        state.editorDrag = {
            pointerId: e.pointerId,
            startY: e.clientY,
            startHeight: editorPane.getBoundingClientRect().height
        };

        document.body.style.userSelect = 'none';
        document.body.style.cursor = 'row-resize';
        if (splitter.setPointerCapture) {
            splitter.setPointerCapture(e.pointerId);
        }
        e.preventDefault();
    }

    function handleEditorDrag(e) {
        if (!state.editorDrag || state.editorDrag.pointerId !== e.pointerId) {
            return;
        }

        const delta = e.clientY - state.editorDrag.startY;
        state.editorHeight = state.editorDrag.startHeight + delta;
        clampEditorHeight();
        editorPane.style.height = state.editorHeight + 'px';
        scheduleFitActiveTerminal();
    }

    function endEditorDrag(e) {
        if (!state.editorDrag || state.editorDrag.pointerId !== e.pointerId) {
            return;
        }

        state.editorDrag = null;
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        if (splitter.releasePointerCapture) {
            splitter.releasePointerCapture(e.pointerId);
        }
        scheduleFitActiveTerminal();
    }

    // ========== Helpers ==========

    function joinPath(base, name) {
        if (!base || base === '~') {
            return '~/' + name;
        }
        if (base === '/') {
            return '/' + name;
        }
        return base.replace(/\/+$/, '') + '/' + name;
    }

    function parentPath(path) {
        const trimmed = path.replace(/\/+$/, '');
        if (!trimmed || trimmed === '/' || trimmed === '~') {
            return trimmed || '/';
        }
        const index = trimmed.lastIndexOf('/');
        if (index <= 0) {
            return trimmed.charAt(0) === '~' ? '~' : '/';
        }
        return trimmed.slice(0, index);
    }

    function baseName(path) {
        const trimmed = path.replace(/\/+$/, '');
        const index = trimmed.lastIndexOf('/');
        return index === -1 ? trimmed : trimmed.slice(index + 1);
    }

    function suggestCopyPath(path) {
        const dir = parentPath(path);
        const name = baseName(path);
        const dot = name.lastIndexOf('.');
        let nextName;

        if (dot > 0) {
            nextName = name.slice(0, dot) + ' copy' + name.slice(dot);
        } else {
            nextName = name + ' copy';
        }

        return joinPath(dir, nextName);
    }

    function normalizePromptPath(rawPath, basePath) {
        const input = String(rawPath || '').trim();

        if (!input) {
            return '';
        }
        if (input === '~' || input === '/' || input.indexOf('~/') === 0 || input.charAt(0) === '/') {
            return input;
        }
        return joinPath(basePath || '~', input);
    }

    function findVisibleFileForPath(path) {
        if (!path || !state.currentPath || parentPath(path) !== state.currentPath) {
            return null;
        }

        return state.files.find(function(file) {
            return file.name === baseName(path);
        }) || null;
    }

    function isKnownDirectoryPath(path, ignorePath) {
        const file = findVisibleFileForPath(path);

        if (!file || !file.isDir) {
            return false;
        }

        return !ignorePath || joinPath(state.currentPath, file.name) !== ignorePath;
    }

    function syncPathConsumersAfterRename(oldPath, newPath) {
        const editor = findEditor(oldPath);

        if (editor) {
            editor.path = newPath;
            editor.name = baseName(newPath);
            editor.isNew = false;
            if (state.activeEditorPath === oldPath) {
                state.activeEditorPath = newPath;
            }
        }

        if (state.previewImagePath === oldPath) {
            state.previewImagePath = newPath;
            state.previewImageName = baseName(newPath);
            state.previewImageVersion = String(Date.now());
            renderViewerPanel(true);
        }

        renderEditorTabs();
        syncActiveEditor(true);
        persistEditorDrafts();
    }

    function isSameDirectory(dir, path) {
        if (!dir) {
            return false;
        }
        return parentPath(path) === dir;
    }

    const SYNTAX_HIGHLIGHT_MAX_CHARS = 240000;
    const SYNTAX_LANGUAGE_ALIASES = {
        md: 'markdown',
        markdown: 'markdown',
        rmd: 'markdown',
        py: 'python',
        pyi: 'python',
        js: 'javascript',
        jsx: 'javascript',
        mjs: 'javascript',
        cjs: 'javascript',
        ts: 'typescript',
        tsx: 'typescript',
        json: 'json',
        jsonc: 'json',
        yaml: 'yaml',
        yml: 'yaml',
        toml: 'toml',
        sh: 'shell',
        bash: 'shell',
        zsh: 'shell',
        fish: 'shell',
        go: 'go',
        css: 'css',
        scss: 'css',
        less: 'css',
        sass: 'css',
        html: 'html',
        htm: 'html',
        xml: 'html',
        svg: 'html',
        txt: 'plaintext',
        text: 'plaintext',
        log: 'plaintext',
        ini: 'yaml',
        cfg: 'yaml',
        conf: 'yaml',
        r: 'r',
        rs: 'rust',
        java: 'clike',
        c: 'clike',
        cc: 'clike',
        cpp: 'clike',
        cxx: 'clike',
        h: 'clike',
        hpp: 'clike',
        sql: 'sql'
    };
    const SYNTAX_PUNCTUATION = new Set(['{', '}', '[', ']', '(', ')', ';', ',', '.', ':']);
    const SYNTAX_OPERATOR_CHARS = new Set(['+', '-', '*', '/', '%', '=', '!', '?', '&', '|', '^', '~', '<', '>']);
    const GENERIC_SYNTAX_CONFIGS = {
        javascript: {
            lineComments: ['//'],
            blockComment: ['/*', '*/'],
            keywords: new Set(['break', 'case', 'catch', 'class', 'const', 'continue', 'debugger', 'default', 'delete', 'do', 'else', 'export', 'extends', 'finally', 'for', 'function', 'if', 'import', 'in', 'instanceof', 'let', 'new', 'return', 'super', 'switch', 'this', 'throw', 'try', 'typeof', 'var', 'void', 'while', 'with', 'yield', 'async', 'await', 'from', 'of']),
            booleans: new Set(['true', 'false']),
            nulls: new Set(['null', 'undefined', 'NaN', 'Infinity']),
            allowTemplateStrings: true
        },
        typescript: {
            lineComments: ['//'],
            blockComment: ['/*', '*/'],
            keywords: new Set(['abstract', 'as', 'async', 'await', 'break', 'case', 'catch', 'class', 'const', 'constructor', 'continue', 'debugger', 'declare', 'default', 'delete', 'do', 'else', 'enum', 'export', 'extends', 'finally', 'for', 'from', 'function', 'get', 'if', 'implements', 'import', 'in', 'infer', 'instanceof', 'interface', 'is', 'keyof', 'let', 'module', 'namespace', 'new', 'of', 'override', 'private', 'protected', 'public', 'readonly', 'return', 'set', 'static', 'super', 'switch', 'this', 'throw', 'try', 'type', 'typeof', 'var', 'void', 'while', 'with', 'yield']),
            booleans: new Set(['true', 'false']),
            nulls: new Set(['null', 'undefined', 'never', 'unknown']),
            allowTemplateStrings: true
        },
        python: {
            lineComments: ['#'],
            keywords: new Set(['and', 'as', 'assert', 'async', 'await', 'break', 'class', 'continue', 'def', 'del', 'elif', 'else', 'except', 'finally', 'for', 'from', 'global', 'if', 'import', 'in', 'is', 'lambda', 'nonlocal', 'not', 'or', 'pass', 'raise', 'return', 'try', 'while', 'with', 'yield']),
            booleans: new Set(['True', 'False']),
            nulls: new Set(['None']),
            tripleQuotes: true,
            decorators: true
        },
        shell: {
            lineComments: ['#'],
            keywords: new Set(['case', 'coproc', 'do', 'done', 'elif', 'else', 'esac', 'fi', 'for', 'function', 'if', 'in', 'select', 'then', 'time', 'until', 'while', 'local', 'export', 'readonly', 'trap']),
            booleans: new Set([]),
            nulls: new Set([]),
            variables: true
        },
        go: {
            lineComments: ['//'],
            blockComment: ['/*', '*/'],
            keywords: new Set(['break', 'case', 'chan', 'const', 'continue', 'default', 'defer', 'else', 'fallthrough', 'for', 'func', 'go', 'goto', 'if', 'import', 'interface', 'map', 'package', 'range', 'return', 'select', 'struct', 'switch', 'type', 'var']),
            booleans: new Set(['true', 'false']),
            nulls: new Set(['nil'])
        },
        r: {
            lineComments: ['#'],
            keywords: new Set(['break', 'else', 'for', 'function', 'if', 'in', 'next', 'repeat', 'return', 'while']),
            booleans: new Set(['TRUE', 'FALSE', 'T', 'F']),
            nulls: new Set(['NULL', 'NA', 'NaN', 'Inf'])
        },
        rust: {
            lineComments: ['//'],
            blockComment: ['/*', '*/'],
            keywords: new Set(['as', 'async', 'await', 'break', 'const', 'continue', 'crate', 'else', 'enum', 'extern', 'fn', 'for', 'if', 'impl', 'in', 'let', 'loop', 'match', 'mod', 'move', 'mut', 'pub', 'ref', 'return', 'self', 'Self', 'static', 'struct', 'super', 'trait', 'type', 'unsafe', 'use', 'where', 'while']),
            booleans: new Set(['true', 'false']),
            nulls: new Set(['None'])
        },
        clike: {
            lineComments: ['//'],
            blockComment: ['/*', '*/'],
            keywords: new Set(['auto', 'break', 'case', 'catch', 'char', 'class', 'const', 'continue', 'default', 'delete', 'do', 'double', 'else', 'enum', 'extern', 'final', 'float', 'for', 'goto', 'if', 'inline', 'int', 'long', 'namespace', 'new', 'operator', 'private', 'protected', 'public', 'register', 'return', 'short', 'signed', 'sizeof', 'static', 'struct', 'switch', 'template', 'this', 'throw', 'try', 'typedef', 'typename', 'union', 'unsigned', 'virtual', 'void', 'volatile', 'while']),
            booleans: new Set(['true', 'false']),
            nulls: new Set(['nullptr', 'null'])
        },
        sql: {
            lineComments: ['--'],
            blockComment: ['/*', '*/'],
            keywords: new Set(['add', 'alter', 'and', 'as', 'asc', 'begin', 'between', 'by', 'case', 'commit', 'create', 'delete', 'desc', 'distinct', 'drop', 'else', 'end', 'exists', 'from', 'group', 'having', 'in', 'insert', 'into', 'is', 'join', 'left', 'like', 'limit', 'not', 'null', 'offset', 'on', 'or', 'order', 'outer', 'primary', 'right', 'rollback', 'select', 'set', 'table', 'then', 'union', 'update', 'values', 'when', 'where']),
            booleans: new Set(['true', 'false']),
            nulls: new Set(['null'])
        },
        css: {
            lineComments: [],
            blockComment: ['/*', '*/'],
            keywords: new Set(['auto', 'block', 'center', 'flex', 'grid', 'inherit', 'inline', 'none', 'relative', 'absolute', 'fixed', 'sticky', 'solid', 'dashed', 'repeat', 'var']),
            booleans: new Set([]),
            nulls: new Set([]),
            atIdentifiers: true,
            hexColors: true
        },
        data: {
            lineComments: ['#'],
            keywords: new Set([]),
            booleans: new Set(['true', 'false', 'yes', 'no', 'on', 'off']),
            nulls: new Set(['null', 'none', '~'])
        }
    };

    function normalizeSyntaxLanguage(language) {
        const candidate = String(language || '').trim().toLowerCase();
        return SYNTAX_LANGUAGE_ALIASES[candidate] || candidate || 'plaintext';
    }

    const KNOWN_TEXT_BASENAMES = new Set([
        'dockerfile',
        'makefile',
        '.envrc',
        '.gitignore',
        '.gitattributes',
        '.gitmodules',
        '.dockerignore',
        '.editorconfig',
        '.npmrc',
        '.yarnrc',
        '.npmignore',
        '.prettierignore',
        '.eslintignore',
        '.stylelintignore',
        '.babelrc',
        '.browserslistrc',
        '.nvmrc',
        '.bashrc',
        '.bash_profile',
        '.bash_aliases',
        '.bash_logout',
        '.zshrc',
        '.profile',
        '.tmux.conf',
        'license',
        'copying'
    ]);

    const SHELL_STYLE_TEXT_BASENAMES = new Set([
        'dockerfile',
        'makefile',
        '.envrc',
        '.bashrc',
        '.bash_profile',
        '.bash_aliases',
        '.bash_logout',
        '.zshrc',
        '.profile',
        '.tmux.conf'
    ]);

    const DATA_STYLE_TEXT_BASENAMES = new Set([
        '.editorconfig',
        '.npmrc',
        '.yarnrc',
        '.nvmrc'
    ]);

    function normalizeKnownTextFilename(path) {
        return baseName(path || '').trim().toLowerCase();
    }

    function isDotEnvLikeFilename(path) {
        return /^\.env(?:\..+)?$/i.test(normalizeKnownTextFilename(path));
    }

    function isKnownTextFilename(path) {
        const name = normalizeKnownTextFilename(path);

        if (!name) {
            return false;
        }
        if (name === 'readme' || name.startsWith('readme.')) {
            return true;
        }
        if (isDotEnvLikeFilename(name)) {
            return true;
        }

        return KNOWN_TEXT_BASENAMES.has(name);
    }

    function getSyntaxLanguageForPath(path) {
        const name = normalizeKnownTextFilename(path);
        const ext = name.indexOf('.') >= 0 ? name.slice(name.lastIndexOf('.') + 1) : '';

        if (!name) {
            return 'plaintext';
        }
        if (isDotEnvLikeFilename(name) || SHELL_STYLE_TEXT_BASENAMES.has(name)) {
            return 'shell';
        }
        if (DATA_STYLE_TEXT_BASENAMES.has(name)) {
            return 'data';
        }
        if (name === 'readme' || name.startsWith('readme.')) {
            return 'markdown';
        }

        return normalizeSyntaxLanguage(ext || name);
    }

    function escapeHTML(value) {
        return String(value || '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }

    function wrapSyntaxToken(type, value) {
        if (!value) {
            return '';
        }
        return '<span class="syntax-' + type + '">' + escapeHTML(value) + '</span>';
    }

    function isIdentifierStart(ch) {
        return /[A-Za-z_]/.test(ch || '');
    }

    function isIdentifierPart(ch) {
        return /[A-Za-z0-9_$]/.test(ch || '');
    }

    function readIdentifier(text, start) {
        let index = start;

        while (index < text.length && isIdentifierPart(text[index])) {
            index += 1;
        }

        return index;
    }

    function readNumberLiteral(text, start) {
        let index = start;

        if (text[index] === '0' && /[xXbBoO]/.test(text[index + 1] || '')) {
            index += 2;
            while (index < text.length && /[0-9A-Fa-f_]/.test(text[index])) {
                index += 1;
            }
            return index;
        }

        while (index < text.length && /[0-9_]/.test(text[index])) {
            index += 1;
        }
        if (text[index] === '.' && /[0-9]/.test(text[index + 1] || '')) {
            index += 1;
            while (index < text.length && /[0-9_]/.test(text[index])) {
                index += 1;
            }
        }
        if (/[eE]/.test(text[index] || '')) {
            index += 1;
            if (/[+-]/.test(text[index] || '')) {
                index += 1;
            }
            while (index < text.length && /[0-9_]/.test(text[index])) {
                index += 1;
            }
        }

        return index;
    }

    function findLineEnd(text, start) {
        const index = text.indexOf('\n', start);
        return index === -1 ? text.length : index;
    }

    function findNonWhitespaceIndex(text, start) {
        let index = start;

        while (index < text.length && /\s/.test(text[index])) {
            index += 1;
        }

        return index;
    }

    function readQuotedString(text, start, config) {
        const quote = text[start];
        let index = start + 1;

        if ((quote === '"' || quote === '\'') && config && config.tripleQuotes && text.slice(start, start + 3) === quote.repeat(3)) {
            index = start + 3;
            while (index < text.length) {
                if (text.slice(index, index + 3) === quote.repeat(3)) {
                    return index + 3;
                }
                if (text[index] === '\\') {
                    index += 2;
                    continue;
                }
                index += 1;
            }
            return text.length;
        }

        while (index < text.length) {
            if (text[index] === '\\') {
                index += 2;
                continue;
            }
            if (text[index] === quote) {
                return index + 1;
            }
            if (quote !== '`' && (text[index] === '\n' || text[index] === '\r')) {
                return index;
            }
            index += 1;
        }

        return text.length;
    }

    function readShellVariable(text, start) {
        let index = start + 1;

        if (text[index] === '{') {
            index += 1;
            while (index < text.length && text[index] !== '}') {
                index += 1;
            }
            return index < text.length ? index + 1 : text.length;
        }
        if (/[0-9@*#?$!-]/.test(text[index] || '')) {
            return index + 1;
        }
        if (!isIdentifierStart(text[index])) {
            return start + 1;
        }

        while (index < text.length && /[A-Za-z0-9_]/.test(text[index])) {
            index += 1;
        }
        return index;
    }

    function highlightGenericCode(text, config) {
        let html = '';
        let index = 0;
        let prefix;
        let end;
        let word;
        let lowerWord;
        let nextIndex;

        while (index < text.length) {
            if (config.blockComment && text.startsWith(config.blockComment[0], index)) {
                end = text.indexOf(config.blockComment[1], index + config.blockComment[0].length);
                end = end === -1 ? text.length : end + config.blockComment[1].length;
                html += wrapSyntaxToken('comment', text.slice(index, end));
                index = end;
                continue;
            }

            prefix = (config.lineComments || []).find(function(commentPrefix) {
                return text.startsWith(commentPrefix, index);
            });
            if (prefix) {
                end = findLineEnd(text, index);
                html += wrapSyntaxToken('comment', text.slice(index, end));
                index = end;
                continue;
            }
            if (config.decorators && text[index] === '@' && isIdentifierStart(text[index + 1])) {
                end = readIdentifier(text, index + 1);
                html += wrapSyntaxToken('meta', text.slice(index, end));
                index = end;
                continue;
            }
            if (config.atIdentifiers && text[index] === '@' && /[A-Za-z_-]/.test(text[index + 1] || '')) {
                end = index + 1;
                while (end < text.length && /[A-Za-z0-9_-]/.test(text[end])) {
                    end += 1;
                }
                html += wrapSyntaxToken('meta', text.slice(index, end));
                index = end;
                continue;
            }
            if (config.variables && text[index] === '$') {
                end = readShellVariable(text, index);
                if (end > index + 1) {
                    html += wrapSyntaxToken('variable', text.slice(index, end));
                    index = end;
                    continue;
                }
            }
            if (config.hexColors && text[index] === '#') {
                word = text.slice(index).match(/^#[0-9A-Fa-f]{3,8}\b/);
                if (word) {
                    html += wrapSyntaxToken('number', word[0]);
                    index += word[0].length;
                    continue;
                }
            }
            if (text[index] === '"' || text[index] === '\'' || (config.allowTemplateStrings && text[index] === '`')) {
                end = readQuotedString(text, index, config);
                html += wrapSyntaxToken('string', text.slice(index, end));
                index = end;
                continue;
            }
            if (/[0-9]/.test(text[index]) || (text[index] === '.' && /[0-9]/.test(text[index + 1] || ''))) {
                end = readNumberLiteral(text, index);
                html += wrapSyntaxToken('number', text.slice(index, end));
                index = end;
                continue;
            }
            if (isIdentifierStart(text[index])) {
                end = readIdentifier(text, index);
                word = text.slice(index, end);
                lowerWord = word.toLowerCase();
                if (config.keywords && (config.keywords.has(word) || config.keywords.has(lowerWord))) {
                    html += wrapSyntaxToken('keyword', word);
                } else if (config.booleans && (config.booleans.has(word) || config.booleans.has(lowerWord))) {
                    html += wrapSyntaxToken('boolean', word);
                } else if (config.nulls && (config.nulls.has(word) || config.nulls.has(lowerWord))) {
                    html += wrapSyntaxToken('null', word);
                } else {
                    nextIndex = findNonWhitespaceIndex(text, end);
                    if (text[nextIndex] === '(') {
                        html += wrapSyntaxToken('function', word);
                    } else {
                        html += escapeHTML(word);
                    }
                }
                index = end;
                continue;
            }
            if (SYNTAX_PUNCTUATION.has(text[index])) {
                html += wrapSyntaxToken('punctuation', text[index]);
                index += 1;
                continue;
            }
            if (SYNTAX_OPERATOR_CHARS.has(text[index])) {
                html += wrapSyntaxToken('operator', text[index]);
                index += 1;
                continue;
            }

            html += escapeHTML(text[index]);
            index += 1;
        }

        return html;
    }

    function highlightMarkdownInline(text) {
        const placeholders = [];
        let html = escapeHTML(text);

        function stash(value) {
            const token = '\u0000' + placeholders.length + '\u0000';
            placeholders.push(value);
            return token;
        }

        html = html.replace(/`([^`]+)`/g, function(match) {
            return stash('<span class="syntax-code">' + match + '</span>');
        });
        html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, function(match) {
            return stash('<span class="syntax-link">' + match + '</span>');
        });
        html = html.replace(/(\*\*|__)(.+?)\1/g, function(match) {
            return stash('<span class="syntax-emphasis">' + match + '</span>');
        });
        html = html.replace(/(\*|_)([^*_]+?)\1/g, function(match) {
            return stash('<span class="syntax-emphasis">' + match + '</span>');
        });

        return html.replace(/\u0000(\d+)\u0000/g, function(_, tokenIndex) {
            return placeholders[Number(tokenIndex)] || '';
        });
    }

    function parseMarkdownFenceLanguage(rawValue) {
        const token = String(rawValue || '').trim().replace(/^\{+|\}+$/g, '').split(/[\s,]/)[0];
        return normalizeSyntaxLanguage(token || 'plaintext');
    }

    function highlightMarkdown(text) {
        const lines = text.split('\n');
        const htmlLines = [];
        let inFence = false;
        let fenceMarker = '';
        let fenceLanguage = 'plaintext';

        lines.forEach(function(line) {
            const fenceMatch = line.match(/^(\s*)(```+|~~~+)\s*([^\s].*)?$/);
            const headingMatch = line.match(/^(\s{0,3}#{1,6})(\s+)(.*)$/);
            const quoteMatch = line.match(/^(\s*>\s?)(.*)$/);
            const listMatch = line.match(/^(\s*(?:[-*+]|\d+[.)])\s+)(.*)$/);

            if (fenceMatch) {
                if (!inFence) {
                    inFence = true;
                    fenceMarker = fenceMatch[2];
                    fenceLanguage = parseMarkdownFenceLanguage(fenceMatch[3]);
                } else if (fenceMatch[2][0] === fenceMarker[0]) {
                    inFence = false;
                    fenceMarker = '';
                    fenceLanguage = 'plaintext';
                }
                htmlLines.push(wrapSyntaxToken('meta', line));
                return;
            }

            if (inFence) {
                htmlLines.push(highlightSyntaxContent(line, fenceLanguage));
                return;
            }
            if (/^\s*([-*_])(?:\s*\1){2,}\s*$/.test(line)) {
                htmlLines.push(wrapSyntaxToken('punctuation', line));
                return;
            }
            if (headingMatch) {
                htmlLines.push(wrapSyntaxToken('punctuation', headingMatch[1]) + escapeHTML(headingMatch[2]) + '<span class="syntax-heading">' + highlightMarkdownInline(headingMatch[3]) + '</span>');
                return;
            }
            if (quoteMatch) {
                htmlLines.push(wrapSyntaxToken('punctuation', quoteMatch[1]) + highlightMarkdownInline(quoteMatch[2]));
                return;
            }
            if (listMatch) {
                htmlLines.push(wrapSyntaxToken('punctuation', listMatch[1]) + highlightMarkdownInline(listMatch[2]));
                return;
            }

            htmlLines.push(highlightMarkdownInline(line));
        });

        return htmlLines.join('\n');
    }

    function highlightMarkup(text) {
        let html = '';
        let index = 0;

        function findTagEnd(start) {
            let cursor = start + 1;
            let quote = '';

            while (cursor < text.length) {
                if (quote) {
                    if (text[cursor] === '\\') {
                        cursor += 2;
                        continue;
                    }
                    if (text[cursor] === quote) {
                        quote = '';
                    }
                    cursor += 1;
                    continue;
                }
                if (text[cursor] === '"' || text[cursor] === '\'') {
                    quote = text[cursor];
                    cursor += 1;
                    continue;
                }
                if (text[cursor] === '>') {
                    return cursor + 1;
                }
                cursor += 1;
            }

            return text.length;
        }

        function renderTag(tagText) {
            const matched = tagText.match(/^<(\/?)([A-Za-z][\w:.-]*)([\s\S]*?)(\/?)>$/);
            let attrsText;
            let cursor;
            let attrMatch;
            let result = '';
            let valueEnd;

            if (tagText.startsWith('<!--') || tagText.startsWith('<!') || tagText.startsWith('<?') || !matched) {
                return wrapSyntaxToken('meta', tagText);
            }

            result += wrapSyntaxToken('punctuation', '<' + matched[1]);
            result += wrapSyntaxToken('tag', matched[2]);
            attrsText = matched[3] || '';
            cursor = 0;

            while (cursor < attrsText.length) {
                if (/\s/.test(attrsText[cursor])) {
                    result += attrsText[cursor];
                    cursor += 1;
                    continue;
                }

                attrMatch = attrsText.slice(cursor).match(/^([^\s=/>]+)/);
                if (!attrMatch) {
                    result += escapeHTML(attrsText[cursor]);
                    cursor += 1;
                    continue;
                }

                result += wrapSyntaxToken('attr', attrMatch[1]);
                cursor += attrMatch[1].length;

                while (cursor < attrsText.length && /\s/.test(attrsText[cursor])) {
                    result += attrsText[cursor];
                    cursor += 1;
                }
                if (attrsText[cursor] === '=') {
                    result += wrapSyntaxToken('punctuation', '=');
                    cursor += 1;
                    while (cursor < attrsText.length && /\s/.test(attrsText[cursor])) {
                        result += attrsText[cursor];
                        cursor += 1;
                    }
                    if (attrsText[cursor] === '"' || attrsText[cursor] === '\'') {
                        valueEnd = readQuotedString(attrsText, cursor, {});
                        result += wrapSyntaxToken('string', attrsText.slice(cursor, valueEnd));
                        cursor = valueEnd;
                    } else {
                        attrMatch = attrsText.slice(cursor).match(/^[^\s/>]+/);
                        if (attrMatch) {
                            result += wrapSyntaxToken('string', attrMatch[0]);
                            cursor += attrMatch[0].length;
                        }
                    }
                }
            }

            if (matched[4]) {
                result += wrapSyntaxToken('punctuation', '/');
            }
            result += wrapSyntaxToken('punctuation', '>');
            return result;
        }

        while (index < text.length) {
            let end;

            if (text.startsWith('<!--', index)) {
                end = text.indexOf('-->', index + 4);
                end = end === -1 ? text.length : end + 3;
                html += wrapSyntaxToken('comment', text.slice(index, end));
                index = end;
                continue;
            }
            if (text[index] === '<') {
                end = findTagEnd(index);
                html += renderTag(text.slice(index, end));
                index = end;
                continue;
            }

            end = text.indexOf('<', index);
            end = end === -1 ? text.length : end;
            html += escapeHTML(text.slice(index, end));
            index = end;
        }

        return html;
    }

    function highlightJson(text) {
        let html = '';
        let index = 0;
        let end;
        let word;

        while (index < text.length) {
            if (/\s/.test(text[index])) {
                html += escapeHTML(text[index]);
                index += 1;
                continue;
            }
            if (text[index] === '"') {
                end = readQuotedString(text, index, {});
                word = text.slice(index, end);
                html += wrapSyntaxToken(text[findNonWhitespaceIndex(text, end)] === ':' ? 'property' : 'string', word);
                index = end;
                continue;
            }
            if (/[0-9-]/.test(text[index])) {
                end = readNumberLiteral(text, index);
                html += wrapSyntaxToken('number', text.slice(index, end));
                index = end;
                continue;
            }
            if (isIdentifierStart(text[index])) {
                end = readIdentifier(text, index);
                word = text.slice(index, end);
                if (word === 'true' || word === 'false') {
                    html += wrapSyntaxToken('boolean', word);
                } else if (word === 'null') {
                    html += wrapSyntaxToken('null', word);
                } else {
                    html += escapeHTML(word);
                }
                index = end;
                continue;
            }
            if (/[{}\[\],:]/.test(text[index])) {
                html += wrapSyntaxToken('punctuation', text[index]);
                index += 1;
                continue;
            }

            html += escapeHTML(text[index]);
            index += 1;
        }

        return html;
    }

    function highlightStructuredConfig(text, mode) {
        const lines = text.split('\n');
        const valueConfig = GENERIC_SYNTAX_CONFIGS.data;

        return lines.map(function(line) {
            let match;

            if (!line.trim()) {
                return '';
            }
            if (/^\s*#/.test(line)) {
                return wrapSyntaxToken('comment', line);
            }
            if (mode === 'toml' && /^\s*\[[^\]]+\]\s*$/.test(line)) {
                return wrapSyntaxToken('type', line);
            }

            match = mode === 'yaml'
                ? line.match(/^(\s*(?:-\s+)?)?([A-Za-z0-9_.-]+)(\s*:\s*)(.*)$/)
                : line.match(/^(\s*)([A-Za-z0-9_.-]+)(\s*=\s*)(.*)$/);

            if (match) {
                return escapeHTML(match[1] || '')
                    + wrapSyntaxToken('property', match[2])
                    + wrapSyntaxToken('punctuation', match[3])
                    + highlightGenericCode(match[4], valueConfig);
            }

            return highlightGenericCode(line, valueConfig);
        }).join('\n');
    }

    function highlightSyntaxContent(text, language) {
        const source = typeof text === 'string' ? text : '';
        const normalized = normalizeSyntaxLanguage(language);
        const config = GENERIC_SYNTAX_CONFIGS[normalized];

        if (!source) {
            return '';
        }
        if (source.length > SYNTAX_HIGHLIGHT_MAX_CHARS) {
            return escapeHTML(source);
        }
        if (normalized === 'markdown') {
            return highlightMarkdown(source);
        }
        if (normalized === 'html') {
            return highlightMarkup(source);
        }
        if (normalized === 'json') {
            return highlightJson(source);
        }
        if (normalized === 'yaml' || normalized === 'toml') {
            return highlightStructuredConfig(source, normalized);
        }
        if (config) {
            return highlightGenericCode(source, config);
        }

        return escapeHTML(source);
    }

    function highlightTextForPath(text, path) {
        return highlightSyntaxContent(text, getSyntaxLanguageForPath(path));
    }

    function isPreviewableImage(name) {
        return /\.(png|jpe?g|gif|webp|bmp|svg|avif)$/i.test(name);
    }

    function isPreviewablePdf(name) {
        return /\.pdf$/i.test(name || '');
    }

    function isPreviewablePptx(name) {
        return /\.pptx$/i.test(name || '');
    }

    function isPreviewableDocx(name) {
        return /\.docx$/i.test(name || '');
    }

    function isPreviewableXlsx(name) {
        return /\.xlsx$/i.test(name || '');
    }

    function supportsLowResPreview(name) {
        return /\.(png|jpe?g|gif)$/i.test(name || '');
    }

    function formatSize(bytes) {
        if (bytes < 1024) {
            return bytes + 'B';
        }
        if (bytes < 1024 * 1024) {
            return (bytes / 1024).toFixed(0) + 'K';
        }
        if (bytes < 1024 * 1024 * 1024) {
            return (bytes / (1024 * 1024)).toFixed(1) + 'M';
        }
        return (bytes / (1024 * 1024 * 1024)).toFixed(1) + 'G';
    }

    function formatFileModTime(value) {
        const timestamp = Number(value) || 0;
        if (!timestamp) {
            return '—';
        }

        try {
            return new Intl.DateTimeFormat(state.language, {
                year: 'numeric',
                month: 'short',
                day: 'numeric',
                hour: 'numeric',
                minute: '2-digit'
            }).format(new Date(timestamp));
        } catch (_) {
            return new Date(timestamp).toLocaleString();
        }
    }

})();
