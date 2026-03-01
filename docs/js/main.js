/* SSM Session Client – Documentation JS */

(function () {
  'use strict';

  /* ── Tab System ─────────────────────────────────────────── */
  document.querySelectorAll('.tab-group-wrapper').forEach(function (wrapper) {
    wrapper.querySelectorAll('.tab-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var target = btn.dataset.tab;

        // Deactivate all in this wrapper
        wrapper.querySelectorAll('.tab-btn').forEach(function (b) { b.classList.remove('active'); });
        wrapper.querySelectorAll('.tab-content').forEach(function (c) { c.classList.remove('active'); });

        // Activate clicked
        btn.classList.add('active');
        var panel = wrapper.querySelector('[id="' + target + '"]');
        if (panel) panel.classList.add('active');
      });
    });
  });

  /* ── Copy Buttons ───────────────────────────────────────── */
  document.querySelectorAll('pre').forEach(function (pre) {
    var btn = document.createElement('button');
    btn.className = 'copy-btn';
    btn.setAttribute('aria-label', 'Copy code');
    btn.textContent = 'Copy';
    pre.appendChild(btn);

    btn.addEventListener('click', function () {
      var code = pre.querySelector('code');
      if (!code) return;
      navigator.clipboard.writeText(code.textContent.trim()).then(function () {
        btn.textContent = 'Copied!';
        btn.classList.add('copied');
        setTimeout(function () {
          btn.textContent = 'Copy';
          btn.classList.remove('copied');
        }, 2000);
      }).catch(function () {
        // Fallback for older browsers
        var ta = document.createElement('textarea');
        ta.value = code.textContent.trim();
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        btn.textContent = 'Copied!';
        btn.classList.add('copied');
        setTimeout(function () { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 2000);
      });
    });
  });

  /* ── Mobile Sidebar ─────────────────────────────────────── */
  var menuBtn  = document.getElementById('menuBtn');
  var sidebar  = document.getElementById('sidebar');
  var overlay  = document.getElementById('sidebarOverlay');

  function openSidebar() {
    sidebar.classList.add('open');
    overlay.classList.add('visible');
    document.body.style.overflow = 'hidden';
  }

  function closeSidebar() {
    sidebar.classList.remove('open');
    overlay.classList.remove('visible');
    document.body.style.overflow = '';
  }

  if (menuBtn) menuBtn.addEventListener('click', openSidebar);
  if (overlay) overlay.addEventListener('click', closeSidebar);

  // Close on nav link click (mobile)
  document.querySelectorAll('.sidebar-nav a').forEach(function (a) {
    a.addEventListener('click', function () {
      if (window.innerWidth <= 768) closeSidebar();
    });
  });

  /* ── Scroll Spy ─────────────────────────────────────────── */
  var allNavLinks = document.querySelectorAll('.sidebar-nav a[href^="#"]');

  function setActive(id) {
    allNavLinks.forEach(function (link) {
      link.classList.toggle('active', link.getAttribute('href') === '#' + id);
    });
  }

  // Collect headings that are nav targets
  var targets = [];
  allNavLinks.forEach(function (link) {
    var id = link.getAttribute('href').slice(1);
    var el = document.getElementById(id);
    if (el) targets.push(el);
  });

  if ('IntersectionObserver' in window && targets.length) {
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) setActive(entry.target.id);
      });
    }, { rootMargin: '-10% 0px -80% 0px', threshold: 0 });

    targets.forEach(function (el) { observer.observe(el); });
  }

  /* ── Smooth-scroll offset for fixed header ───────────────── */
  document.querySelectorAll('a[href^="#"]').forEach(function (anchor) {
    anchor.addEventListener('click', function (e) {
      var id = anchor.getAttribute('href').slice(1);
      var target = document.getElementById(id);
      if (!target) return;
      e.preventDefault();
      var offset = parseInt(getComputedStyle(document.documentElement)
        .getPropertyValue('--header-h')) || 58;
      var top = target.getBoundingClientRect().top + window.pageYOffset - offset - 16;
      window.scrollTo({ top: top, behavior: 'smooth' });
    });
  });

  /* ── Version Badge ──────────────────────────────────────── */
  var versionEl = document.getElementById('versionBadge');
  if (versionEl) {
    var injected = versionEl.textContent.trim();
    // If the placeholder wasn't replaced at build time, fetch from GitHub API
    if (!injected || injected === '%%VERSION%%') {
      fetch('https://api.github.com/repos/alexbacchin/ssm-session-client/releases/latest')
        .then(function (r) { return r.json(); })
        .then(function (data) {
          if (data.tag_name) versionEl.textContent = data.tag_name;
        })
        .catch(function () {
          versionEl.textContent = '';
        });
    }
  }

})();
