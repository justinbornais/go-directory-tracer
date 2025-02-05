package utilities

import (
	"log"
	"regexp"
	"strings"
)

func CheckError(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func GetCSS() string {
	var data = `
body { margin: 0 2rem 2rem; font-family: sans-serif; word-break: break-all; max-width: 100vw; }
#top { position: sticky; top: 0; padding-top: 1rem; }
@media (prefers-color-scheme: light) {
    a, a:visited { color: blue; }
    a:hover { background-color: yellow; }
    body { background-color: #f5f5f5; }
    #top { background-color: #f5f5f5; }
}
@media (prefers-color-scheme: dark) {
    body { background-color: #121212; color: #f5f5f5; }
    a, a:visited { color: skyblue; }
    a:hover { color: white; }
    #top { background-color: #121212; }
}
h1 { margin: 0.25rem 0; }
ul, li { list-style-type: none; }
.n { text-decoration: none; color: inherit !important; }
.p { margin-right: 6px; }
.d { font-size: 20px; font-weight: bold; }
.f { font-size: 18px; padding-right: 10%; }
.q { width: 100%; font-size: 20px; padding: 12px 20px; margin: 8px 0; box-sizing: border-box; }
	`

	re := regexp.MustCompile(`/\*.*?\*/`)
	data = re.ReplaceAllString(data, "")
	data = strings.ReplaceAll(data, "\n", "")
	data = strings.ReplaceAll(data, "\r", "")
	data = strings.ReplaceAll(data, "\t", "")
	data = strings.Join(strings.Fields(data), " ") // Remove extra spaces.
	return data
}

func GetJS() string {
	data := `/* Check if on an Android device or not. */
var userAgent = navigator.userAgent.toLowerCase();
var Android = userAgent.indexOf("android") > -1;
var link = window.location.href;

const fuse = new Fuse(d, {
	keys: ['n'],
    includeScore: true
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
    'pdf': '📄'
};
  
const emoji = (f) => {
    const ext = f.split('.').pop();
    return exts[ext] || '📄';
};

function addData(val) {
    var ul = document.getElementById("dl"); /* Get the ul element. */
    let d2 = [];
    
    if(val.length === 0) d2 = [...d];
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
        return ` + "`<li class=\"d\"><a href=\"${o.n}\">📁 ${o.n}</a></li>`" + `;
    }).join('');
    ul.innerHTML += fh;
    
    var br = document.createElement("br");
    ul.appendChild(br);

    let ih = d2.map(o => {
        if (o.t !== "f") return "";
        let href = Android ? ` + "`https://docs.google.com/viewerng/viewer?url=${link}${o.n}`:`${o.n}`" + `;
        return ` + "`<li class=\"f\"><a href=\"${href}\" target=\"_blank\">${emoji(o.n)} ${o.n}</a></li>`" + `;
    }).join('');
    ul.innerHTML += ih;
}

addData("");
document.getElementById("q").addEventListener("keyup", (e) => addData(e.target.value));
`
	re := regexp.MustCompile(`/\*.*?\*/`)
	data = re.ReplaceAllString(data, "")
	data = strings.ReplaceAll(data, "\n", "")
	data = strings.ReplaceAll(data, "\r", "")
	data = strings.ReplaceAll(data, "\t", "")
	data = strings.Join(strings.Fields(data), " ") // Remove extra spaces.
	return data
}
