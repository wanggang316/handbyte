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

// Enable the Mac download once the Sparkle feed exists on this site.
// The release workflow writes appcast.xml next to this page; its newest
// enclosure is the notarized DMG attached to the matching GitHub Release.
const downloadBtn = document.getElementById('download-mac');
if (downloadBtn) {
  fetch('appcast.xml', { cache: 'no-cache' })
    .then((r) => (r.ok ? r.text() : Promise.reject(new Error(r.status))))
    .then((xml) => {
      const doc = new DOMParser().parseFromString(xml, 'application/xml');
      const items = Array.from(doc.getElementsByTagName('item'));
      const sparkle = 'http://www.andymatuschak.org/xml-namespaces/sparkle';
      const latest = items
        .map((item) => {
          const enclosure = item.getElementsByTagName('enclosure')[0];
          const url = enclosure?.getAttribute('url');
          if (!url) return null;
          // Sparkle writes version attributes on the enclosure or as item elements.
          const field = (name) => enclosure.getAttributeNS(sparkle, name)
            || item.getElementsByTagNameNS(sparkle, name)[0]?.textContent || '';
          const build = Number(field('version')) || 0;
          const version = field('shortVersionString');
          return { url, build, version };
        })
        .filter(Boolean)
        .sort((a, b) => b.build - a.build)[0];
      if (!latest) return;
      downloadBtn.href = latest.url;
      downloadBtn.classList.remove('is-soon');
      downloadBtn.removeAttribute('aria-disabled');
      downloadBtn.removeAttribute('tabindex');
      const meta = document.getElementById('download-mac-meta');
      if (meta) meta.textContent = `${latest.version ? 'v' + latest.version + ' · ' : ''}notarized DMG · macOS 14+`;
    })
    .catch(() => { /* no feed yet: keep the "coming soon" state */ });
}

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
