import { en, lt, type MessageKey } from '$lib/messages';

export type Language = 'lt' | 'en';

const storageKey = 'language';
const catalogs = { en, lt };

// The saved choice, else Lithuanian (ADR 0005).
function initialLanguage(): Language {
	try {
		const saved = localStorage.getItem(storageKey);
		if (saved === 'lt' || saved === 'en') return saved;
	} catch {
		// storage blocked: Lithuanian
	}
	return 'lt';
}

export const locale = $state<{ language: Language }>({ language: initialLanguage() });

export function chooseLanguage(language: Language) {
	locale.language = language;
	try {
		localStorage.setItem(storageKey, language);
	} catch {
		// storage blocked: the choice lasts until the tab closes
	}
}

// The text for key in the chosen language. {name} marks where params.name goes;
// a plural message is picked by params.count.
export function t(key: MessageKey, params: Record<string, string | number> = {}): string {
	const message = catalogs[locale.language][key];
	const text =
		typeof message === 'string'
			? message
			: ((message as unknown as Record<string, string | undefined>)[
					new Intl.PluralRules(locale.language).select(Number(params.count))
				] ?? message.other);
	return text.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ''));
}

export function formatNumber(value: number, fractionDigits: number): string {
	return value.toLocaleString(locale.language, {
		minimumFractionDigits: fractionDigits,
		maximumFractionDigits: fractionDigits
	});
}

// The first letter of each month's name: Intl's own narrow form is a number in
// Lithuanian.
export function monthInitials(): string[] {
	const format = new Intl.DateTimeFormat(locale.language, { month: 'long' });
	return Array.from({ length: 12 }, (_, month) => format.format(new Date(2026, month, 1))[0].toUpperCase());
}

// "Oct 19" in English, "spalio 19 d." in Lithuanian, where Intl's short form
// is a bare "10-19".
export function formatDate(date: Date, { weekday = false, year = false } = {}): string {
	const part = (options: Intl.DateTimeFormatOptions) => date.toLocaleDateString(locale.language, options);
	if (locale.language === 'en') {
		return part({ weekday: weekday ? 'short' : undefined, day: 'numeric', month: 'short', year: year ? 'numeric' : undefined });
	}
	const day = part({ month: 'long', day: 'numeric' });
	return `${weekday ? `${part({ weekday: 'short' })}, ` : ''}${year ? `${date.getFullYear()} m. ` : ''}${day}`;
}

// The places the page names itself: the browser's location and a point
// clicked on the map. Any other label is a place's own name.
export function placeLabel(label: string): string {
	if (label === 'you') return t('place.you');
	if (label === 'your point') return t('place.point');
	return label;
}

// Words that mark a part of an eBird hotspot name as English.
const englishWords = new Set(
	`park forest lake river island islands tower bog pond ponds reserve trail pier cape national regional
	landfill preserve observation town ancient station sea fish north south castle old new and nature
	biosphere ornithological landscape bay marsh meadows meadow wetland wetlands beach coast area hill
	hills of the near at in road village city centre center mouth spit lagoon reservoir farm fields field
	swamp quarry cemetery garden gardens museum harbour harbor port bridge dam peninsula point to`.split(/\s+/)
);

function englishWordCount(text: string): number {
	return text.toLowerCase().split(/[^a-ząčęėįšųūžõäö]+/).filter((word) => englishWords.has(word)).length;
}

function lithuanianLetterCount(text: string): number {
	return (text.match(/[ąčęėįšųūž]/gi) ?? []).length;
}

// eBird hotspot names often hold both languages, in either order: "Vingis Park
// (Vingio parkas)" and "Sapieginės miškas (Sapieginė forest)". In Lithuanian
// only the part with fewer English words is shown; on a tie, the part with
// more Lithuanian letters, else the part outside the brackets. English keeps
// the whole name.
export function hotspotName(name: string): string {
	if (locale.language !== 'lt') return name;
	const match = name.match(/^(.*\S)\s*\(([^()]+)\)\s*$/);
	if (!match) return name;
	const [, outer, inner] = match;
	const outerEnglish = englishWordCount(outer);
	const innerEnglish = englishWordCount(inner);
	if (outerEnglish !== innerEnglish) return outerEnglish < innerEnglish ? outer : inner;
	return lithuanianLetterCount(inner) > lithuanianLetterCount(outer) ? inner : outer;
}
