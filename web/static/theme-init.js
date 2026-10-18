var saved = null;
try {
	saved = localStorage.getItem('theme');
} catch (e) {}
document.documentElement.dataset.theme = saved === 'dark' ? 'dark' : 'light';
