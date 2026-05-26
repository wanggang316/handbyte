// Copy install command on click.
document.querySelectorAll('.cmd[data-copy]').forEach((btn) => {
  btn.addEventListener('click', async () => {
    const text = btn.querySelector('.cmd__text')?.textContent?.trim();
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    btn.classList.add('is-copied');
    clearTimeout(btn._t);
    btn._t = setTimeout(() => btn.classList.remove('is-copied'), 1600);
  });
});

// Animate stat counters when they enter the viewport.
const ease = (t) => 1 - Math.pow(1 - t, 3);
const counters = document.querySelectorAll('[data-counter]');
const animate = (el) => {
  const target = parseFloat(el.dataset.counter);
  if (Number.isNaN(target)) return;
  const decimals = (el.dataset.counter.split('.')[1] || '').length;
  const duration = 1100;
  const start = performance.now();
  const step = (now) => {
    const p = Math.min(1, (now - start) / duration);
    const v = target * ease(p);
    el.textContent = decimals ? v.toFixed(decimals) : Math.round(v).toString();
    if (p < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
};
if ('IntersectionObserver' in window) {
  const io = new IntersectionObserver((entries) => {
    entries.forEach((e) => {
      if (e.isIntersecting) { animate(e.target); io.unobserve(e.target); }
    });
  }, { threshold: 0.3 });
  counters.forEach((c) => io.observe(c));
} else {
  counters.forEach(animate);
}
