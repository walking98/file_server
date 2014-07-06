package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/sessions"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	//"path/filepath"
	"strings"
	"time"
)

var dir string
var port string
var logging bool
var store = sessions.NewCookieStore([]byte("keysecret"))

const MAX_MEMORY = 1 * 1024 * 1024
const VERSION = "0.9"

type File struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

func main() {

	//fmt.Println(len(os.Args), os.Args)
	if len(os.Args) > 1 && os.Args[1] == "-v" {
		fmt.Println("Version " + VERSION)
		os.Exit(0)
	}

	flag.StringVar(&dir, "dir", "/Users/jordi/", "Specify a directory to server files from.")
	flag.StringVar(&port, "port", ":8080", "Port to bind the file server")
	flag.BoolVar(&logging, "log", true, "Enable Log (true/false)")

	flag.Parse()

	if logging == false {
		log.SetOutput(ioutil.Discard)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(handleReq))
	log.Printf("Listening on port %s .....", port)
	http.ListenAndServe(port, mux)

}

func handleReq(w http.ResponseWriter, r *http.Request) {

	if r.Method == "PUT" {
		AjaxUpload(w, r)
		return
	}

	if r.FormValue("ajax") == "true" {
		AjaxActions(w, r)
		return
	}

	if strings.HasSuffix(r.URL.Path, "/") {
		log.Printf("Index dir %s", r.URL.Path)
		handleDir(w, r)
	} else {
		log.Printf("downloading file %s", path.Clean(dir+r.URL.Path))
		http.ServeFile(w, r, path.Clean(dir+r.URL.Path))
		//http.ServeContent(w, r, r.URL.Path)
		//w.Write([]byte("this is a test inside file handler"))

	}

}

func AjaxActions(w http.ResponseWriter, r *http.Request) {

	if r.FormValue("action") == "delete" {
		f := strings.Trim(r.FormValue("file"), "/")
		err := os.Remove(dir + f)
		if err != nil {
			fmt.Fprint(w, fmt.Sprintf("Error %s", err))
			return
		}
		fmt.Fprint(w, fmt.Sprintf("ok"))
		return
	}

	if r.FormValue("action") == "create_folder" {
		f := strings.Trim(r.FormValue("path"), "/") + "/" + r.FormValue("file")
		err := os.Mkdir(dir+f, 0777)
		if err != nil {
			fmt.Fprint(w, fmt.Sprintf("Error %s", err))
			return
		}
		fmt.Fprint(w, fmt.Sprintf("ok"))
		return
	}

	// @todo make some test cases of trim renames...
	if r.FormValue("action") == "rename" {
		fo := strings.Trim(r.FormValue("file"), "/")
		fn := strings.Trim(r.FormValue("new"), "/")
		fo = strings.Trim(fo, "../")
		fn = strings.Trim(fn, "../")
		fmt.Printf("Old %s new %s", fo, fn)

		if dir != "." {
			fo = dir + fo
			fn = dir + fn
		}

		err := os.Rename(fo, fn)
		if err != nil {
			fmt.Fprint(w, err)
			return
		}

		fmt.Fprint(w, "ok")
		return
	}

}

func handleDir(w http.ResponseWriter, r *http.Request) {

	var d string = ""

	//log.Printf("len %d,, %s", len(r.URL.Path), dir)
	if len(r.URL.Path) == 1 {
		// handle root dir
		d = dir

	} else {
		// @todo convert pahts to absolutes
		if dir == "." {
			d += r.URL.Path[1:]
		} else {
			d += dir + r.URL.Path[1:]
		}
		//log.Printf("filename %s", d)
	}

	thedir, err := os.Open(d)
	if err != nil {
		// not a directory, handle a 404
		//http.Error(w, "Page not found %s", 404)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer thedir.Close()

	finfo, err := thedir.Readdir(-1)
	if err != nil {
		return
	}

	// handle json format of dir...
	if r.FormValue("format") == "json" {

		var aout []*File

		for _, fi := range finfo {
			xf := &File{
				fi.Name(),
				fi.Size(),
				fi.ModTime(),
				fi.IsDir(),
			}
			aout = append(aout, xf)
		}

		xo, err := json.Marshal(aout)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(xo)
		return

	}

	out := ""
	for _, fi := range finfo {
		//log.Println(fi.Name())
		class := "file glyphicon glyphicon-file"
		name := fi.Name()
		if fi.IsDir() {
			class = "dir glyphicon glyphicon-folder-open"
			name += "/"
		}
		out += fmt.Sprintf("<li><a href='%s'><span class='%s'></span> %s</a>", name, class, name)
		out += fmt.Sprintf(" <a href='%s?action=delete' class='pull-right delete'><span class='glyphicon glyphicon-trash'> </span></a></li>", name)
	}

	// get flash messages?
	session, err := store.Get(r, "flash-session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	fm := session.Flashes("message")
	session.Save(r, w)
	//fmt.Fprintf(w, "%v", fm[0])

	t := template.Must(template.New("listing").Delims("[%", "%]").Parse(templateList))
	v := map[string]interface{}{
		"Title":   d,
		"Listing": template.HTML(out),
		"Path":    r.URL.Path,
		"notroot": len(r.URL.Path) > 1,
		"message": fm,
		"version": VERSION,
	}

	t.Execute(w, v)

	//w.Write([]byte("this is a test inside dir handle"))
}

func AjaxUpload(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		fmt.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pa := r.URL.Path[1:]

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		var ff string
		if dir != "." {
			ff = dir + pa + part.FileName()
		} else {
			ff = pa + part.FileName()
		}

		dst, err := os.Create(ff)
		defer dst.Close()

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err := io.Copy(dst, part); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	fmt.Fprint(w, "ok")
	return
}
