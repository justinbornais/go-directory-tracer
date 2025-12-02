/* Check if on an Android device or not. */
var userAgent = navigator.userAgent.toLowerCase();
var Android = userAgent.indexOf("android") > -1;
var link = window.location.href;

const fuse = new Fuse(d, {
	keys: ['n'],
    isCaseInsensitive: false,
    distance: 100,
    threshold: 0.25,
    includeScore: true,
    shouldSort: true,
    minMatchCharLength: 3,
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
    return q.replace(/[.,\/#!?$%\^&\*;:{}=\-_`~()]/g, '').trim();
}

function addData(val) {
    var ul = document.getElementById("dl"); /* Get the ul element. */
    let d2 = [];
    
    if(val.length <= 2) d2 = [...d];
    else {
        const results = fuse.search(val);
        d2 = results.map(result => {
            return {
                n: result.item.n,
                t: result.item.t,
                m: result.item.m,
                s: result.item.s,
            };
        });
    }
    
    ul.textContent = "";

    let fh = d2.map(o => {
        if (o.t !== "d") return "";
        return `<li class="d"><a href="${toUrl(o.n)}">📁 ${o.n}</a></li>`;
    }).join('');
    ul.innerHTML += fh;
    
    var br = document.createElement("br");
    ul.appendChild(br);

    let ih = d2.map(o => {
        if (o.t !== "f") return "";
        let href = (Android && [android_pdf]) ? `https://docs.google.com/viewerng/viewer?url=${link}${toUrl(o.n)}`:`${toUrl(o.n)}`;
        let temp = '';
        if ([audio_embed] && ia(o.n))
            temp = `<br /><audio controls class="ia" preload="none"><source src="${toUrl(o.n)}" type="audio/mpeg"></audio>`;
        return `<li class="f"><a href="${href}" target="_blank">${em(o.n)} ${o.n}</a>${temp}</li>`;
    }).join('');
    ul.innerHTML += ih;
}

addData("");
document.getElementById("q").addEventListener("keyup", (e) => addData(nq(e.target.value)));