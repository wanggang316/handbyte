// Copy-to-clipboard for install command buttons.
document.querySelectorAll('.copy-btn').forEach((btn) => {
  btn.addEventListener('click', async () => {
    const target = document.querySelector(btn.dataset.copy);
    if (!target) return;
    const text = target.textContent.trim();
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const range = document.createRange();
      range.selectNodeContents(target);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
      document.execCommand('copy');
      sel.removeAllRanges();
    }
    const label = btn.querySelector('.copy-btn__text');
    const prev = label.textContent;
    btn.classList.add('is-copied');
    label.textContent = 'Copied';
    setTimeout(() => {
      btn.classList.remove('is-copied');
      label.textContent = prev;
    }, 1600);
  });
});
