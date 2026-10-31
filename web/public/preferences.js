try {
  var preference = localStorage.getItem('safeops-preferences');
  if (/^(zh|en)\|(light|dark)$/.test(preference || '')) {
    document.documentElement.dataset.theme = preference.split('|')[1];
    document.documentElement.lang = preference.startsWith('en|') ? 'en' : 'zh-CN';
  }
} catch {}
