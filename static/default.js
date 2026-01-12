/* Check if on an Android device or not. */
var userAgent = navigator.userAgent.toLowerCase();
var Android = userAgent.indexOf("android") > -1;
var link = window.location.href;

const fuse = new Fuse(d, {
	keys: ['n'],
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
    'exe': '🖥️',
    'mp3': '🔊',
    'wav': '🔊',
    'ogg': '🔊'
};
  
const em = (f) => {
    const ext = f.split('.').pop();
    return exts[ext] || '📄';
};

const toUrl = (f) => {
    return encodeURIComponent(f).replaceAll('#', '%23');
};

const ia = (f) => {
    return !!f?.toLowerCase().match(/\.(mp3|wav|ogg|aac|flac|m4a)$/);
};

const nq = (q) => {
    return q.replace(/[.,\/#!?$%\^&\*;:{}=\-_`~()]/g, '').trim().toLowerCase();
};

function buildList() {
    const ul = document.getElementById("dl");
    const frag = document.createDocumentFragment();

    d.forEach(o => {
        if (o.t !== "d") return;
        const li = document.createElement("li");
        li.dataset.name = nq(o.n);
        li.dataset.type = "d";
        li.className = "d";
        li.innerHTML = `<a href="${toUrl(o.n)}">📁 ${o.n}</a>`;
        frag.appendChild(li);
    });

    d.forEach(o => {
        if (o.t !== "f") return;
        const li = document.createElement("li");
        li.dataset.name = nq(o.n);
        li.dataset.type = o.t;
        li.className = "f";
        const href = (Android && [android_pdf]) ? `https://docs.google.com/viewerng/viewer?url=${link}${toUrl(o.n)}` : toUrl(o.n);
        let temp = "";
        if ([audio_embed] && ia(o.n)) {
            temp = `<audio controls class="ia" preload="none"><source src="${toUrl(o.n)}" type="audio/mpeg"></audio>`;
        }

        li.innerHTML = `<a href="${href}" target="_blank">${em(o.n)} ${o.n}</a>${temp}`;
        frag.appendChild(li);
    });

    ul.appendChild(frag);
}

function filterList(q) {
    const items = document.querySelectorAll('#dl > li');

    if (q.length < 2) {
        items.forEach(li => li.classList.remove("hidden"));
        return;
    }

    const results = fuse.search(q).map(r => r.item);
    const visibleNames = new Set(results.map(r => nq(r.n)));

    items.forEach(li => {
        const name = li.dataset.name;
        if (visibleNames.has(name)) li.classList.remove("hidden");
        else li.classList.add("hidden");
    });
}

function debounce(fn, delay = 150) {
    let timeout;
    return (...args) => {
        clearTimeout(timeout);
        timeout = setTimeout(() => fn(...args), delay);
    };
}

buildList();
document.getElementById("q").addEventListener("keyup", debounce((e) => filterList(nq(e.target.value)), 150));