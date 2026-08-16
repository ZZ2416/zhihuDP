/* keys.js —— 开屏动画 + 密钥配置弹窗
 * 首次进入：开屏展示看山 banner → 弹出密钥配置（DeepSeek / 知乎）
 * 用户可跳过（用默认密钥）或提交自己的密钥。
 * 安全：默认密钥只存在于服务端，绝不下发明文；
 *       用户填写的密钥在浏览器内用服务端下发的 RSA 公钥加密后传输（RSA-OAEP/SHA-256），
 *       服务端持私钥解密，明文不落日志。 */
(function () {
  const STORAGE_KEY = 'zhihudp-keys'; // 'skip' | 'custom' | 无 = 首次
  const SPLASH_MS = 2000;             // 开屏时长

  /* ---- 轻提示 ---- */
  function toast(msg, ms) {
    const el = document.getElementById('toast');
    if (!el) return;
    el.textContent = msg;
    el.classList.add('show');
    clearTimeout(toast._t);
    toast._t = setTimeout(() => el.classList.remove('show'), ms || 2600);
  }

  /* ---- 开屏动画 ---- */
  function runSplash(cb) {
    const splash = document.getElementById('splash');
    if (!splash) { if (cb) cb(); return; }
    setTimeout(() => {
      splash.classList.add('hide');
      setTimeout(() => { splash.remove(); if (cb) cb(); }, 650);
    }, SPLASH_MS);
  }

  /* ---- 弹窗 ---- */
  function showKeyModal() { document.getElementById('key-modal-mask')?.classList.add('show'); }
  function hideKeyModal() { document.getElementById('key-modal-mask')?.classList.remove('show'); }
  function showKeyErr(msg) {
    const el = document.getElementById('key-err');
    if (!el) return;
    el.textContent = msg;
    el.classList.add('show');
  }
  function clearKeyErr() { document.getElementById('key-err')?.classList.remove('show'); }
  function setSaving(on) {
    const btn = document.getElementById('key-save');
    const skip = document.getElementById('key-skip');
    if (btn) { btn.disabled = on; btn.textContent = on ? '加密保存中…' : '保存我的密钥'; }
    if (skip) skip.disabled = on;
  }

  /* ---- 跳过：使用默认密钥（服务端 config.yaml，从未下发明文） ---- */
  window.skipKeys = function () {
    localStorage.setItem(STORAGE_KEY, 'skip');
    hideKeyModal();
    toast('已使用内置默认密钥');
  };

  /* ---- RSA-OAEP 加密（Web Crypto，SPKI 公钥导入） ---- */
  async function rsaEncrypt(pem, text) {
    const body = pem
      .replace(/-----BEGIN PUBLIC KEY-----/g, '')
      .replace(/-----END PUBLIC KEY-----/g, '')
      .replace(/\s+/g, '');
    const der = Uint8Array.from(atob(body), c => c.charCodeAt(0));
    const key = await crypto.subtle.importKey(
      'spki', der,
      { name: 'RSA-OAEP', hash: 'SHA-256' },
      false, ['encrypt']
    );
    const enc = await crypto.subtle.encrypt(
      { name: 'RSA-OAEP' },
      key,
      new TextEncoder().encode(text)
    );
    return btoa(String.fromCharCode(...new Uint8Array(enc)));
  }

  /* ---- 保存：公钥加密 → POST 到服务端 ---- */
  window.saveKeys = async function () {
    clearKeyErr();
    const ds = document.getElementById('key-deepseek').value.trim();
    const zh = document.getElementById('key-zhihu').value.trim();
    if (!ds && !zh) {
      showKeyErr('请至少填写一个密钥，或直接点击「跳过」使用默认密钥');
      return;
    }
    setSaving(true);
    try {
      // 1) 拉取服务端 RSA 公钥
      const pkResp = await fetch('/api/config/pubkey');
      if (!pkResp.ok) throw new Error('获取加密公钥失败，请稍后重试');
      const { public_key: pem } = await pkResp.json();
      // 2) 仅加密非空字段，避免无效密文
      const payload = {};
      if (ds) payload.deepseek_key_enc = await rsaEncrypt(pem, ds);
      if (zh) payload.zhihu_secret_enc = await rsaEncrypt(pem, zh);
      // 3) 提交（私钥仅在服务端，能解密的只有服务端）
      const resp = await fetch('/api/config/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await resp.json().catch(() => ({}));
      if (!resp.ok) throw new Error(data.error || '保存失败，请稍后重试');
      localStorage.setItem(STORAGE_KEY, 'custom');
      hideKeyModal();
      toast('密钥已加密保存并持久化，重启后继续生效');
    } catch (e) {
      showKeyErr(e.message || '保存失败，请稍后重试');
    } finally {
      setSaving(false);
    }
  };

  /* ---- 启动：开屏 → 首次弹窗 ---- */
  window.addEventListener('load', () => {
    runSplash(() => {
      if (!localStorage.getItem(STORAGE_KEY)) showKeyModal();
    });
  });
})();
