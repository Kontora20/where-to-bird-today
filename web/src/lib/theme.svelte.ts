export type ThemeChoice = 'system' | 'light' | 'dark';

const storageKey = 'theme';
const darkQuery = window.matchMedia('(prefers-color-scheme: dark)');

function savedChoice(): ThemeChoice {
	try {
		const saved = localStorage.getItem(storageKey);
		return saved === 'light' || saved === 'dark' ? saved : 'system';
	} catch {
		return 'system';
	}
}

export const theme = $state({ choice: savedChoice() });

function applyTheme() {
	const dark = theme.choice === 'dark' || (theme.choice === 'system' && darkQuery.matches);
	document.documentElement.dataset.theme = dark ? 'dark' : 'light';
}

export function chooseTheme(choice: ThemeChoice) {
	theme.choice = choice;
	try {
		localStorage.setItem(storageKey, choice);
	} catch {
		// storage blocked: the choice lasts until the tab closes
	}
	applyTheme();
}

darkQuery.addEventListener('change', applyTheme);
