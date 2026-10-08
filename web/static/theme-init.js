var saved = null;
try {
	saved = localStorage.getItem('theme');
} catch (e) {}
var dark = saved === 'dark' || (saved !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches);
document.documentElement.dataset.theme = dark ? 'dark' : 'light';
