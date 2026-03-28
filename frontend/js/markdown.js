// ===== Markdown 渲染配置 =====
function setupMarked() {
    const renderer = new marked.Renderer();
    // 代码块：添加语言标签 + 复制按钮
    // marked v5+ renderer.code 接收 token 对象 { text, lang, escaped }
    renderer.code = function(token) {
        // 兼容新旧版本：新版传入 token 对象，旧版传入 (code, language)
        const code = (typeof token === 'object' && token !== null) ? (token.text || '') : token;
        const language = (typeof token === 'object' && token !== null) ? (token.lang || '') : arguments[1];
        const lang = language || 'plaintext';
        let highlighted = '';
        try {
            if (hljs.getLanguage(lang)) {
                highlighted = hljs.highlight(code, { language: lang }).value;
            } else {
                highlighted = hljs.highlightAuto(code).value;
            }
        } catch (e) {
            highlighted = code.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
        }
        const escapedCode = code.replace(/`/g, '\\`').replace(/\$/g, '\\$');
        return `<div class="code-block-wrap">
  <div class="code-block-header">
    <span class="code-block-lang">${lang}</span>
    <button class="code-copy-btn" onclick="copyCode(this, \`${escapedCode}\`)">
      <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>
      复制
    </button>
  </div>
  <pre><code class="hljs">${highlighted}</code></pre>
</div>`;
    };
    // marked v9+ 废弃了 setOptions，改用 marked.use()
    marked.use({ renderer, breaks: true, gfm: true });
}

// 全局代码复制函数（供 v-html 内联调用）
window.copyCode = function(btn, code) {
    navigator.clipboard.writeText(code).then(() => {
        btn.classList.add('copied');
        btn.innerHTML = `<svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg> 已复制`;
        setTimeout(() => {
            btn.classList.remove('copied');
            btn.innerHTML = `<svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v8a2 2 0 002 2z"/></svg> 复制`;
        }, 2000);
    }).catch(() => {});
};
