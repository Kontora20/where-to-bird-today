export type ThemeChoice = 'light' | 'dark';

const storageKey = 'theme';

function savedChoice(): ThemeChoice {
	try {
		return localStorage.getItem(storageKey) === 'dark' ? 'dark' : 'light';
	} catch {
		return 'light';
	}
}

export const theme = $state({ choice: savedChoice() });

function applyTheme() {
	document.documentElement.dataset.theme = theme.choice;
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
