# Go Directory Tracer
## Author: Justin Bornais

This program, written in Golang, is used for turning a given directory into a makeshift file server. It does this by tracing through the root directory of your project and creating index.html webpages in each folder and subfolder, displaying the contents of each directory.

### Searching for Files and Folders
This program also incorporates [Fuse.js](https://www.fusejs.io/) for lightweight fuzzy-searching of files on the webpage.

### Omitting Files and Folders Using `.fileignore`
To prevent files and folders from being included in the generated html pages, simply create a `.fileignore` file and list all file/folder names that you want omitted (one per line). By default, the `.fileignore` file is also ignored, so there's no need to add it to `.fileignore`.  
- **Note**: You do not need to include any `/` for folder names you want to omit. The name of the folder should suffice.

## Running Locally
In order to run this code locally, you have the option of either cloning this repository or downloading the binary from the releases.

### Binaries
Feel free to download one of the binaries in the releases section. The `.exe` binary is for Windows while the plain file is for Linux machines.

To run, execute the following in the terminal, ensuring you pass the name of the website in the `--title` parameter:
```sh
./tracer --title "Some Website Title"
```

### Cloning the Repository
Make sure you have Golang installed. After cloning the repository, you have more flexibility to modify the CSS and JS to your own styles.

### Building the Binaries
To build the binaries for both linux and windows, follow these commands:
```sh
$Env:GOOS = "linux"
go build -ldflags='-s -w' -trimpath -o tracer
$Env:GOOS = "windows"
go build -ldflags='-s -w' -trimpath -o tracer.exe
```

## Running via GitHub Pages
It also works for Github Pages. There is a sample workflow file [here](./.github/workflows/ghpages.yml) that runs the program and deploys it on Github Pages.
- It does not commit the index.html files to your branch directly. Instead, it runs the script and uploads it to the servers.
- In fact, it does not even add the binary to your project whatsoever!