const fuse = new Fuse(sd, {
	keys: ['n', 'p'],
	isCaseSensitive: false,
	distance: 100,
	threshold: 0.25,
	includeScore: true,
	shouldSort: true,
	minMatchCharLength: 2,
	ignoreLocation: true
});

const exts = {
	'doc': '📝',
	'docx': '📝',
	'exe': '💻',
	'csv': '📊',
	'xls': '📊',
	'xlsx': '📊',
	'jpg': '📷',
	'jpeg': '📷',
	'png': '📷',
	'pdf': '📄',
	'mp3': '🔊',
	'wav': '🔊',
	'ogg': '🔊'
};

const em = (f) => {
	const ext = f.split('.').pop();
	return exts[ext] || '📄';
};

const toUrl = (f) => {
	return f.split('/').map(s => encodeURIComponent(s).replaceAll('#', '%23')).join('/');
};

const nq = (q) => {
	return q.replace(/[.,\/#!?$%\^&\*;:{}=\-_`~()]/g, '').trim().toLowerCase();
};

function buildHref(o) {
	const pathStr = o.p ? o.p + '/' + o.n : o.n;
	const url = toUrl(pathStr);
	return o.t === 'd' ? url + '/' : url;
}

function renderResults(q) {
	const dl = document.getElementById("dl");
	dl.innerHTML = '';
	if (q.length < 2) return;
	const results = fuse.search(q);
	if (results.length === 0) return;

	/* Group results by their path field */
	const groups = new Map();
	results.forEach(r => {
		const key = r.item.p || '';
		if (!groups.has(key)) groups.set(key, []);
		groups.get(key).push(r.item);
	});

	/* Sort groups: root first, then shallower paths, then alphabetically */
	const sortedPaths = [...groups.keys()].sort((a, b) => {
		const da = a ? a.split('/').length : 0;
		const db = b ? b.split('/').length : 0;
		if (da !== db) return da - db;
		return a.localeCompare(b);
	});

	/* Set of paths that will appear as group headers (excludes root) */
	const headerPaths = new Set(sortedPaths.filter(p => p !== ''));

	const frag = document.createDocumentFragment();

	const makeItem = (o) => {
		if (o.t === 'd') {
			const fullPath = (o.p ? o.p + '/' : '') + o.n;
			if (headerPaths.has(fullPath)) return null;
		}
		const div = document.createElement("div");
		div.className = 'sr-item';
		const icon = o.t === 'd' ? '📁' : em(o.n);
		div.innerHTML = '<a href="' + buildHref(o) + '">' + icon + ' ' + o.n + '</a>';
		return div;
	};

	sortedPaths.forEach(path => {
		const depth = path ? path.split('/').length : 0;

		if (path) {
			/* Bordered group block, indented by depth (top-level folders have no indent) */
			const groupDiv = document.createElement("div");
			groupDiv.className = 'sr-group';
			groupDiv.style.marginLeft = (Math.max(0, depth - 1) * 1.4) + 'rem';

			const headerDiv = document.createElement("div");
			headerDiv.className = 'sr-header';
			headerDiv.innerHTML = '<a href="' + toUrl(path) + '/">📁 ' + path + '/</a>';
			groupDiv.appendChild(headerDiv);

			groups.get(path).forEach(o => {
				const el = makeItem(o);
				if (el) groupDiv.appendChild(el);
			});

			frag.appendChild(groupDiv);
		} else {
			/* Root-level items: no block wrapper, flush with the left edge */
			groups.get(path).forEach(o => {
				const el = makeItem(o);
				if (el) frag.appendChild(el);
			});
		}
	});

	dl.appendChild(frag);
}

function debounce(fn, delay) {
	let timeout;
	return (...args) => {
		clearTimeout(timeout);
		timeout = setTimeout(() => fn(...args), delay || 150);
	};
}

document.getElementById("q").addEventListener("input", debounce((e) => renderResults(nq(e.target.value)), 150));
